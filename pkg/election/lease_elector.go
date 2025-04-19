// pkg/election/lease_elector.go
package election

import (
	"context"
	"log"
	"pic_offload/pkg/health"
	"pic_offload/pkg/mdns"
	"sort"
	"sync"
	"time"
)

// LeaseElector 基于租期的选举器
type LeaseElector struct {
	mu            sync.RWMutex
	currentLeader string
	leaseExpiry   time.Time
	myHostname    string
	registry      *mdns.PeerRegistry

	config Config
}

type Config struct {
	ElectionInterval time.Duration // 选举检查间隔
	LeaseDuration    time.Duration // 租期时长
	RenewalInterval  time.Duration // 租约续期间隔
}

func New(hostname string, registry *mdns.PeerRegistry, cfg Config) *LeaseElector {
	return &LeaseElector{
		myHostname: hostname,
		registry:   registry,
		config:     cfg,
	}
}

// 启动选举守护进程
func (e *LeaseElector) Run(ctx context.Context) {
	// 初始等待网络稳定
	time.Sleep(5 * time.Second)

	// 启动选举循环
	go e.electionLoop(ctx)

	// 启动租约维护循环
	go e.leaseMaintenanceLoop(ctx)
}

// 选举主循环
func (e *LeaseElector) electionLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.ElectionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.shouldStartElection() {
				e.attemptAcquireLeadership()
			}
		}
	}
}

// 租约维护循环
func (e *LeaseElector) leaseMaintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.RenewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if e.amILeader() && time.Until(e.leaseExpiry) < e.config.RenewalInterval {
				e.renewLease()
			}
		}
	}
}

// 判断是否需要发起选举
func (e *LeaseElector) shouldStartElection() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 当前无领导者或租约过期
	return e.currentLeader == "" || time.Now().After(e.leaseExpiry)
}

// 尝试获取领导权
func (e *LeaseElector) attemptAcquireLeadership() {
	candidates := e.evaluateCandidates()

	// 选择最优候选者
	if len(candidates) > 0 && candidates[0].Name == e.myHostname {
		e.mu.Lock()
		e.currentLeader = e.myHostname
		e.leaseExpiry = time.Now().Add(e.config.LeaseDuration)
		e.mu.Unlock()
		log.Printf("[ELECTION] Acquired leadership until %s", e.leaseExpiry)
	}
}

// 评估候选节点
func (e *LeaseElector) evaluateCandidates() []health.HealthStatus {
	e.registry.Lock.RLock()
	defer e.registry.Lock.RUnlock()

	var candidates []health.HealthStatus
	for _, status := range health.MapNameHealth {
		if status.Usage.Enable && status.SuccessRate() > 0.8 {
			candidates = append(candidates, status)
		}
	}

	// 按健康评分排序
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].HealthScore() > candidates[j].HealthScore()
	})

	return candidates
}

// 续期租约
func (e *LeaseElector) renewLease() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.leaseExpiry = time.Now().Add(e.config.LeaseDuration)
	log.Printf("[ELECTION] Renewed lease until %s", e.leaseExpiry)
}

// 当前是否是领导者
func (e *LeaseElector) amILeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentLeader == e.myHostname
}

// 获取当前领导者
func (e *LeaseElector) GetLeader() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentLeader
}
