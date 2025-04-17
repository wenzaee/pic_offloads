package mdns

import (
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Config mDNS 服务配置
type Config struct {
	Rendezvous string            // 服务发现标识
	Host       host.Host         // libp2p 主机
	Registry   RegistryInterface // 节点注册接口
}

// RegistryInterface 注册中心接口定义
type RegistryInterface interface {
	AddPeer(peer.AddrInfo)
}
