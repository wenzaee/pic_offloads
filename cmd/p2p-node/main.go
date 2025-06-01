package main

import (
	"fmt"
	"github.com/libp2p/go-libp2p"
	"golang.org/x/net/context"
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
	deafault.Hostname, _ = os.Hostname()
	core.Edgehost, err = libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/5001",
		),
		libp2p.Ping(true), // 启用 Ping 协议
	)
	deafault.DeleteWorkerDir = true
	core.Edgehost.SetStreamHandler(deafault.HostnameProtocol, core.HandleNameRequest)
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
	core.Edgehost.SetStreamHandler(deafault.RequestProtocol, TaskScheduler.HandleRequestTask)
	core.Edgehost.SetStreamHandler(deafault.SendTaskProtocal, TaskScheduler.HandleSendTask)
	core.Edgehost.SetStreamHandler(deafault.AskProtocol, TaskScheduler.HandleAskTask)

	taskChan := make(chan task.Task, 10)
	ctx, _ := context.WithCancel(context.Background())
	//启动监控协程
	go func() {
		if err := task.MonitorImages(ctx, deafault.WorkDir, deafault.Threshold, deafault.Interval, taskChan, TaskScheduler); err != nil {
			log.Fatalf("Monitoring failed: %v", err)
		}
	}()
	go TaskScheduler.TimerList()
	//// 任务处理协程
	//go func() {
	//	for task := range taskChan {
	//		log.Printf("Processing task %s in %s\n", task.ID, task.FilePath)
	//		// 添加实际处理逻辑（如执行命令）
	//	}
	//}()

	// 保持主协程运行
	select {}
	//hs, _ := os.Hostname()
	//var task1 *task.Task
	//if hs == "edge02" {
	//	time.Sleep(5 * time.Second)
	//	fmt.Println(task1)
	//
	//	err = TaskScheduler.TransferTaskToTargetHost("task1", "edge01")
	//
	//	if err != nil {
	//		log.Println("err ", err)
	//		return
	//	}
	//}

	//
	//time.Sleep(1 * time.Second)
	//if hs == "edge02" {
	//
	//	TaskScheduler.RequestTaskMigration("task1", "master")
	//}
	//time.Sleep(5 * time.Second)
	//if hs == "master" {
	//	TaskScheduler.DoTask("task1")
	//	TaskScheduler.SendTask("task1", "edge02")
	//}
	//// 信号处理
	//
	//if hs == "edge01" {
	//	time.Sleep(15 * time.Second)
	//	TaskScheduler.AskTaskDone("task1")
	//}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	// 退出时打印节点信息
	fmt.Println("\nDiscovered peers:")
	registry.Print()
}
