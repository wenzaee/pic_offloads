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
}
type TaskScheduler struct {
	Tasks    map[string]*Task
	lock     sync.RWMutex
	registry *mdns.PeerRegistry
	h        host.Host
}
type MigrationTask struct {
	ID          string // 可选：任务唯一标识
	Source      string // 源主机地址（如 "host1:22" 或 "192.168.1.100"）
	Destination string // 目标主机地址（如 "host2:22" 或 "192.168.1.200"）
	Resource    string // 要迁移的资源路径（如 "/data/backup.tar"）
	Command     string // 迁移命令（如 "scp /data/backup.tar user@host2:/remote/path"）
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
		fmt.Printf("任务ID: %s, 命令: %s, 主机: %s\n", task.ID, task.Command, task.Hostname)
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
	}
	ts.Tasks[ID] = aTask
	return aTask
}
func (ts *TaskScheduler) TimerList() {

	ctx, _ := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(deafault.ListtaskInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				for _, i := range ts.Tasks {
					fmt.Printf("任务 %s 已调度给节点 %s 工作路径 %s Command为 %s\n", i.ID, i.Hostname, i.FilePath, i.Command)
				}
			}
		}
	}()
	return
}
