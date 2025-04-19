package health

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	healthEndpoint  = "http://localhost:8080/api/health"
	refreshInterval = 10 * time.Second
)

var (
	statusMap   = make(map[string]HealthStatus)
	statusMux   sync.RWMutex
	lastUpdated time.Time
	httpClient  = &http.Client{Timeout: 5 * time.Second}
)

// StartHealthMonitor 启动健康监控服务
func StartHealthMonitor() {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	log.Println("start HealthMonitor success")
	for {
		select {
		case <-ticker.C:
			if err := refreshHealthData(); err != nil {
				log.Printf("Health update failed: %v", err)
			}
		}
	}
}

// refreshHealthData 原子化更新健康数据
func refreshHealthData() error {
	resp, err := httpClient.Get(healthEndpoint)
	if err != nil {
		return fmt.Errorf("HTTP请求失败: %w", err)
	}

	defer resp.Body.Close()

	rawData, _ := io.ReadAll(resp.Body)
	log.Println("it is data", string(rawData))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("非200响应: %d, Body: %s", resp.StatusCode, rawData)
	}

	var buffer bytes.Buffer
	if err := json.Compact(&buffer, rawData); err != nil {
		return fmt.Errorf("JSON格式错误: %w", err)
	}

	newMap, err := ParseHealthResponse(buffer.Bytes())
	if err != nil {
		return fmt.Errorf("数据解析失败: %w", err)
	}

	StatusMux.Lock()
	defer StatusMux.Unlock()
	MapNameHealth = newMap
	LastUpdated = time.Now()
	return nil
}

// GetNodeStatus 获取指定节点状态
func GetNodeStatus(name string) (HealthStatus, bool) {
	statusMux.RLock()
	defer statusMux.RUnlock()

	status, exists := statusMap[name]
	return status, exists
}

// GetAllStatus 获取完整状态快照
func GetAllStatus() map[string]HealthStatus {
	statusMux.RLock()
	defer statusMux.RUnlock()

	snapshot := make(map[string]HealthStatus, len(statusMap))
	for k, v := range statusMap {
		snapshot[k] = v
	}
	return snapshot
}
