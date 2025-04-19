// 路径：pkg/election/election.go
package election

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"pic_offload/pkg/health"
	"pic_offload/pkg/mdns"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	electionProtocol  = "/leader-election/1.0.0"
	heartbeatInterval = 5 * time.Second
	electionTimeout   = 10 * time.Second
)

// ElectionService 选举服务
type ElectionService struct {
	host          host.Host
	registry      *mdns.PeerRegistry
	currentLeader *LeaderInfo
	mu            sync.RWMutex
	electionTimer *time.Timer
	ctx           context.Context
	cancel        context.CancelFunc
	currentEpoch  string // 新增当前轮次
}

type LeaderInfo struct {
	Epoch    string    `json:"epoch"`
	PeerID   peer.ID   `json:"peerId"`
	Hostname string    `json:"hostname"`
	Score    float64   `json:"score"`
	LastSeen time.Time `json:"lastSeen"`
}

func parseEpochTimestamp(epoch string) (int64, error) {
	parts := strings.Split(epoch, "-")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid epoch format")
	}
	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse timestamp failed: %v", err)
	}
	return timestamp, nil
}

func (es *ElectionService) generateEpoch() string {
	hostname, _ := os.Hostname()

	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), hostname)
}

func NewElectionService(h host.Host, r *mdns.PeerRegistry) *ElectionService {
	ctx, cancel := context.WithCancel(context.Background())
	return &ElectionService{
		host:         h,
		registry:     r,
		ctx:          ctx,
		cancel:       cancel,
		currentEpoch: "init",
	}
}

// Start 启动选举服务
func (es *ElectionService) Start() {
	es.host.SetStreamHandler(electionProtocol, es.handleStream)
	go es.runElectionLoop()
	go es.monitorLeader()
}

// Stop 停止服务
func (es *ElectionService) Stop() {
	es.cancel()
	es.host.RemoveStreamHandler(electionProtocol)
}

func (es *ElectionService) runElectionLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-es.ctx.Done():
			return
		case <-ticker.C:
			if es.shouldStartElection() {
				es.startElection()
			}
		}
	}
}

/*开始选举*/
func (es *ElectionService) shouldStartElection() bool {
	es.mu.RLock()
	defer es.mu.RUnlock()

	return es.currentLeader == nil ||
		time.Since(es.currentLeader.LastSeen) > electionTimeout
}

// 获取本节点健康评分
func (es *ElectionService) getLocalScore() float64 {
	hostName, _ := os.Hostname()
	healthStatus, exists := health.MapNameHealth[hostName]
	if !exists {
		return 0
	}
	log.Println("get host socre:", healthStatus.HealthScore())
	return healthStatus.HealthScore()
}

// 启动选举流程
func (es *ElectionService) startElection() {
	log.Printf("🏁 [Epoch:%s] Starting new election", es.currentEpoch)
	allPeers := es.getCandidatePeers()
	if len(allPeers) == 0 {
		es.declareSelfAsLeader()
		return
	}

	var maxScore float64
	var candidate *LeaderInfo

	// 本地决策选出候选
	for _, p := range allPeers {
		if score := p.Score; score > maxScore {
			maxScore = score
			candidate = p
		}
	}

	// 声明自己为Leader如果得分最高
	if candidate.PeerID == es.host.ID() {
		es.declareSelfAsLeader()
	} else {
		es.sendVoteRequest(candidate)
	}
}

// 获取所有候选节点信息 基于MapNamePeer
func (es *ElectionService) getCandidatePeers() []*LeaderInfo {
	es.registry.Lock.RLock()
	defer es.registry.Lock.RUnlock()

	var peers []*LeaderInfo
	for hostname, peerID := range es.registry.MapNamePeer {
		status, exists := health.MapNameHealth[hostname]
		if !exists {
			continue
		}

		peers = append(peers, &LeaderInfo{
			PeerID:   peerID,
			Hostname: hostname,
			Score:    status.HealthScore(),
			LastSeen: status.LastUpdated,
		})
	}
	return peers
}

