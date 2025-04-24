package deafault

import "time"

const (
	HostnameProtocol  = "/hostname-protocol/1.0.0"
	FilesProtocol     = "/file-transfer/1.0.0"
	AskProtocol       = "/ask-tasks/1.0.0"
	RequestProtocol   = "/Request-transfer/1.0.0"
	ListtaskInterval  = 5 * time.Second
	SendTaskProtocal  = "/task-send/1.0.0"
	CheckTaskInterval = 10 * time.Second
)
