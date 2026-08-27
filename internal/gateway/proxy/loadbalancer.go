package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"modelgate/internal/config"
	"modelgate/internal/infra/logger"
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
	BackendID         string    `json:"backend_id"`
	URL               string    `json:"url"`
	ModelName         string    `json:"model_name"`
	Healthy           bool      `json:"healthy"`
	LastCheck         time.Time `json:"last_check"`
	FailCount         int       `json:"fail_count"`
	Latency           int64     `json:"latency_ms"`
	MaxConcurrency    int       `json:"max_concurrency"`    // 最大并发限制 (0=不限制)
	ActiveConcurrency int32     `json:"active_concurrency"` // 当前活跃并发数（atomic）
}

// RoundRobinBalancer 加权最少连接负载均衡器（Weighted Least-Connections）
type RoundRobinBalancer struct {
	mu       sync.RWMutex
	backends map[string][]Backend      // modelID -> backends
	health   map[string]*BackendHealth // backend -> health status
	httpClient *http.Client
}

// Backend represents a backend instance with metadata
type Backend struct {
	ID             string
	URL            string
	Weight         int
	ModelName      string // The actual model name used by the backend
	APIKey         string // API key for backend authentication
	MaxConcurrency int    // 最大并发请求数，0 表示不限制
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{
		backends:   make(map[string][]Backend),
		health:     make(map[string]*BackendHealth),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (lb *RoundRobinBalancer) AddBackend(modelID string, backend Backend) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	lb.backends[modelID] = append(lb.backends[modelID], backend)

	// 初始化健康状态 - 使用 backend ID 作为 key
	if _, exists := lb.health[backend.ID]; !exists {
		lb.health[backend.ID] = &BackendHealth{
			BackendID:      backend.ID,
			URL:            backend.URL,
			ModelName:      backend.ModelName,
			Healthy:        true,
			LastCheck:      time.Now(),
			MaxConcurrency: backend.MaxConcurrency,
		}
	}
}

func (lb *RoundRobinBalancer) Next(modelID string, defaultModel string) (*Backend, string, bool) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	backends, exists := lb.backends[modelID]
	if !exists || len(backends) == 0 {
		// 如果没有找到对应 model 的 backend，尝试使用 default model
		if defaultModel != "" && defaultModel != modelID {
			logger.Infof("Next: no backends found for model %s, trying default model %s", modelID, defaultModel)
			return lb.tryGetBackend(defaultModel, modelID)
		}
		logger.Infof("Next: no backends found for model %s (not in map)", modelID)
		return nil, modelID, false
	}

	return lb.tryGetBackend(modelID, modelID)
}

