package mdns

import (
	"fmt"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

// Service 封装 mDNS 服务
type Service struct {
	service mdns.Service
}

// NewService 创建 mDNS 服务实例
func NewService(cfg Config) *Service {

	return &Service{
		service: mdns.NewMdnsService(
			cfg.Host,
			cfg.Rendezvous,
			&notifier{registry: cfg.Registry}, // 必须传递指针
		),
	}
}

// Start 启动服务
func (s *Service) Start() error {
	return s.service.Start()
}

type notifier struct {
	registry RegistryInterface
}

func (n *notifier) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("[DEBUG] Found peer: %s\n", pi.ID)
	n.registry.AddPeer(pi)
}
