package task

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

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

			task, err := createTask()
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
func createTask() (Task, error) {
	id := uuid.New().String()
	hostname, _ := os.Hostname()
	destDir := filepath.Join("worker", id)
	err := os.MkdirAll(destDir, 0755)
	if err != nil {
		return Task{}, err
	}
	return Task{
		ID:       id,
		Command:  "process_images",
		Hostname: hostname,
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
