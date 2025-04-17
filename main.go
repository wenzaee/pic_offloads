package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

func main() {
	// 创建 libp2p 主机
	host, err := libp2p.New()
	if err != nil {
		log.Fatalf("Failed to create host: %v", err)
	}
	defer host.Close()

	// 设置 mDNS 服务
	rendezvous := "your_rendezvous_string"
	notifee := &mdnsNotifee{PeerChan: make(chan peer.AddrInfo)}
	mdnsService := mdns.NewMdnsService(host, rendezvous, notifee)

	// 启动 mDNS 服务
	if err := mdnsService.Start(); err != nil {
		log.Fatalf("Failed to start mDNS service: %v", err)
	}
	defer mdnsService.Close()

	// 监听中断信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

type mdnsNotifee struct {
	PeerChan chan peer.AddrInfo
}

// 处理发现的节点
func (n *mdnsNotifee) HandlePeerFound(pi peer.AddrInfo) {
	fmt.Printf("Discovered peer: %v\n", pi)
	n.PeerChan <- pi
}
