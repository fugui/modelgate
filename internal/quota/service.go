package quota

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"llmgate/internal/models"
)

// RateCounter 内存速率计数器
type RateCounter struct {
	counts    map[string]int
	windows   map[string]time.Time
	windowSec int           // 窗口大小（秒）
	mu        sync.RWMutex
}

func NewRateCounter() *RateCounter {
	rc := &RateCounter{
		counts:    make(map[string]int),
		windows:   make(map[string]time.Time),
		windowSec: 60, // 默认 60 秒窗口
	}
	// 启动每分钟清理任务
	go rc.cleanupLoop()
	return rc
}

// cleanupLoop 每分钟清理过期的计数器
func (rc *RateCounter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rc.cleanup()
	}
}

// cleanup 清理过期的计数器条目
func (rc *RateCounter) cleanup() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	for key, window := range rc.windows {
		if now.Sub(window) >= time.Duration(rc.windowSec)*time.Second {
			delete(rc.counts, key)
			delete(rc.windows, key)
		}
	}
}

// getWindowKey 获取当前窗口的 key
// 使用窗口起始时间作为 key，确保同一窗口内的请求使用相同的 key
func (rc *RateCounter) getWindowKey(userID string, window int) string {
	now := time.Now()
	// 计算窗口起始时间
	windowStart := now.Truncate(time.Duration(window) * time.Second)
	return fmt.Sprintf("%s:%d", userID, windowStart.Unix())
}

// Increment 增加计数并返回当前计数
func (rc *RateCounter) Increment(userID string, window int) int {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	key := rc.getWindowKey(userID, window)
	
	// 检查是否需要重置窗口
	if lastWindow, exists := rc.windows[key]; !exists || time.Since(lastWindow) >= time.Duration(window)*time.Second {
		rc.counts[key] = 0
	}
	
	rc.windows[key] = time.Now()
	rc.counts[key]++
	return rc.counts[key]
}

// GetCount 获取当前计数
func (rc *RateCounter) GetCount(userID string, window int) int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	key := rc.getWindowKey(userID, window)
	return rc.counts[key]
}

type Service struct {
	store           *models.QuotaStore
	modelStore      *models.ModelStore
	rateCounter     *RateCounter
	dailyUsageCache *DailyUsageCounter
}

// DailyUsageCounter 每日用量内存计数器
type DailyUsageCounter struct {
	counts map[string]int64 // key: user_id:date
	mu     sync.RWMutex
}

func NewDailyUsageCounter() *DailyUsageCounter {
	return &DailyUsageCounter{
		counts: make(map[string]int64),
	}
}

// Get 获取用户当日用量
func (c *DailyUsageCounter) Get(userID string, date time.Time) (int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", userID, date.Format("2006-01-02"))
	val, exists := c.counts[key]
	return val, exists
}

// Add 增加用户当日用量
func (c *DailyUsageCounter) Add(userID string, date time.Time, tokens int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%s", userID, date.Format("2006-01-02"))
	c.counts[key] += tokens
}

// Set 设置用户当日用量
func (c *DailyUsageCounter) Set(userID string, date time.Time, tokens int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := fmt.Sprintf("%s:%s", userID, date.Format("2006-01-02"))
	c.counts[key] = tokens
}

// CleanupExpired 清理过期缓存（非当日）
func (c *DailyUsageCounter) CleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	for key := range c.counts {
		// key 格式: user_id:date，检查 date 部分
		parts := strings.Split(key, ":")
		if len(parts) == 2 && parts[1] != today {
			delete(c.counts, key)
		}
	}
}

func NewService(store *models.QuotaStore, modelStore *models.ModelStore) *Service {
	s := &Service{
		store:           store,
		modelStore:      modelStore,
		rateCounter:     NewRateCounter(),
		dailyUsageCache: NewDailyUsageCounter(),
	}
	// 启动每日清理任务
	go s.dailyCleanupLoop()
	return s
}

// dailyCleanupLoop 每日清理过期缓存
func (s *Service) dailyCleanupLoop() {
	// 每小时检查一次
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.dailyUsageCache.CleanupExpired()
	}
}

