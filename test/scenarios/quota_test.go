package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modelgate/internal/repository"
)

// TestScenario_QuotaExceedsDailyLimit
// 场景：用户超出日配额后被拒绝
// Given: 用户日配额 1000 请求，已使用 900
// When: 发起第 901-1000 个请求
// Then: 请求被允许，直到超过 1000
func TestScenario_QuotaExceedsDailyLimit(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// 创建用户
	user := scenario.CreateUser(t, "quotauser@example.com", "Quota User", entity.RoleUser)

	// 先消耗 900 个请求（模拟已使用）
	for i := 0; i < 900; i++ {
		err := scenario.QuotaSvc.RecordRequest(user.ID, "llama3-70b", 0)
		require.NoError(t, err)
	}

	// 检查当前配额
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "default", "llama3-70b")
	require.NoError(t, err)
	assert.True(t, result.Allowed, "900/1000 应该还允许")
	assert.Equal(t, 900, result.DailyRequests)
	t.Logf("✓ 当前配额: %d/%d 请求，请求仍被允许", result.DailyRequests, result.DailyRequestLimit)

	// 再消耗 100 个请求，达到 1000
	for i := 0; i < 100; i++ {
		err = scenario.QuotaSvc.RecordRequest(user.ID, "llama3-70b", 0)
		require.NoError(t, err)
	}

	// 此时配额已满
	result, err = scenario.QuotaSvc.CheckQuota(user.ID, "default", "llama3-70b")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "1000/1000 应该被拒绝")
	assert.Equal(t, "daily request quota exceeded", result.Reason)
	t.Logf("✓ 配额超限被拒绝: %s", result.Reason)
}

// TestScenario_QuotaMultipleModels
// 场景：多模型配额共享计算（按用户总配额）
// Given: 用户调用 model A 消耗请求
// When: 检查 model B 的配额
// Then: model B 的配额与 model A 共享同一用户级配额
func TestScenario_QuotaMultipleModels(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "multi@example.com", "Multi Model User", entity.RoleUser)

	// 在模型 A 上消耗 4 个请求
	for i := 0; i < 4; i++ {
		err := scenario.QuotaSvc.RecordRequest(user.ID, "llama3-70b", 0)
		require.NoError(t, err)
	}

	// 检查模型 B 的配额（应该共享同一配额）
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "default", "qwen-72b")
	require.NoError(t, err)

	// 注意：当前实现是用户级配额，不是模型级
	assert.True(t, result.Allowed)
	assert.Equal(t, 4, result.DailyRequests)
	t.Logf("✓ 多模型共享配额: 总共使用 %d 请求", result.DailyRequests)
}
