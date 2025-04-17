package core

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"log"
	"net"
	"os"
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

var Edgehost host.Host

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
	ctx := context.Background()
	filtered := filterAddresses(pi.Addrs)
	if len(filtered) == 0 {
		return
	}
	hostname, err := requestHostname(ctx, Edgehost, pi.ID)
	if err != nil {
		log.Fatal("Failed to request host:", err)
	}
	r.peers[pi.ID] = PeerInfo{
		Hostname: hostname,
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
func requestHostname(ctx context.Context, host host.Host, peerID peer.ID) (string, error) {
	// 创建一个新的流
	stream, err := host.NewStream(ctx, peerID, "/hostname-protocol")
	if err != nil {
		return "", fmt.Errorf("failed to create new stream: %w", err)
	}
	defer stream.Close()

	// 发送请求数据
	_, err = stream.Write([]byte("request-hostname"))
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	// 读取响应数据（主机名）
	buf := make([]byte, 256)
	n, err := stream.Read(buf)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// 返回对方的主机名
	return string(buf[:n]), nil
}
func HandleRequest(s network.Stream) {
	defer s.Close()

	// 读取请求数据
	buf := make([]byte, 256)
	_, err := s.Read(buf)
	if err != nil {
		fmt.Println("Error reading request:", err)
		return
	}

	// 获取当前主机名
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 发送主机名作为响应
	_, err = s.Write([]byte(hostname))
	if err != nil {
		fmt.Println("Error sending hostname:", err)
	}
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
