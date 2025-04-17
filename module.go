package main

import (
	"fmt"
	"net"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

// PeerInfo 节点信息结构
type PeerInfo struct {
	Hostname  string
	AddrInfo  peer.AddrInfo
	Protocols []string
}

// PeerRegistry 节点注册中心
type PeerRegistry struct {
	peers map[peer.ID]PeerInfo
	lock  sync.RWMutex
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers: make(map[peer.ID]PeerInfo),
	}
}

func (r *PeerRegistry) AddPeer(pi peer.AddrInfo) {
	r.lock.Lock()
	defer r.lock.Unlock()

	filteredAddrs := filterLocalAddresses(pi.Addrs)
	if len(filteredAddrs) == 0 {
		return
	}

	hostname := getHostname(filteredAddrs)
	r.peers[pi.ID] = PeerInfo{
		Hostname: hostname,
		AddrInfo: peer.AddrInfo{
			ID:    pi.ID,
			Addrs: filteredAddrs,
		},
	}
}

func (r *PeerRegistry) PrintPeers() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for id, info := range r.peers {
		fmt.Printf("Peer %s (%s):\n", id, info.Hostname)
		for _, addr := range info.AddrInfo.Addrs {
			fmt.Printf("  - %s\n", addr)
		}
	}
}

// mdnsNotifee mDNS通知处理器
type mdnsNotifee struct {
	PeerChan     chan peer.AddrInfo
	peerRegistry *PeerRegistry
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.peerRegistry.AddPeer(pi)
}

// 获取主机名
func getHostname(addrs []multiaddr.Multiaddr) string {
	if len(addrs) == 0 {
		return "unknown"
	}

	_, host, err := manet.DialArgs(addrs[0])
	if err != nil {
		return "unknown"
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return "unknown"
	}

	names, _ := net.LookupAddr(ip.String())
	if len(names) > 0 {
		return names[0]
	}
	return "unknown"
}

// 地址过滤
func filterLocalAddresses(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	var filtered []multiaddr.Multiaddr
	for _, addr := range addrs {
		if !isIPProtocol(addr) {
			continue
		}

		_, host, err := manet.DialArgs(addr)
		if err != nil {
			continue
		}

		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}

		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			continue
		}

		if isPrivateIP(ip) || ip.IsGlobalUnicast() {
			filtered = append(filtered, addr)
		}
	}
	return filtered
}

func isIPProtocol(addr multiaddr.Multiaddr) bool {
	for _, proto := range addr.Protocols() {
		if proto.Code == multiaddr.P_IP4 || proto.Code == multiaddr.P_IP6 {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsUnspecified()
}
