package election

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"pic_offload/pkg/mdns"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	lp2pProto "github.com/libp2p/go-libp2p/core/protocol"
)

const (
	protoElection    = "/bully/election/1.0.0"
	protoCoordinator = "/bully/coord/1.0.0"

	heartbeatInterval = 5 * time.Second  // leader → followers
	leaderTimeout     = 15 * time.Second // follower 检测 leader 心跳超时
)

// ElectionService implements the Bully algorithm on top of libp2p.
type ElectionService struct {
	h        host.Host
	registry *mdns.PeerRegistry

	mu         sync.RWMutex
	leader     peer.ID
	leaderSeen time.Time
	inElection bool

	ctx    context.Context
	cancel context.CancelFunc
	stopHB context.CancelFunc // 用于停止 leader 心跳 goroutine
}

// NewElectionService constructs an ElectionService bound to a libp2p host.
func NewElectionService(h host.Host, r *mdns.PeerRegistry) *ElectionService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ElectionService{
		h:        h,
		registry: r,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start registers stream handlers and boots auxiliary goroutines.
func (es *ElectionService) Start() {
	es.h.SetStreamHandler(protoElection, es.handleElection)
	es.h.SetStreamHandler(protoCoordinator, es.handleCoordinator)

	// 延时触发选举，给网络发现一些时间
	time.AfterFunc(2*time.Second, es.startElection)

	go es.monitorLeader()
}

// Stop shuts everything down gracefully.
func (es *ElectionService) Stop() {
	es.cancel()
	es.h.RemoveStreamHandler(protoElection)
	es.h.RemoveStreamHandler(protoCoordinator)
	if es.stopHB != nil {
		es.stopHB()
	}
}

// -------------------- Bully algorithm core --------------------

// startElection initiates a Bully round if not already running.
func (es *ElectionService) startElection() {
	es.mu.Lock()
	if es.inElection {
		es.mu.Unlock()
		return
	}
	es.inElection = true
	es.mu.Unlock()

	log.Printf("🔔 [%s] start ELECTION", es.h.ID())

	higher := es.higherPriorityPeers()
	if len(higher) == 0 {
		es.becomeLeader()
		return
	}

	// 向所有更高优先级节点并发发送 ELECTION
	var wg sync.WaitGroup
	okCh := make(chan struct{}, len(higher))
	for _, pid := range higher {
		wg.Add(1)
		go func(p peer.ID) {
			defer wg.Done()
			if es.sendMsg(p, protoElection, "ELECTION") {
				okCh <- struct{}{}
			}
		}(pid)
	}
	wg.Wait()
	close(okCh)

	// 若无人回应 OK，自身称王
	if len(okCh) == 0 {
		es.becomeLeader()
	} else {
		log.Printf("⏳ [%s] waiting COORDINATOR", es.h.ID())
	}
}

// handleElection processes an incoming ELECTION request.
func (es *ElectionService) handleElection(s network.Stream) {
	defer s.Close()

	remote := s.Conn().RemotePeer()
	if remote == es.h.ID() {
		// 忽略自己发给自己的选举
		return
	}
	log.Printf("📨 [%s] ← ELECTION from %s", es.h.ID(), remote)

	// 回复 OK
	_ = es.sendMsg(remote, protoElection, "OK")
	// 等待高优先级节点宣布协调者，不再发起新的选举
}

// handleCoordinator processes a COORDINATOR message.
func (es *ElectionService) handleCoordinator(s network.Stream) {
	defer s.Close()

	var msg string
	if err := json.NewDecoder(s).Decode(&msg); err != nil || msg != "COORDINATOR" {
		return
	}
	remote := s.Conn().RemotePeer()

	es.mu.Lock()
	es.leader = remote
	es.leaderSeen = time.Now()
	es.inElection = false
	es.mu.Unlock()

	log.Printf("👑 [%s] accepted COORDINATOR %s", es.h.ID(), remote)
}

// becomeLeader transitions this node into leader state.
func (es *ElectionService) becomeLeader() {
	es.mu.Lock()
	es.leader = es.h.ID()
	es.leaderSeen = time.Now()
	es.inElection = false
	es.mu.Unlock()

	log.Printf("🥳 [%s] I AM THE NEW LEADER", es.h.ID())

	es.broadcast(protoCoordinator, "COORDINATOR")
	es.startHeartbeat()
}

// -------------------- Heartbeat handling --------------------

func (es *ElectionService) startHeartbeat() {
	if es.stopHB != nil {
		es.stopHB()
	}
	ctx, cancel := context.WithCancel(es.ctx)
	es.stopHB = cancel

	go func() {
		t := time.NewTicker(heartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				es.mu.Lock()
				es.leaderSeen = time.Now() // 更新本地心跳时间
				es.mu.Unlock()
				es.broadcast(protoCoordinator, "COORDINATOR")
			}
		}
	}()
}

func (es *ElectionService) monitorLeader() {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()

	for {
		select {
		case <-es.ctx.Done():
			return
		case <-t.C:
			es.mu.RLock()
			ld, seen := es.leader, es.leaderSeen
			es.mu.RUnlock()

			if ld == es.h.ID() {
				// 我就是 leader
				continue
			}
			if ld == "" || time.Since(seen) > leaderTimeout {
				log.Printf("⚠️  [%s] leader lost, restarting election", es.h.ID())
				es.startElection()
			}
		}
	}
}

// -------------------- Utility --------------------

// higherPriorityPeers returns peers whose ID is lexicographically greater than ours.
func (es *ElectionService) higherPriorityPeers() []peer.ID {
	all := es.h.Peerstore().Peers()
	sort.Sort(all) // peer.IDSlice implements sort.Interface
	self := es.h.ID()

	var higher []peer.ID
	for _, p := range all {
		if p == self {
			continue
		}
		if p > self {
			higher = append(higher, p)
		}
	}
	return higher
}

func (es *ElectionService) broadcast(protocol string, payload string) {
	for _, pid := range es.h.Peerstore().Peers() {
		if pid == es.h.ID() {
			continue
		}
		es.sendMsg(pid, protocol, payload)
	}
}

func (es *ElectionService) sendMsg(pid peer.ID, protocol string, msg string) bool {
	ctx, cancel := context.WithTimeout(es.ctx, 3*time.Second)
	defer cancel()

	s, err := es.h.NewStream(ctx, pid, lp2pProto.ID(protocol))
	if err != nil {
		return false
	}
	defer s.Close()

	if err := json.NewEncoder(s).Encode(msg); err != nil {
		return false
	}
	return true
}
