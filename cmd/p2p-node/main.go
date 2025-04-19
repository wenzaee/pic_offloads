package main

import (
	"fmt"
	"golang.org/x/net/context"
	"log"
	"os"
	"os/signal"
	"pic_offload/pkg/election"
	"pic_offload/pkg/health"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"pic_offload/pkg/core"
	"pic_offload/pkg/mdns"
)

func main() {
	// 初始化 libp2p 主机
	var err error
	core.Edgehost, err = libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/44599/quic",
			"/ip4/0.0.0.0/tcp/44599",
			"/ip4/0.0.0.0/udp/0/quic",
		),
		libp2p.DisableRelay(),
		libp2p.Ping(true), // 启用 Ping 协议
	)
	core.Edgehost.SetStreamHandler("/hostname-protocol", core.HandleRequest)
	if err != nil {
		log.Fatalf("Failed to create host: %v", err)
	}
	defer core.Edgehost.Close()
	fmt.Printf("host Peer ID: %s\n", core.Edgehost.ID())

	// 初始化组件
	registry := mdns.NewRegistry()
	mdnsService := mdns.NewService(mdns.Config{
		Rendezvous: "my-network",
		Host:       core.Edgehost,
		Registry:   registry,
	},
	)

	// 启动服务
	if err := mdnsService.Start(); err != nil {
		log.Fatalf("mDNS service failed: %v", err)
	}

	electorCfg := election.Config{
		ElectionInterval: 10 * time.Second,
		LeaseDuration:    30 * time.Second,
		RenewalInterval:  5 * time.Second,
	}
	hostname, _ := os.Hostname()
	elector := election.New(hostname, registry, electorCfg)
	go health.StartHealthMonitor()

	// 启动选举守护进程
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go elector.Run(ctx)

	// 添加选举状态监控
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for {
			select {
			case <-ticker.C:
				log.Printf("Current leader: %s", elector.GetLeader())
			case <-ctx.Done():
				return
			}
		}
	}()

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 退出时打印节点信息
	fmt.Println("\nDiscovered peers:")
	registry.Print()
}