// tryGetBackend 使用加权最少连接策略（Weighted Least-Connections）选择后端并原子性地获取并发许可
// 加权负载率 = ActiveConcurrency / Weight，优先选择加权负载率最低的健康后端
// 使用随机起始索引打破平局，防止低并发时流量偏斜到固定后端
// 返回的后端已完成并发计数递增，调用方无需再次调用 AcquireBackend
func (lb *RoundRobinBalancer) tryGetBackend(lookupModel string, requestedModel string) (*Backend, string, bool) {
	backends := lb.backends[lookupModel]
	if len(backends) == 0 {
		return nil, requestedModel, false
	}

	// 加权最少连接策略（Weighted Least-Connections）：
	// 选择 ActiveConcurrency/Weight 最小的健康后端
	// 使用随机起始索引打破平局，防止并发数相同时总是选择同一个后端
	// 重试最多 3 次以应对 CAS 竞争
	for attempt := 0; attempt < 3; attempt++ {
		var bestBackend *Backend
		var bestHealth *BackendHealth
		var bestConcurrency int32
		var bestWeight int
		found := false

		startIdx := rand.Intn(len(backends))
		for j := 0; j < len(backends); j++ {
			i := (startIdx + j) % len(backends)

			health, ok := lb.health[backends[i].ID]
			if !ok || !health.Healthy {
				continue
			}

			current := atomic.LoadInt32(&health.ActiveConcurrency)

			// 跳过已满载的后端
			if health.MaxConcurrency > 0 && current >= int32(health.MaxConcurrency) {
				logger.Infof("tryGetBackend: backend %s is at capacity (%d/%d), skipping", backends[i].ID, current, health.MaxConcurrency)
				continue
			}

			weight := backends[i].Weight
			if weight <= 0 {
				weight = 1
			}

			if !found {
				bestConcurrency = current
				bestWeight = weight
				bestBackend = &backends[i]
				bestHealth = health
				found = true
				continue
			}

			// 加权比较: (current+1)/weight vs (bestConcurrency+1)/bestWeight
			// 交叉乘法避免浮点: (current+1) * bestWeight < (bestConcurrency+1) * weight
			if int64(current+1)*int64(bestWeight) < int64(bestConcurrency+1)*int64(weight) {
				bestConcurrency = current
				bestWeight = weight
				bestBackend = &backends[i]
				bestHealth = health
			}
		}

		if !found {
			break // 没有健康且未满载的后端
		}

		// 原子性地获取并发许可
		if bestHealth.MaxConcurrency > 0 {
			// 使用 CAS 确保不超过并发上限
			if atomic.CompareAndSwapInt32(&bestHealth.ActiveConcurrency, bestConcurrency, bestConcurrency+1) {
				if lookupModel != requestedModel {
					logger.Infof("tryGetBackend: using fallback model %s for request model %s (backend: %s)", lookupModel, requestedModel, bestBackend.ID)
				}
				return bestBackend, lookupModel, true
			}
			// CAS 失败（其他 goroutine 先获取了），重新扫描
			continue
		}

		// 无并发限制，直接递增计数
		atomic.AddInt32(&bestHealth.ActiveConcurrency, 1)
		if lookupModel != requestedModel {
			logger.Infof("tryGetBackend: using fallback model %s for request model %s (backend: %s)", lookupModel, requestedModel, bestBackend.ID)
		}
		return bestBackend, lookupModel, true
	}

	// 所有后端都不健康或满载
	// 如果所有健康后端都满载，返回 false
	healthyCount := 0
	busyCount := 0
	for i := 0; i < len(backends); i++ {
		health, ok := lb.health[backends[i].ID]
		if !ok || !health.Healthy {
			continue
		}
		healthyCount++
		if health.MaxConcurrency > 0 {
			current := atomic.LoadInt32(&health.ActiveConcurrency)
			if current >= int32(health.MaxConcurrency) {
				busyCount++
			}
		}
	}
	allBusy := healthyCount > 0 && busyCount == healthyCount

	if allBusy {
		if lookupModel != requestedModel {
			logger.Infof("tryGetBackend: all backends for fallback model %s are at capacity", lookupModel)
		} else {
			logger.Infof("tryGetBackend: all backends for model %s are at capacity", lookupModel)
		}
		return nil, requestedModel, false
	}

	// 所有后端都不健康，降级使用第一个（仍获取并发许可以保持计数准确）
	if health, exists := lb.health[backends[0].ID]; exists {
		atomic.AddInt32(&health.ActiveConcurrency, 1)
	}
	if lookupModel != requestedModel {
		logger.Infof("tryGetBackend: all %d backends for fallback model %s are unhealthy, using first backend", len(backends), lookupModel)
	} else {
		logger.Infof("tryGetBackend: all %d backends for model %s are unhealthy, using first backend", len(backends), lookupModel)
	}
	return &backends[0], lookupModel, true
}

func (lb *RoundRobinBalancer) MarkFailed(backendID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if health, exists := lb.health[backendID]; exists {
		health.FailCount++
		// 连续失败 3 次才标记为不健康（与健康检查阈值一致）
		if health.FailCount >= 3 {
			health.Healthy = false
		}
	}
}

