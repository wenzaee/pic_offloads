package task

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	deafault "pic_offload/pkg/apis"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 新增函数：检测本地 worker 目录下的文件夹并恢复任务
func (ts *TaskScheduler) RecoverTasks() error {
	workerDir := "worker"
	entries, err := os.ReadDir(workerDir)
	if err != nil {
		return fmt.Errorf("failed to read worker directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		folder := entry.Name()                  // e.g. "flask-550e8400-e29b-41d4-a716-446655440000"
		parts := strings.SplitN(folder, "-", 2) // split only on first dash
		if len(parts) != 2 {
			log.Printf("跳过无效目录名: %s", folder)
			continue
		}
		taskType, taskID := parts[0], parts[1]
		taskPath := filepath.Join(workerDir, folder)

		// 如果已经在内存里，就不用重复恢复
		if _, exists := ts.Tasks[taskID]; exists {
			continue
		}

		task := Task{
			ID:       taskID,
			Command:  "recover_images",
			Hostname: deafault.Hostname, // 或者 deafault.Hostname
			FilePath: taskPath,
			Tasktype: taskType,
			Done:     false,
		}
		ts.Tasks[taskID] = &task
		log.Printf("Recovered task %s (type=%s) from %s", taskID, taskType, taskPath)
	}
	return nil
}

func MonitorImages(
	ctx context.Context,
	workDir string,
	threshold int,
	interval time.Duration,
	taskChan chan<- Task,
	ts *TaskScheduler) error {

	fileInfo, err := os.Stat(workDir)
	if err != nil {
		return fmt.Errorf("invalid work directory: %w", err)
	}
	if !fileInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", workDir)
	}

	// 调用 RecoverTasks 函数恢复未完成的任务
	if err := ts.RecoverTasks(); err != nil {
		log.Printf("Failed to recover tasks: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			imgs, err := getImages(workDir)
			if err != nil {
				log.Printf("Error counting images: %v", err)
				continue
			}
			imgCount := len(imgs)
			if imgCount < threshold {
				continue
			}

			task, err := createTask("yolov5")
			task.Tasktype = "yolov5"
			ts.Tasks[task.ID] = &task
			if err != nil {
				log.Printf("Error creating task: %v", err)
				continue
			}
			taskChan <- task

			if err := moveImages(imgs, task.FilePath); err != nil {
				log.Printf("Error moving images to %s: %v", task.FilePath, err)
				os.RemoveAll(task.FilePath) // 清理失败任务目录
				continue
			}

			log.Printf("Created task %s with %d images", task.ID, imgCount)
		}
	}
}

// 获取目录中的图片列表
func getImages(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var imgs []string
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name()))
		switch ext {
		case ".jpg", ".jpeg", ".png", ".gif":
			imgs = append(imgs, filepath.Join(dir, f.Name()))
		}
	}
	return imgs, nil
}

// 创建新任务并生成目录
func createTask(taskType string) (Task, error) {
	id := uuid.New().String()
	hostname, _ := os.Hostname() // 动态获取当前主机名

	folderName := fmt.Sprintf("%s-%s", taskType, id)
	destDir := filepath.Join("worker", folderName)

	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return Task{}, err
	}
	return Task{
		ID:       id,
		Command:  "process_images",
		Hostname: hostname, // 使用动态主机名
		FilePath: destDir,
		Done:     false,
	}, nil
}

// 安全移动文件（先复制后删除）
func moveImages(srcFiles []string, destDir string) error {
	for _, srcPath := range srcFiles {
		fileName := filepath.Base(srcPath)
		destPath := filepath.Join(destDir, fileName)
		if err := copyAndDelete(srcPath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func copyAndDelete(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	err = out.Close()
	if err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}
