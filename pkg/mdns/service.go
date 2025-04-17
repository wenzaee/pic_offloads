package mdns

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// Service 封装 mDNS 服务
type Service struct {
	service mdns.Service
}

// NewService 创建 mDNS 服务实例
func NewService(cfg Config) *Service {
	notifee := notifee{
		registry: cfg.Registry,
	}

	return &Service{
		service: mdns.NewMdnsService(cfg.Host, cfg.Rendezvous, &notifee),
	}
}

// Start 启动服务
func (s *Service) Start() error {
	return s.service.Start()
}

// 修正点：结构体名称需要与实例化时的名称一致
type notifee struct {
	registry RegistryInterface
}

func (n *notifee) HandlePeerFound(pi peer.AddrInfo) {
	n.registry.AddPeer(pi)
}