func (lb *RoundRobinBalancer) MarkSuccess(backendID string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	if health, exists := lb.health[backendID]; exists {
		health.Healthy = true
		health.FailCount = 0
	}
}

// AcquireBackend 尝试获取指定后端的并发许可（原子递增）
// 返回 false 表示该后端并发已达上限
func (lb *RoundRobinBalancer) AcquireBackend(backendID string) bool {
	lb.mu.RLock()
	health, exists := lb.health[backendID]
	lb.mu.RUnlock()

	if !exists {
		return true // 未知后端不限制
	}

	if health.MaxConcurrency <= 0 {
		// 不限制，只计数
		atomic.AddInt32(&health.ActiveConcurrency, 1)
		return true
	}

	// CAS 循环尝试递增，确保不超限
	for {
		current := atomic.LoadInt32(&health.ActiveConcurrency)
		if current >= int32(health.MaxConcurrency) {
			return false
		}
		if atomic.CompareAndSwapInt32(&health.ActiveConcurrency, current, current+1) {
			return true
		}
	}
}

// ReleaseBackend 释放指定后端的并发许可（原子递减）
func (lb *RoundRobinBalancer) ReleaseBackend(backendID string) {
	lb.mu.RLock()
	health, exists := lb.health[backendID]
	lb.mu.RUnlock()

	if !exists {
		return
	}

	for {
		current := atomic.LoadInt32(&health.ActiveConcurrency)
		if current <= 0 {
			return
		}
		if atomic.CompareAndSwapInt32(&health.ActiveConcurrency, current, current-1) {
			return
		}
	}
}