// CheckQuota 检查用户配额
func (s *Service) CheckQuota(userID uuid.UUID, policyName string, modelID string) (*models.QuotaCheckResult, error) {
	result := &models.QuotaCheckResult{
		Allowed: true,
	}

	// 如果未指定策略，使用 default
	if policyName == "" {
		policyName = "default"
	}

	// 获取用户配额策略
	policy, err := s.store.GetPolicy(policyName)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("policy not found: %s", policyName)
	}

	// 检查模型权限
	hasModelAccess := false
	for _, m := range policy.Models {
		if m == "*" || m == modelID {
			hasModelAccess = true
			break
		}
	}
	if !hasModelAccess {
		result.Allowed = false
		result.Reason = "model not allowed"
		return result, nil
	}

	// 检查速率限制
	current := s.rateCounter.GetCount(userID.String(), policy.RateLimitWindow)

	if current >= policy.RateLimit {
		result.Allowed = false
		result.Reason = "rate limit exceeded"
		result.RateLimit = policy.RateLimit
		result.RateRemaining = 0
		return result, nil
	}

	result.RateLimit = policy.RateLimit
	result.RateRemaining = policy.RateLimit - current - 1

	// 检查 Token 配额
	today := time.Now()
	dailyTokens, err := s.getDailyUsageWithCache(userID, today)
	if err != nil {
		return nil, err
	}

	result.DailyTokens = dailyTokens
	result.DailyLimit = policy.TokenQuotaDaily

	if dailyTokens >= policy.TokenQuotaDaily {
		result.Allowed = false
		result.Reason = "daily token quota exceeded"
		return result, nil
	}

	return result, nil
}

// getDailyUsageWithCache 获取每日用量（带缓存）
func (s *Service) getDailyUsageWithCache(userID uuid.UUID, date time.Time) (int64, error) {
	// 1. 先查缓存
	if cached, exists := s.dailyUsageCache.Get(userID.String(), date); exists {
		return cached, nil
	}

	// 2. 缓存未命中，查数据库
	dailyTokens, err := s.store.GetDailyUsage(userID, date)
	if err != nil {
		return 0, err
	}

	// 3. 写入缓存
	s.dailyUsageCache.Set(userID.String(), date, dailyTokens)
	return dailyTokens, nil
}

// IncrementRate 增加速率计数
func (s *Service) IncrementRate(userID uuid.UUID, window int) error {
	s.rateCounter.Increment(userID.String(), window)
	return nil
}

// DeductQuota 扣除配额
func (s *Service) DeductQuota(userID uuid.UUID, policyName string, modelID string, inputTokens, outputTokens int) error {
	// 如果未指定策略，使用 default
	if policyName == "" {
		policyName = "default"
	}

	// 获取策略以使用正确的窗口大小
	policy, err := s.store.GetPolicy(policyName)
	if err != nil {
		return err
	}
	window := 60
	if policy != nil && policy.RateLimitWindow > 0 {
		window = policy.RateLimitWindow
	}

	// 增加速率计数
	if err := s.IncrementRate(userID, window); err != nil {
		return err
	}

	// 增加 Token 使用统计
	err = s.store.IncrementUsage(userID, modelID, inputTokens, outputTokens)
	if err != nil {
		return err
	}

	// 更新内存缓存
	totalTokens := int64(inputTokens + outputTokens)
	s.dailyUsageCache.Add(userID.String(), time.Now(), totalTokens)

	return nil
}

// GetQuotaStats 获取配额统计
func (s *Service) GetQuotaStats(userID uuid.UUID, policyName string) (map[string]interface{}, error) {
	// 如果未指定策略，使用 default
	if policyName == "" {
		policyName = "default"
	}

	policy, err := s.store.GetPolicy(policyName)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, fmt.Errorf("policy not found: %s", policyName)
	}

	// 使用缓存获取每日用量
	dailyTokens, err := s.getDailyUsageWithCache(userID, time.Now())
	if err != nil {
		return nil, err
	}

	// 处理 models_allowed：如果包含 *，返回所有模型名称列表
	var modelsAllowed []string
	if len(policy.Models) == 1 && policy.Models[0] == "*" {
		// 获取所有启用的模型
		models, err := s.modelStore.ListEnabled()
		if err != nil {
			return nil, err
		}
		for _, m := range models {
			modelsAllowed = append(modelsAllowed, m.ID)
		}
	} else {
		modelsAllowed = policy.Models
	}

	return map[string]interface{}{
		"daily_tokens_used":  dailyTokens,
		"daily_tokens_limit": policy.TokenQuotaDaily,
		"rate_limit":         policy.RateLimit,
		"rate_window":        policy.RateLimitWindow,
		"models_allowed":     modelsAllowed,
		"reset_time":         "00:00",
	}, nil
}
