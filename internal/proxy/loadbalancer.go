package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	Next(modelID string) (string, bool)
	MarkFailed(backend string)
	MarkSuccess(backend string)
	GetHealthStatus() map[string]BackendHealth
}

// BackendHealth 后端健康状态
type BackendHealth struct {
	URL       string    `json:"url"`
	Healthy   bool      `json:"healthy"`
	LastCheck time.Time `json:"last_check"`
	FailCount int       `json:"fail_count"`
	Latency   int64     `json:"latency_ms"`
}

// RoundRobinBalancer 轮询负载均衡器
type RoundRobinBalancer struct {
	mu       sync.RWMutex
	backends map[string][]Backend // modelID -> backends
	counters map[string]*uint32   // modelID -> counter
	health   map[string]*BackendHealth // backend -> health status
	httpClient *http.Client
}

type Backend struct {
	URL    string
	Weight int
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		backends:   make(map[string][]Backend),
		counters:   make(map[string]*uint32),
		health:     make(map[string]*BackendHealth),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (lb *RoundRobinBalancer) AddBackend(modelID string, backend Backend) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.backends[modelID] = append(lb.backends[modelID], backend)
	
	// 初始化健康状态
	if _, exists := lb.health[backend.URL]; !exists {
		lb.health[backend.URL] = &BackendHealth{
			URL:       backend.URL,
			Healthy:   true,
			LastCheck: time.Now(),
		}
	}

	if lb.counters[modelID] == nil {
		var counter uint32
		lb.counters[modelID] = &counter
	}
}

func (lb *RoundRobinBalancer) Next(modelID string) (string, bool) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	backends, exists := lb.backends[modelID]
	if !exists || len(backends) == 0 {
		return "", false
	}

	// 找到健康的后端
	counter := lb.counters[modelID]
	attempts := len(backends)

	for i := 0; i < attempts; i++ {
		idx := atomic.AddUint32(counter, 1) % uint32(len(backends))
		backend := backends[idx]

		if health, ok := lb.health[backend.URL]; ok && health.Healthy {
			return backend.URL, true
		}
	}

	// 所有后端都不健康，返回第一个（降级）
	return backends[0].URL, true
}

func (lb *RoundRobinBalancer) MarkFailed(backend string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if health, exists := lb.health[backend]; exists {
		health.Healthy = false
		health.FailCount++
	}
}

func (lb *RoundRobinBalancer) MarkSuccess(backend string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	if health, exists := lb.health[backend]; exists {
		health.Healthy = true
		health.FailCount = 0
	}
}

func (lb *RoundRobinBalancer) GetHealthyBackends(modelID string) []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var healthy []string
	for _, backend := range lb.backends[modelID] {
		if health, ok := lb.health[backend.URL]; ok && health.Healthy {
			healthy = append(healthy, backend.URL)
		}
	}
	return healthy
}

// GetHealthStatus 获取所有后端的健康状态
func (lb *RoundRobinBalancer) GetHealthStatus() map[string]BackendHealth {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	status := make(map[string]BackendHealth)
	for url, health := range lb.health {
		status[url] = BackendHealth{
			URL:       health.URL,
			Healthy:   health.Healthy,
			LastCheck: health.LastCheck,
			FailCount: health.FailCount,
			Latency:   health.Latency,
		}
	}
	return status
}

// CheckHealth 检查单个后端的健康状态
func (lb *RoundRobinBalancer) CheckHealth(backendURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 构造健康检查 URL（尝试 /health 端点，如果不存在则用 /v1/models）
	healthURL := backendURL + "/health"
	
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := lb.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()

	lb.mu.Lock()
	defer lb.mu.Unlock()

	if health, exists := lb.health[backendURL]; exists {
		health.LastCheck = time.Now()
		health.Latency = latency

		if err != nil || resp.StatusCode >= 500 {
			health.FailCount++
			// 连续失败 3 次才标记为不健康
			if health.FailCount >= 3 {
				health.Healthy = false
			}
			return false
		}

		if resp != nil {
			resp.Body.Close()
		}

		// 恢复健康
		health.Healthy = true
		health.FailCount = 0
		return true
	}

	return false
}

// StartHealthCheck 启动定期健康检查
func (lb *RoundRobinBalancer) StartHealthCheck(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			lb.runHealthChecks()
		}
	}()
	
	// 立即执行一次
	go lb.runHealthChecks()
}

// runHealthChecks 执行所有后端的健康检查
func (lb *RoundRobinBalancer) runHealthChecks() {
	lb.mu.RLock()
	
	// 收集所有唯一的后端 URL
	backendURLs := make(map[string]struct{})
	for _, backends := range lb.backends {
		for _, backend := range backends {
			backendURLs[backend.URL] = struct{}{}
		}
	}
	lb.mu.RUnlock()

	// 并行检查所有后端
	var wg sync.WaitGroup
	for url := range backendURLs {
		wg.Add(1)
		go func(backendURL string) {
			defer wg.Done()
			lb.CheckHealth(backendURL)
		}(url)
	}
	wg.Wait()
}

// GetModelBackends 获取指定模型的所有后端
func (lb *RoundRobinBalancer) GetModelBackends(modelID string) []BackendHealth {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var result []BackendHealth
	for _, backend := range lb.backends[modelID] {
		if health, exists := lb.health[backend.URL]; exists {
			result = append(result, BackendHealth{
				URL:       health.URL,
				Healthy:   health.Healthy,
				LastCheck: health.LastCheck,
				FailCount: health.FailCount,
				Latency:   health.Latency,
			})
		}
	}
	return result
}

// String 返回负载均衡器状态（用于日志）
func (lb *RoundRobinBalancer) String() string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var models []string
	for modelID := range lb.backends {
		models = append(models, modelID)
	}
	
	var healthy, unhealthy int
	for _, health := range lb.health {
		if health.Healthy {
			healthy++
		} else {
			unhealthy++
		}
	}

	return fmt.Sprintf("LoadBalancer[models=%v, healthy=%d, unhealthy=%d]", 
		models, healthy, unhealthy)
}