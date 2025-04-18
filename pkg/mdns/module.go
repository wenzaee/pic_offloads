package mdns

import (
	"context"
	"fmt"
	"net"
	"pic_offload/pkg/core"
	"sync"
	"time"

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
	peers       map[peer.ID]PeerInfo
	lock        sync.RWMutex
	MapNamePeer map[string]peer.ID
}

func NewRegistry() *PeerRegistry {
	return &PeerRegistry{
		peers:       make(map[peer.ID]PeerInfo),
		MapNamePeer: make(map[string]peer.ID),
	}
}

func (r *PeerRegistry) AddPeer(pi peer.AddrInfo) {
	ctx := context.Background()
	r.lock.Lock()
	defer r.lock.Unlock()

	filteredAddrs := filterLocalAddresses(pi.Addrs)
	fmt.Println(filteredAddrs)
	if len(filteredAddrs) == 0 {
		return
	}
	core.Edgehost.Peerstore().AddAddrs(pi.ID, pi.Addrs, 10*time.Minute)

	hostname, err := core.RequestHostname(ctx, core.Edgehost, pi.ID)
	if err != nil {
		fmt.Println("err", err)
	}
	fmt.Println("hostname", hostname)
	r.MapNamePeer[hostname] = pi.ID
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

// mdnsNotifee  mDNS通知处理器
type mdnsNotifee struct {
	PeerChan     chan peer.AddrInfo
	peerRegistry *PeerRegistry
}

func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	n.peerRegistry.AddPeer(pi)
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
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			fmt.Println("hosterror:", err)
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

func (r *PeerRegistry) Print() {
	r.lock.RLock()
	defer r.lock.RUnlock()

	for id, info := range r.peers {
		fmt.Printf("[%s] %s\n", id, info.Hostname)
		for _, addr := range info.AddrInfo.Addrs {
			fmt.Printf("  └─ %s\n", addr)
		}
	}
	ps := core.Edgehost.Peerstore()

	// 假设我们已经将一些 peer 加入 Peerstore，这里我们可以通过遍历 Peerstore 来查看节点信息
	// 获取所有存储的 Peer IDs
	peers := ps.Peers()

	// 遍历每个 peer ID 并打印相关信息
	for _, pid := range peers {
		fmt.Println("Peer ID:", pid)

		// 获取 Peer ID 对应的地址列表
		addresses := ps.Addrs(pid)
		for _, addr := range addresses {
			fmt.Println("Peer Address:", addr)
		}
	}
}
