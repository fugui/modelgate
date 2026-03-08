package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modelgate/internal/entity"
)

// TestScenario_QuotaExceedsDailyLimit
// 场景：用户超出日配额后被拒绝
// Given: 用户日配额 1000 tokens，已使用 900
// When: 发起消耗 200 tokens 的请求
// Then: 请求被拒绝，返回配额超限原因
func TestScenario_QuotaExceedsDailyLimit(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// 创建用户
	user := scenario.CreateUser(t, "quotauser@example.com", "Quota User", entity.RoleUser)

	// 先消耗 900 tokens（模拟已使用）
	for i := 0; i < 9; i++ {
		err := scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 100, 0)
		require.NoError(t, err)
	}

	// 检查当前配额
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.True(t, result.Allowed, "900/1000 应该还允许")
	assert.Equal(t, int64(900), result.DailyTokens)
	t.Logf("✓ 当前配额: %d/%d tokens，请求仍被允许", result.DailyTokens, result.DailyLimit)

	// 再消耗 100 tokens，达到 1000
	err = scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 50, 50)
	require.NoError(t, err)

	// 此时配额已满
	result, err = scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.False(t, result.Allowed, "1000/1000 应该被拒绝")
	assert.Equal(t, "daily token quota exceeded", result.Reason)
	t.Logf("✓ 配额超限被拒绝: %s", result.Reason)
}

// TestScenario_QuotaMultipleModels
// 场景：多模型配额独立计算
// Given: 用户调用 model A 消耗 tokens
// When: 检查 model B 的配额
// Then: model B 的配额不受 model A 影响（实际应该按用户总配额）
func TestScenario_QuotaMultipleModels(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "multi@example.com", "Multi Model User", entity.RoleUser)

	// 在模型 A 上消耗 tokens
	err := scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 300, 100)
	require.NoError(t, err)

	// 检查模型 B 的配额（应该共享同一配额）
	result, err := scenario.QuotaSvc.CheckQuota(user.ID, "qwen-72b")
	require.NoError(t, err)
	
	// 注意：当前实现是用户级配额，不是模型级
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(400), result.DailyTokens) // 300 + 100
	t.Logf("✓ 多模型共享配额: 总共使用 %d tokens", result.DailyTokens)
}
