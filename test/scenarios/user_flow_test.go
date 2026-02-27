package scenarios

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"llmgate/internal/models"
)

// TestScenario_UserEndToEndFlow
// 场景：完整用户流程
// Given: 新用户注册
// When: 用户登录 -> 创建 API Key -> 调用模型 -> 消耗 Token
// Then: 配额正确扣减，使用记录可查
func TestScenario_UserEndToEndFlow(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// Step 1: 创建用户（模拟注册）
	user := scenario.CreateUser(t, "user@example.com", "Test User", models.RoleUser)
	assert.NotEqual(t, uuid.Nil, user.ID)
	t.Logf("✓ 用户创建成功: %s (%s)", user.Email, user.ID)

	// Step 2: 用户登录（生成 JWT）
	token, err := scenario.JWTManager.Generate(user)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	t.Logf("✓ JWT Token 生成成功")

	// Step 3: 验证 JWT
	claims, err := scenario.JWTManager.Validate(token)
	require.NoError(t, err)
	assert.Equal(t, user.ID, claims.UserID)
	t.Logf("✓ JWT 验证通过")

	// Step 4: 创建 API Key
	keyReq := &models.APIKeyCreateRequest{
		Name:            "开发测试",
		Models:          []string{},
		RateLimit:       0,
		RateLimitWindow: 60,
	}
	keyWithSecret, err := scenario.APIKeySvc.GenerateKey(user.ID, keyReq)
	require.NoError(t, err)
	assert.NotEmpty(t, keyWithSecret.Key)
	assert.Equal(t, "开发测试", keyWithSecret.Name)
	t.Logf("✓ API Key 创建成功: %s...", keyWithSecret.Key[:16])

	// Step 5: 验证 API Key
	validatedKey, _, err := scenario.APIKeySvc.ValidateKey(keyWithSecret.Key)
	require.NoError(t, err)
	assert.Equal(t, keyWithSecret.ID, validatedKey.ID)
	t.Logf("✓ API Key 验证通过")

	// Step 6: 检查配额（首次检查，应该通过）
	quotaResult, err := scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.True(t, quotaResult.Allowed, "首次检查应该允许")
	assert.Equal(t, int64(1000), quotaResult.DailyLimit)
	assert.Equal(t, int64(0), quotaResult.DailyTokens)
	t.Logf("✓ 配额检查通过: %d/%d tokens", quotaResult.DailyTokens, quotaResult.DailyLimit)

	// Step 7: 模拟调用模型，扣除配额
	err = scenario.QuotaSvc.DeductQuota(user.ID, "llama3-70b", 100, 50)
	require.NoError(t, err)
	t.Logf("✓ 配额扣减: input=100, output=50")

	// Step 8: 再次检查配额
	quotaResult, err = scenario.QuotaSvc.CheckQuota(user.ID, "llama3-70b")
	require.NoError(t, err)
	assert.True(t, quotaResult.Allowed)
	assert.Equal(t, int64(150), quotaResult.DailyTokens) // 100 + 50
	t.Logf("✓ 配额更新正确: %d/%d tokens", quotaResult.DailyTokens, quotaResult.DailyLimit)

	// Step 9: 获取配额统计
	stats, err := scenario.QuotaSvc.GetQuotaStats(user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(150), stats["daily_tokens_used"])
	assert.Equal(t, int64(1000), stats["daily_tokens_limit"])
	t.Logf("✓ 配额统计正确: used=%d, limit=%d", stats["daily_tokens_used"], stats["daily_tokens_limit"])
}
