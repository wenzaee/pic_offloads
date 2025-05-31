package deafault

import "time"

var (
	Hostname        string
	DeleteWorkerDir = false // 新增变量，控制是否删除 worker 目录下的任务文件夹
)

const (
	WorkDir           = "./upload" // 上传目录路径
	Threshold         = 10         // 触发阈值
	Interval          = 5 * time.Second
	HostnameProtocol  = "/hostname-protocol/1.0.0"
	FilesProtocol     = "/file-transfer/1.0.0"
	AskProtocol       = "/ask-tasks/1.0.0"
	RequestProtocol   = "/Request-transfer/1.0.0"
	ListtaskInterval  = 5 * time.Second
	SendTaskProtocal  = "/task-send/1.0.0"
	CheckTaskInterval = 10 * time.Second
)
