package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	registry := core.NewRegistry()
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

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 退出时打印节点信息
	fmt.Println("\nDiscovered peers:")
	registry.Print()
}
