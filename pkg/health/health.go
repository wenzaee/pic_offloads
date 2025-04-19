package health

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ResourceUsage struct {
	CPUUsage float64   `json:"cpuUsage"`
	MemUsage float64   `json:"memUsage"`
	Down     float64   `json:"down"`   // 下行带宽 (MB/s)
	Up       float64   `json:"up"`     // 上行带宽 (MB/s)
	Enable   bool      `json:"enable"` // 是否启用监控
	Update   time.Time `json:"update"` // 资源数据更新时间
}

type HealthStatus struct {
	Name        string        `json:"name"`
	AveRTT      float64       `json:"aveRTT"`      // 平均往返延迟(ms)
	Loss        float64       `json:"loss"`        // 丢包率(0-1)
	Times       int           `json:"times"`       // 总检测次数
	Success     int           `json:"success"`     // 成功次数
	Usage       ResourceUsage `json:"usage"`       // 资源使用
	LastUpdated time.Time     `json:"lastUpdated"` // 最后状态更新时间
}

func (h HealthStatus) HealthScore() float64 {
	// 权重分配
	cpuWeight := 0.3
	memWeight := 0.2
	latencyWeight := 0.3
	lossWeight := 0.2

	// 归一化计算
	cpuScore := (1 - h.Usage.CPUUsage) * 100
	memScore := (1 - h.Usage.MemUsage) * 100
	latencyScore := (1 - h.AveRTT/1000) * 100 // 假设最大延迟1秒
	if latencyScore < 0 {
		latencyScore = 0
	}
	lossScore := (1 - h.Loss) * 100

	return cpuScore*cpuWeight +
		memScore*memWeight +
		latencyScore*latencyWeight +
		lossScore*lossWeight
}

// 计算成功率
func (h HealthStatus) SuccessRate() float64 {
	if h.Times == 0 {
		return 0
	}
	return float64(h.Success) / float64(h.Times)
}

var (
	MapNameHealth = make(map[string]HealthStatus)
	StatusMux     sync.RWMutex
	LastUpdated   time.Time
)

func ParseHealthResponse(jsonData []byte) (map[string]HealthStatus, error) {
	type RawHealthData struct {
		AveRTT  float64 `json:"aveRTT"`
		Loss    float64 `json:"loss"`
		Times   int     `json:"times"`
		Success int     `json:"success"`
		Usage   struct {
			CPUUsage float64 `json:"cpuUsage"`
			MemUsage float64 `json:"memUsage"`
			Down     float64 `json:"down"`
			Up       float64 `json:"up"`
			Enable   bool    `json:"enable"`
			Update   string  `json:"update"`
		} `json:"usage"`
		LastUpdated string `json:"lastUpdated"`
	}

	var rawMap map[string]RawHealthData
	if err := json.Unmarshal(jsonData, &rawMap); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %v", err)
	}

	healthMap := make(map[string]HealthStatus)
	for name, data := range rawMap {
		// 解析时间字段
		parseTime := func(timeStr string) time.Time {
			t, _ := time.Parse(time.RFC3339Nano, timeStr)
			return t
		}

		healthMap[name] = HealthStatus{
			Name:    name,
			AveRTT:  data.AveRTT,
			Loss:    data.Loss,
			Times:   data.Times,
			Success: data.Success,
			Usage: ResourceUsage{
				CPUUsage: data.Usage.CPUUsage,
				MemUsage: data.Usage.MemUsage,
				Down:     data.Usage.Down,
				Up:       data.Usage.Up,
				Enable:   data.Usage.Enable,
				Update:   parseTime(data.Usage.Update),
			},
			LastUpdated: parseTime(data.LastUpdated),
		}
	}
	return healthMap, nil
}
func UpdateHealthStatus(jsonData []byte) error {
	newHealthMap, err := ParseHealthResponse(jsonData)
	if err != nil {
		return err
	}
	MapNameHealth = newHealthMap
	return nil
}

// 通过节点名称获取完整状态
func GetPeerStatus(name string) (HealthStatus, bool) {
	status, exists := MapNameHealth[name]
	if !exists {
		return HealthStatus{}, false
	}

	return status, true
}
