package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func (ts *TaskScheduler) StartCPUTask(taskID, _ string) error {
	// CPU 服务的固定地址
	url := "http://10.96.209.61:12345/"

	// 发起 GET 请求
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("task %s: 请求 CPU 服务失败: %w", taskID, err)
	}
	defer resp.Body.Close()

	// 定义接收结构
	var result struct {
		Message   string `json:"message"`
		TimeTaken string `json:"time_taken"`
	}

	// 解析 JSON 响应
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("task %s: 解析响应 JSON 失败: %w", taskID, err)
	}
	// 直接输出结果
	log.Printf(
		"Task %s 完成 -> message: %s; time_taken: %s",
		taskID, result.Message, result.TimeTaken,
	)

	return nil
}

// 新增方法：与 Flask 服务通信，启动任务
func (ts *TaskScheduler) StartTaskWithFlask(taskID, flaskURL string) error {
	task, exists := ts.Tasks[taskID]
	if !exists {
		return fmt.Errorf("任务ID %s 不存在", taskID)
	}

	// 构造请求数据，新增 file_path 字段
	reqBody := map[string]string{
		"task_id":   task.ID,
		"command":   task.Command,
		"file_path": task.FilePath, // 新增字段，传递文件路径
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("JSON编码失败: %v", err)
	}

	// 发送 HTTP POST 请求
	resp, err := http.Post(flaskURL+"/start_task", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Flask服务返回错误状态: %s", resp.Status)
	}

	// 解析 Flask 返回的 JSON 响应
	var response struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("解析 Flask 响应失败: %v", err)
	}

	// 根据 status 字段处理响应
	if response.Status != "success" {
		return fmt.Errorf("Flask 服务处理失败: %s", response.Message)
	}

	fmt.Printf("任务 %s 已成功发送到 Flask 服务，响应消息: %s\n", taskID, response.Message)
	return nil
}

// 调用 StartTaskWithFlask
func (ts *TaskScheduler) DoTask(taskID string) {
	task, ok := ts.Tasks[taskID]
	if !ok {
		log.Printf("任务 %s 不存在", taskID)
		return
	}

	log.Println("准备开始任务", task.ID, task.Command, "类型:", task.Tasktype)

	switch task.Tasktype {

	case "yolov5":
		flaskURL := "http://localhost:5000"
		go func(id, url string) {
			if err := ts.StartTaskWithFlask(id, url); err != nil {
				log.Printf("Task %s (yolov5) 启动失败: %v", id, err)
				return
			}
			log.Printf("Task %s (yolov5) 完成", id)
		}(taskID, flaskURL)

	case "yolov4":
		/*go func(id, cmd string) {
			if err := ts.RunShellCommand(cmd); err != nil {
				log.Printf("Task %s (yolov4) 启动失败: %v", id, err)
				return
			}
			log.Printf("Task %s (yolov4) 完成", id)
		}(taskID, task.Command)*/

	case "cpu":
		//if err := ts.StartCPUTask(taskID, ""); err != nil {
		//	log.Printf("Task %s (cpu) 启动失败: %v", taskID, err)
		//	return
		//}
		//ts.Tasks[taskID].Done = true
		go func(id string) {
			if err := ts.StartCPUTask(id, ""); err != nil {
				log.Printf("Task %s (cpu) 启动失败: %v", id, err)
				return
			}
			ts.Tasks[taskID].Done = true
			log.Printf("Task %s (cpu) 完成", id)
		}(taskID)

	default:
		log.Printf("未知的任务类型: %s", task.Tasktype)
	}
}
