package election

import (
	"context"
	"encoding/json"
	"log"
	"os"
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

	heartbeatInterval = 5 * time.Second
	leaderTimeout     = 15 * time.Second
)

// ElectionService implements Bully algorithm but **stores leader as hostname**.
type ElectionService struct {
	h        host.Host
	registry *mdns.PeerRegistry

	mu         sync.RWMutex
	leaderHost string // 只存主机名，不存 peer.ID
	leaderSeen time.Time
	inElection bool

	ctx    context.Context
	cancel context.CancelFunc
	stopHB context.CancelFunc
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

// -------------------------------------------------- public --------------------------------------------------
func (es *ElectionService) Start() {
	es.h.SetStreamHandler(protoElection, es.handleElection)
	es.h.SetStreamHandler(protoCoordinator, es.handleCoordinator)

	time.AfterFunc(2*time.Second, es.startElection)
	go es.monitorLeader()
}

func (es *ElectionService) Stop() {
	es.cancel()
	es.h.RemoveStreamHandler(protoElection)
	es.h.RemoveStreamHandler(protoCoordinator)
	if es.stopHB != nil {
		es.stopHB()
	}
}

// -------------------------------------------------- Bully core --------------------------------------------------
func (es *ElectionService) startElection() {
	es.mu.Lock()
	if es.inElection {
		es.mu.Unlock()
		return
	}
	es.inElection = true
	es.mu.Unlock()

	selfHost, _ := os.Hostname()
	log.Printf("🔔 [%s] start ELECTION", selfHost)

	higher := es.higherPriorityHosts()
	log.Println("higher", higher)
	if len(higher) == 0 {
		es.becomeLeader()
		return
	}

	var wg sync.WaitGroup
	okCh := make(chan struct{}, len(higher))
	for _, hostName := range higher {
		pid, ok := es.registry.MapNamePeer[hostName]
		if !ok {
			continue
		}
		wg.Add(1)
		go func(p peer.ID) {
			defer wg.Done()
			log.Println("send eliction to ", p)
			if es.sendMsg(p, protoElection, selfHost) { // 发送自己 hostname 作为负载
				okCh <- struct{}{}
				log.Println("receive ok")
			}
		}(pid)
	}
	wg.Wait()
	close(okCh)
	log.Println("receive ", len(okCh), "ok")
	if len(okCh) == 0 {
		es.becomeLeader()
	} else {
		log.Printf("⏳ [%s] waiting COORDINATOR", selfHost)
	}
}

func (es *ElectionService) handleElection(s network.Stream) {
	defer s.Close()
	remotePID := s.Conn().RemotePeer()
	remoteHost := es.registry.Peers[remotePID].Hostname
	selfHost, _ := os.Hostname()

	if remoteHost == "" || remoteHost == selfHost {
		return
	}

	log.Printf("📨 [%s] ← ELECTION from %s", selfHost, remoteHost)

	// 回复 OK (空字符串即可)
	_ = es.sendMsg(remotePID, protoElection, "OK")

	// 若我优先级更高则发起选举
	if selfHost > remoteHost {
		es.startElection()
	}
}

// 处理COORDINATOR消息
func (es *ElectionService) handleCoordinator(s network.Stream) {
	defer s.Close()

	var leaderHost string
	if err := json.NewDecoder(s).Decode(&leaderHost); err != nil {
		return
	}

	es.mu.Lock()
	defer es.mu.Unlock()

	// 如果已经是Leader，就不再处理 COORDINATOR
	if es.leaderHost != "" && es.leaderHost != leaderHost {
		log.Printf("⚠️ [%s] Already have a leader: %s, ignoring new COORDINATOR", es.h.ID(), es.leaderHost)
		return
	}

	// 如果没有Leader，或者接收到更高优先级的COORDINATOR
	es.leaderHost = leaderHost
	es.leaderSeen = time.Now()
	es.inElection = false

	log.Printf("👑 [%s] accepted COORDINATOR %s", es.h.ID(), leaderHost)
}

func (es *ElectionService) becomeLeader() {
	selfHost, _ := os.Hostname()

	es.mu.Lock()
	es.leaderHost = selfHost
	es.leaderSeen = time.Now()
	es.inElection = false
	es.mu.Unlock()

	log.Printf("🥳 [%s] I AM THE NEW LEADER", selfHost)
	es.broadcast(protoCoordinator, selfHost)
	es.startHeartbeat()
}

// -------------------------------------------------- heartbeat --------------------------------------------------
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
				es.leaderSeen = time.Now()
				es.mu.Unlock()
				selfHost, _ := os.Hostname()
				es.broadcast(protoCoordinator, selfHost)
			}
		}
	}()
}

func (es *ElectionService) monitorLeader() {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	selfHost, _ := os.Hostname()

	for {
		select {
		case <-es.ctx.Done():
			return
		case <-t.C:
			es.mu.RLock()
			leader, seen := es.leaderHost, es.leaderSeen
			es.mu.RUnlock()

			if leader == selfHost {
				continue
			}
			if leader == "" || time.Since(seen) > leaderTimeout {
				log.Printf("⚠️  [%s] leader lost, restart election", selfHost)
				es.startElection()
			}
		}
	}
}

// -------------------------------------------------- utils --------------------------------------------------
func (es *ElectionService) higherPriorityHosts() []string {
	var hosts []string
	for name := range es.registry.MapNamePeer {
		hosts = append(hosts, name)
		log.Println("append", name)
	}
	sort.Strings(hosts)
	selfHost, _ := os.Hostname()

	var higher []string
	for _, h := range hosts {
		if h > selfHost {
			higher = append(higher, h)
		}
	}
	return higher
}

func (es *ElectionService) broadcast(protocol string, payload string) {
	for name, pid := range es.registry.MapNamePeer {
		if name == payload { // 不发给自己
			continue
		}
		es.sendMsg(pid, protocol, payload)
	}
}

func (es *ElectionService) sendMsg(pid peer.ID, protocol string, payload string) bool {
	ctx, cancel := context.WithTimeout(es.ctx, 3*time.Second)
	defer cancel()
	s, err := es.h.NewStream(ctx, pid, lp2pProto.ID(protocol))
	if err != nil {
		return false
	}
	defer s.Close()
	if err := json.NewEncoder(s).Encode(payload); err != nil {
		return false
	}
	return true
}