func (lb *RoundRobinBalancer) GetHealthyBackends(modelID string) []*Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var healthy []*Backend
	for i := range lb.backends[modelID] {
		backend := &lb.backends[modelID][i]
		if health, ok := lb.health[backend.ID]; ok && health.Healthy {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}

// GetHealthStatus 获取所有后端的健康状态
func (lb *RoundRobinBalancer) GetHealthStatus() map[string]BackendHealth {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	status := make(map[string]BackendHealth)
	for id, health := range lb.health {
		status[id] = BackendHealth{
			BackendID:         health.BackendID,
			URL:               health.URL,
			ModelName:         health.ModelName,
			Healthy:           health.Healthy,
			LastCheck:         health.LastCheck,
			FailCount:         health.FailCount,
			Latency:           health.Latency,
			MaxConcurrency:    health.MaxConcurrency,
			ActiveConcurrency: atomic.LoadInt32(&health.ActiveConcurrency),
		}
	}
	return status
}

// CheckHealth 检查单个后端的健康状态
func (lb *RoundRobinBalancer) CheckHealth(backendID string) bool {
	lb.mu.RLock()
	backendHealth, exists := lb.health[backendID]
	if !exists {
		lb.mu.RUnlock()
		return false
	}
	backendURL := backendHealth.URL
	lb.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 构造健康检查 URL（尝试 /health 端点，如果不存在则用 /v1/models）
	healthURL := strings.TrimSuffix(backendURL, "/") + "/health"

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false
	}

	resp, err := lb.httpClient.Do(req)
	latency := time.Since(start).Milliseconds()

	lb.mu.Lock()
	defer lb.mu.Unlock()

	if health, exists := lb.health[backendID]; exists {
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

	// 收集所有唯一的后端 ID
	backendIDs := make([]string, 0, len(lb.health))
	for id := range lb.health {
		backendIDs = append(backendIDs, id)
	}
	lb.mu.RUnlock()

	// 并行检查所有后端
	var wg sync.WaitGroup
	for _, id := range backendIDs {
		wg.Add(1)
		go func(backendID string) {
			defer wg.Done()
			lb.CheckHealth(backendID)
		}(id)
	}
	wg.Wait()
}

// GetModelBackends 获取指定模型的所有后端
func (lb *RoundRobinBalancer) GetModelBackends(modelID string) []BackendHealth {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	var result []BackendHealth
	for _, backend := range lb.backends[modelID] {
		if health, exists := lb.health[backend.ID]; exists {
			result = append(result, BackendHealth{
				BackendID:         health.BackendID,
				URL:               health.URL,
				ModelName:         health.ModelName,
				Healthy:           health.Healthy,
				LastCheck:         health.LastCheck,
				FailCount:         health.FailCount,
				Latency:           health.Latency,
				MaxConcurrency:    health.MaxConcurrency,
				ActiveConcurrency: atomic.LoadInt32(&health.ActiveConcurrency),
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

// ReloadConfig 热重载配置 - 在运行时更新后端配置
func (lb *RoundRobinBalancer) ReloadConfig(models []config.ModelConfig) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	logger.Infof("ReloadConfig: loading %d models", len(models))

	// 1. 构建新的后端映射
	newBackends := make(map[string][]Backend)

	// 2. 遍历所有模型配置
	for _, modelConfig := range models {
		modelID := modelConfig.ID

		// 跳过禁用的模型
		if !modelConfig.Enabled {
			logger.Infof("ReloadConfig: skipping disabled model %s", modelID)
			continue
		}

		// 3. 处理该模型的后端
		var modelBackends []Backend
		for _, backendConfig := range modelConfig.Backends {
			// 跳过禁用的后端
			if !backendConfig.Enabled {
				logger.Infof("ReloadConfig: model %s - skipping disabled backend %s", modelID, backendConfig.ID)
				continue
			}

			backend := Backend{
				ID:             backendConfig.ID,
				URL:            backendConfig.BaseURL,
				Weight:         backendConfig.Weight,
				ModelName:      backendConfig.ModelName,
				APIKey:         backendConfig.APIKey,
				MaxConcurrency: backendConfig.MaxConcurrency,
			}

			if backend.Weight == 0 {
				backend.Weight = 1
			}

			modelBackends = append(modelBackends, backend)
			logger.Infof("ReloadConfig: model %s - added backend %s (url=%s)", modelID, backend.ID, backend.URL)

			// 4. 保留现有健康状态或初始化新的
			if _, exists := lb.health[backend.ID]; !exists {
				lb.health[backend.ID] = &BackendHealth{
					BackendID:      backend.ID,
					URL:            backend.URL,
					ModelName:      backend.ModelName,
					Healthy:        true,
					LastCheck:      time.Now(),
					MaxConcurrency: backend.MaxConcurrency,
				}
			} else {
				// 更新URL、ModelName和MaxConcurrency（可能已更改）
				lb.health[backend.ID].URL = backend.URL
				lb.health[backend.ID].ModelName = backend.ModelName
				lb.health[backend.ID].MaxConcurrency = backend.MaxConcurrency
			}
		}

		// 5. 只添加有后端的模型
		if len(modelBackends) > 0 {
			newBackends[modelID] = modelBackends

			logger.Infof("ReloadConfig: model %s registered with %d backends", modelID, len(modelBackends))
		} else {
			logger.Infof("ReloadConfig: WARNING - model %s has no enabled backends!", modelID)
		}
	}

	// 7. 清理已删除后端的健康状态
	existingBackendIDs := make(map[string]bool)
	for _, backends := range newBackends {
		for _, backend := range backends {
			existingBackendIDs[backend.ID] = true
		}
	}

	for id := range lb.health {
		if !existingBackendIDs[id] {
			delete(lb.health, id)
		}
	}

	// 8. 更新后端映射
	lb.backends = newBackends

	logger.Infof("LoadBalancer config reloaded: %d models configured with backends", len(newBackends))
	for modelID, backends := range newBackends {
		logger.Infof("  - %s: %d backends", modelID, len(backends))
	}
}
