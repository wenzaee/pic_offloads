package core

import (
	"fmt"
	"net"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// PeerInfo 存储节点详细信息
type PeerInfo struct {
	Hostname  string
	AddrInfo  peer.AddrInfo
	Protocols []string
}

// Registry 节点注册中心
type Registry struct {
	peers map[peer.ID]PeerInfo
	lock  sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		peers: make(map[peer.ID]PeerInfo),
	}
}

// AddPeer 添加并过滤节点
func (r *Registry) AddPeer(pi peer.AddrInfo) {
	r.lock.Lock()
	defer r.lock.Unlock()

	filtered := filterAddresses(pi.Addrs)
	if len(filtered) == 0 {
		return
	}

	r.peers[pi.ID] = PeerInfo{
		Hostname: resolveHostname(filtered),
		AddrInfo: peer.AddrInfo{
			ID:    pi.ID,
			Addrs: filtered,
		},
	}
}

// 地址过滤逻辑
func filterAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var valid []multiaddr.Multiaddr
	for _, addr := range addrs {
		if isAcceptableAddress(addr) {
			valid = append(valid, addr)
		}
	}
	return valid
}

// 主机名解析逻辑
func resolveHostname(addrs []multiaddr.Multiaddr) string {
	if len(addrs) == 0 {
		return "unknown"
	}

	_, host, _ := manet.DialArgs(addrs[0])
	names, _ := net.LookupAddr(host)
	if len(names) > 0 {
		return names[0]
	}
	return host
}

// 地址有效性检查
func isAcceptableAddress(addr multiaddr.Multiaddr) bool {
	if !isIPProtocol(addr) {
		return false
	}

	_, host, err := manet.DialArgs(addr)
	if err != nil {
		return false
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return !ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast()
}

func isIPProtocol(addr multiaddr.Multiaddr) bool {
	for _, proto := range addr.Protocols() {
		if proto.Code == multiaddr.P_IP4 || proto.Code == multiaddr.P_IP6 {
			return true
		}
	}
	return false
}

// 打印节点信息
func (r *Registry) Print() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for id, info := range r.peers {
		fmt.Printf("[%s] %s\n", id, info.Hostname)
		for _, addr := range info.AddrInfo.Addrs {
			fmt.Printf("  └─ %s\n", addr)
		}
	}
}
