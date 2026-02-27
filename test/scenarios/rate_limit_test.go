package scenarios

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"llmgate/internal/models"
)

// TestScenario_RateLimitExceeded
// 场景：用户超出速率限制后被拒绝
// Given: 用户速率限制 5 req/min
// When: 1 分钟内发起 6 次请求
// Then: 第 6 次请求被拒绝
func TestScenario_RateLimitExceeded(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// 修改默认策略，设置较低的速率限制以便测试
	_, err := scenario.DB.Exec(`
		UPDATE quota_policies SET rate_limit = 5 WHERE name = 'default'
	`)
	require.NoError(t, err)

	user := scenario.CreateUser(t, "ratelimit@example.com", "Rate Limit User", models.RoleUser)

	// 前 5 次请求应该成功
	for i := 0; i < 5; i++ {
		result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
		require.NoError(t, err)
		assert.True(t, result.Allowed, "第 %d 次请求应该被允许", i+1)
		
		// 模拟请求消耗
		err = scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 10, 10)
		require.NoError(t, err)
	}
	t.Logf("✓ 前 5 次请求都成功")

	// 第 6 次应该被拒绝
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "第 6 次请求应该被拒绝")
	assert.Equal(t, "rate limit exceeded", result.Reason)
	t.Logf("✓ 第 6 次请求被拒绝: %s", result.Reason)
}

// TestScenario_ModelAccessControl
// 场景：用户只能访问授权的模型
// Given: 用户只被授权访问 llama3-70b
// When: 尝试访问 qwen-72b
// Then: 请求被拒绝，返回模型未授权
func TestScenario_ModelAccessControl(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// 修改策略，只允许特定模型
	_, err := scenario.DB.Exec(`
		UPDATE quota_policies SET models = '["llama3-70b"]' WHERE name = 'default'
	`)
	require.NoError(t, err)

	user := scenario.CreateUser(t, "limited@example.com", "Limited User", models.RoleUser)

	// 访问授权模型应该成功
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.True(t, result.Allowed, "授权模型应该可以访问")
	t.Logf("✓ 授权模型 llama3-70b 可访问")

	// 访问未授权模型应该被拒绝
	result, err = scenario.QuotaSvc.CheckQuota(user.ID, "qwen-72b")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "未授权模型应该被拒绝")
	assert.Equal(t, "model not allowed", result.Reason)
	t.Logf("✓ 未授权模型 qwen-72b 被拒绝: %s", result.Reason)
}

// TestScenario_ConcurrentRequests
// 场景：并发请求下配额计算正确
// Given: 用户并发发起多个请求
// When: 同时扣除配额
// Then: 最终配额计算准确，无竞态条件
func TestScenario_ConcurrentRequests(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "concurrent@example.com", "Concurrent User", models.RoleUser)

	// 并发 10 个请求，每个消耗 10 tokens
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 5, 5)
			require.NoError(t, err)
		}()
	}
	wg.Wait()

	// 检查最终配额
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.Equal(t, int64(100), result.DailyTokens, "并发请求应该正确累计为 100 tokens")
	t.Logf("✓ 并发请求后配额正确: %d tokens", result.DailyTokens)
}

// TestScenario_TokenQuotaResetNextDay
// 场景：配额次日自动重置
// Given: 用户今日已用完配额
// When: 次日检查配额
// Then: 配额已重置，可以正常请求
// 
// 注意：此测试演示了需求，实际实现可能需要定时任务支持
func TestScenario_TokenQuotaResetNextDay(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "reset@example.com", "Reset User", models.RoleUser)

	// 用完全部配额
	for i := 0; i < 10; i++ {
		err := scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 100, 0)
		require.NoError(t, err)
	}

	// 确认配额已用完
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "今日配额应已用完")
	t.Logf("✓ 今日配额已用完: %d/%d", result.DailyTokens, result.DailyLimit)

	// 注意：实际测试次日重置需要修改系统时间或使用 mock
	// 这里只是演示这个场景的重要性
	t.Logf("⚠ 次日重置测试需要额外机制（如 mock 时间）")
}
