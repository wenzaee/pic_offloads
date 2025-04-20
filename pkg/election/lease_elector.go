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
)
import lp2pProto "github.com/libp2p/go-libp2p/core/protocol"

const (
	protoElection    = "/bully/election/1.0.0"
	protoCoordinator = "/bully/coord/1.0.0"

	heartbeatInterval = 5 * time.Second  // Leader → All
	leaderTimeout     = 15 * time.Second // Follower检测Leader
)

type ElectionService struct {
	h        host.Host
	registry *mdns.PeerRegistry

	mu           sync.RWMutex
	leader       peer.ID
	leaderSeen   time.Time
	inElection   bool
	ctx          context.Context
	cancel       context.CancelFunc
	stopHB       context.CancelFunc // 心跳 goroutine 关闭句柄
	listenCancel context.CancelFunc // proto 句柄注销
}

func NewElectionService(h host.Host, r *mdns.PeerRegistry) *ElectionService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ElectionService{
		h:        h,
		registry: r,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// -------------------- Public API --------------------

func (es *ElectionService) Start() {
	// 流处理
	es.h.SetStreamHandler(protoElection, es.handleElection)
	es.h.SetStreamHandler(protoCoordinator, es.handleCoordinator)

	go es.monitorLeader()
	// 初始发起一次选举（可延时 2s，防止其他节点还未就绪）
	time.AfterFunc(2*time.Second, es.startElection)
}

func (es *ElectionService) Stop() {
	es.cancel()
	es.h.RemoveStreamHandler(protoElection)
	es.h.RemoveStreamHandler(protoCoordinator)
	if es.stopHB != nil {
		es.stopHB()
	}
}

// -------------------- Bully 核心逻辑 --------------------

func (es *ElectionService) startElection() {
	es.mu.Lock()
	if es.inElection {
		es.mu.Unlock()
		return
	}
	es.inElection = true
	es.mu.Unlock()

	log.Printf("🔔 [%s] Start ELECTION", short(es.h.ID()))
	higherPeers := es.higherPriorityPeers()
	if len(higherPeers) == 0 {
		es.becomeLeader()
		return
	}

	// 并行向更高优先级节点发送 ELECTION
	var wg sync.WaitGroup
	okCh := make(chan struct{}, len(higherPeers))
	for _, pid := range higherPeers {
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

	if len(okCh) == 0 {
		// 没人回应，做老大
		es.becomeLeader()
		return
	}
	// 收到 OK，等待 Coordinator
	log.Printf("⏳ [%s] Waiting COORDINATOR ...", short(es.h.ID()))
}

func (es *ElectionService) handleElection(s network.Stream) {
	defer s.Close()

	remote := s.Conn().RemotePeer()
	log.Printf("📨 [%s] <- ELECTION from %s", short(es.h.ID()), short(remote))

	// 1. 回复 OK
	_ = es.sendMsg(remote, protoElection, "OK")

	// 2. 如果自己还没在选举，立刻发起一次
	es.startElection()
}

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

	log.Printf("👑 [%s] Accept COORDINATOR %s", short(es.h.ID()), short(remote))
}

func (es *ElectionService) becomeLeader() {
	es.mu.Lock()
	es.leader = es.h.ID()
	es.leaderSeen = time.Now()
	es.inElection = false
	es.mu.Unlock()

	log.Printf("🥳 [%s] I am the new LEADER", short(es.h.ID()))
	es.broadcast(protoCoordinator, "COORDINATOR")
	es.startHeartbeat()
}

// -------------------- Heartbeat --------------------

func (es *ElectionService) startHeartbeat() {
	// 关闭旧 goroutine
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
			ld := es.leader
			seen := es.leaderSeen
			es.mu.RUnlock()

			if ld == "" || time.Since(seen) > leaderTimeout {
				log.Printf("⚠️  [%s] Leader lost, restart election", short(es.h.ID()))
				es.startElection()
			}
		}
	}
}

// -------------------- Utility --------------------

func (es *ElectionService) higherPriorityPeers() []peer.ID {
	all := es.h.Peerstore().Peers()
	self := es.h.ID().String()
	sort.Sort(all)
	var higher []peer.ID
	for _, p := range all {
		if p.String() > self { // 优先级：peer.ID 字面值更大
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

func short(id peer.ID) string {
	s := id.String()
	if len(s) > 8 {
		return s[:8]
	}
	return s
}
