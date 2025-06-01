package task

import (
	"fmt"
	"github.com/libp2p/go-libp2p/core/host"
	"golang.org/x/net/context"
	deafault "pic_offload/pkg/apis"
	"pic_offload/pkg/mdns"
	"sync"
	"time"
)

type Task struct {
	ID       string
	Command  string
	Hostname string
	FilePath string
	Tasktype string
	Done     bool
}
type TaskScheduler struct {
	Tasks    map[string]*Task
	lock     sync.RWMutex
	registry *mdns.PeerRegistry
	h        host.Host
}

/*
	type MigrationTask struct {
		ID          string
		Source      string
		Destination string
		Resource    string
		Command     string
	}
*/
type TaskMigrationRequest struct {
	TaskID      string // 任务ID，用于标识任务
	SourceHost  string // 当前主机名称
	TargetHost  string // 目标主机名称
	TaskDetails Task   // 任务详情，包括图像文件路径、命令等
}

func NewTaskScheduler(h host.Host, r *mdns.PeerRegistry) *TaskScheduler {
	return &TaskScheduler{
		Tasks:    make(map[string]*Task),
		h:        h,
		registry: r,
	}
}
func (ts *TaskScheduler) ListTasks() {
	ts.lock.RLock()
	defer ts.lock.RUnlock()

	fmt.Println("已调度的任务:")
	for _, task := range ts.Tasks {
		fmt.Printf("任务ID: %s, 命令: %s, 主机: %s 类型: %s\n", task.ID, task.Command, task.Hostname, task.Tasktype)
	}
}
func (ts *TaskScheduler) ScheduleTask(task *Task) {
	ts.lock.Lock()
	defer ts.lock.Unlock()

	ts.Tasks[task.ID] = task
	fmt.Printf("任务 %s 已调度给节点 %s\n", task.ID, task.Hostname)
}
func (ts *TaskScheduler) NewTask(ID string, Command string, Hostname string, FilePath string) *Task {
	aTask := &Task{
		ID:       ID,
		Command:  Command,
		Hostname: Hostname,
		FilePath: FilePath,
		Done:     false,
	}
	ts.Tasks[ID] = aTask
	return aTask
}
func (ts *TaskScheduler) TimerList() {
	// 记录调度开始时间

	//now we do exp
	ts.TransferTaskToTargetHost("yolov5-2", "edge02")
	ts.TransferTaskToTargetHost("yolov5-3", "edge03")
	fmt.Println("now we trans two missions")
	start := time.Now()
	allDoneLogged := false
	flag := false
	ctx, _ := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(deafault.ListtaskInterval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				// 如果你在别处调用了 cancel()，这里会退出；否则，ctx.Done 永远不会被触发，循环会一直运行
				return

			case <-t.C:
				// 遍历并打印当前所有任务状态
				for _, i := range ts.Tasks {
					fmt.Printf(
						"任务 %s 已调度给节点 %s 工作路径 %s Command为 %s 类型 %s 完成情况为 %v\n",
						i.ID, i.Hostname, i.FilePath, i.Command, i.Tasktype, i.Done,
					)
					if !i.Done && i.Hostname == deafault.Hostname {
						flag = true
						ts.DoTask(i.ID)

					} else if !i.Done && i.Hostname != deafault.Hostname {
						ts.AskTaskDone(i.ID)
					}
				}

				// 如果之前没打印完成耗时，检查是否所有任务都已完成
				if !allDoneLogged && flag {
					allDone := true
					for _, i := range ts.Tasks {
						if !i.Done {
							allDone = false
							break
						}
					}
					if allDone {
						elapsed := time.Since(start)
						fmt.Printf("所有任务已完成，总耗时：%v\n", elapsed)
						allDoneLogged = true
					}
				}
				// 如果 allDoneLogged 已经是 true，则不再重复打印
			}
		}
	}()
}