// 声明自己为Leader
func (es *ElectionService) declareSelfAsLeader() {
	es.mu.Lock()
	defer es.mu.Unlock()

	hostname, _ := os.Hostname()
	newEpoch := es.generateEpoch()
	es.currentLeader = &LeaderInfo{
		PeerID:   es.host.ID(),
		Hostname: hostname,
		Score:    es.getLocalScore(),
		LastSeen: time.Now(),
	}
	es.currentEpoch = newEpoch

	log.Printf("🎉 [Epoch:%s] Elected as new leader! Score: %.2f",
		newEpoch, es.currentLeader.Score)
	es.broadcastLeaderInfo()
}

// 广播Leader信息
func (es *ElectionService) broadcastLeaderInfo() {
	if es.currentLeader == nil {
		return
	}

	leaderData, err := json.Marshal(es.currentLeader)
	if err != nil {
		log.Printf("[Epoch:%s] Marshal error: %v", es.currentEpoch, err)
		return
	}

	log.Printf("📢 [Epoch:%s] Broadcasting leader info", es.currentEpoch)

	for _, pid := range es.host.Peerstore().Peers() {
		if pid == es.host.ID() {
			continue
		}
		tarGetname := es.registry.Peers[pid].Hostname
		log.Println("will send to", pid, tarGetname, "leader", es.currentLeader.Hostname)
		go func(target peer.ID) {
			ctx, cancel := context.WithTimeout(es.ctx, 3*time.Second)
			defer cancel()

			s, err := es.host.NewStream(ctx, target, electionProtocol)
			if err != nil {
				return
			}
			defer s.Close()

			if _, err := s.Write(leaderData); err != nil {
				log.Printf("[Epoch:%s] Send error: %v", es.currentEpoch, err)
			}
		}(pid)
	}
}

// 处理网络流
func (es *ElectionService) handleStream(s network.Stream) {
	defer s.Close()

	var li LeaderInfo
	if err := json.NewDecoder(s).Decode(&li); err != nil {
		log.Printf("[Epoch:%s] Decode error: %v", es.currentEpoch, err)
		return
	}

	es.mu.Lock()
	defer es.mu.Unlock()
	log.Println("get a leader")

	if es.currentLeader == nil || es.currentEpoch == "init" {
		es.currentLeader = &li
		es.currentEpoch = li.Epoch
		log.Printf("接受初始Leader: %s (Epoch: %s)", li.Hostname, li.Epoch)
		return
	}

	currentTimestamp, _ := parseEpochTimestamp(es.currentLeader.Epoch)
	newTimestamp, _ := parseEpochTimestamp(li.Epoch)

	// 优先比较epoch新旧
	if es.currentLeader == nil ||
		newTimestamp > currentTimestamp {

		es.currentLeader = &li
		es.currentEpoch = li.Epoch
		log.Printf("🔄 [Epoch:%s] Updated leader: %s (Score: %.2f)",
			li.Epoch, li.Hostname, li.Score)
	}
}

// 监控Leader状态
func (es *ElectionService) monitorLeader() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-es.ctx.Done():
			return
		case <-ticker.C:
			es.checkLeaderStatus()
		}
	}
}

func (es *ElectionService) checkLeaderStatus() {
	es.mu.RLock()
	current := es.currentLeader
	es.mu.RUnlock()

	if current == nil {
		log.Printf("[Epoch:%s] No active leader", es.currentEpoch)
		return
	}

	if current.PeerID == es.host.ID() {
		log.Printf("[Epoch:%s] Sending heartbeat", es.currentEpoch)
		es.broadcastLeaderInfo()
		return
	}

	if time.Since(current.LastSeen) > electionTimeout {
		log.Printf("[Epoch:%s] Leader timeout, last seen: %v",
			es.currentEpoch, current.LastSeen)
		es.startElection()
	}
}

// 发送投票请求
func (es *ElectionService) sendVoteRequest(candidate *LeaderInfo) {
	// 实现分布式共识逻辑（示例使用简单直接选择）
	// 实际生产环境需要实现Raft/Paxos等算法
	es.mu.Lock()
	defer es.mu.Unlock()

	if es.currentLeader == nil || candidate.Score > es.currentLeader.Score {
		es.currentLeader = candidate
	}
}
