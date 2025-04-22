package main

import (
	"fmt"
	"github.com/libp2p/go-libp2p"
	"log"
	"os"
	"os/signal"
	deafault "pic_offload/pkg/apis"
	"pic_offload/pkg/core"
	"pic_offload/pkg/election"
	"pic_offload/pkg/mdns"
	"pic_offload/pkg/task"
	"syscall"
	"time"
)

func main() {

	var err error

	core.Edgehost, err = libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
		),
		libp2p.DisableRelay(),
		libp2p.Ping(true), // 启用 Ping 协议
	)
	core.Edgehost.SetStreamHandler(deafault.HostnameProtocol, core.HandleRequest)
	if err != nil {
		log.Fatalf("Failed to create host: %v", err)
	}
	defer core.Edgehost.Close()

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

	electionService := election.NewElectionService(core.Edgehost, registry)
	electionService.Start()
	defer electionService.Stop()

	TaskScheduler := task.NewTaskScheduler(
		core.Edgehost,
		registry,
	)
	core.Edgehost.SetStreamHandler(deafault.FilesProtocol, TaskScheduler.Handlefiles)
	core.Edgehost.SetStreamHandler(deafault.AskProtocol, TaskScheduler.Handleasks)

	TaskScheduler.ListTasks()
	hs, _ := os.Hostname()
	if hs == "edge02" {
		time.Sleep(5 * time.Second)
		task1 := TaskScheduler.NewTask("task1", "图匹配任务", "edge02", "./test/")
		fmt.Println(task1)

		err = TaskScheduler.TransferTaskToTargetHost(task1, "edge01")

		if err != nil {
			log.Println("err ", err)
			return
		}
	}
	go TaskScheduler.TimerList()

	// 信号处理
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 退出时打印节点信息
	fmt.Println("\nDiscovered peers:")
	registry.Print()
}
