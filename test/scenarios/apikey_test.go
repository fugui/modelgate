package scenarios

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"modelgate/internal/repository"
)

// TestScenario_APIKeyExpires
// 场景：API Key 过期后无法使用
// Given: 创建一个 1 秒后过期的 API Key
// When: 等待过期后尝试验证
// Then: 验证失败，返回过期错误
func TestScenario_APIKeyExpires(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "expire@example.com", "Expire User", entity.RoleUser)

	// 创建 2秒后过期的 Key（要考虑 bcrypt 验证耗时）
	expiresAt := time.Now().Add(2 * time.Second)
	keyReq := &entity.APIKeyCreateRequest{
		Name:      "临时Key",
		ExpiresAt: &expiresAt,
	}
	key, err := scenario.APIKeySvc.GenerateKey(user.ID, keyReq)
	require.NoError(t, err)
	t.Logf("✓ 创建临时 Key，过期时间: %v", expiresAt)

	// 过期前验证应该成功
	_, _, err = scenario.APIKeySvc.ValidateKey(key.Key)
	require.NoError(t, err)
	t.Logf("✓ 过期前验证通过")

	// 等待过期（2.1秒）
	time.Sleep(2100 * time.Millisecond)

	// 过期后验证应该失败
	_, _, err = scenario.APIKeySvc.ValidateKey(key.Key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
	t.Logf("✓ 过期后验证失败: %v", err)
}

// TestScenario_APIKeyDisabled
// 场景：管理员禁用 Key 后立即失效
// Given: 用户创建了 API Key
// When: 管理员禁用该 Key
// Then: 用户无法再使用该 Key
func TestScenario_APIKeyDisabled(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "disable@example.com", "Disable User", entity.RoleUser)

	// 创建 Key
	keyReq := &entity.APIKeyCreateRequest{Name: "将被禁用的Key"}
	key, err := scenario.APIKeySvc.GenerateKey(user.ID, keyReq)
	require.NoError(t, err)

	// 验证成功
	validatedKey, _, err := scenario.APIKeySvc.ValidateKey(key.Key)
	require.NoError(t, err)
	t.Logf("✓ Key 初始状态有效")

	// 禁用 Key
	validatedKey.Enabled = false
	err = scenario.APIKeyStore.Update(validatedKey)
	require.NoError(t, err)
	t.Logf("✓ Key 已被禁用")

	// 再次验证应该失败
	_, _, err = scenario.APIKeySvc.ValidateKey(key.Key)
	assert.Error(t, err)
	t.Logf("✓ 禁用后验证失败")
}

// TestScenario_UserDisabledKeysInvalid
// 场景：用户被禁用后，其所有 Key 失效
// Given: 用户有多个 API Key
// When: 管理员禁用用户账号
// Then: 该用户的所有 Key 都无法验证通过
func TestScenario_UserDisabledKeysInvalid(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	user := scenario.CreateUser(t, "banned@example.com", "Banned User", entity.RoleUser)

	// 创建两个 Key
	key1, _ := scenario.APIKeySvc.GenerateKey(user.ID, &entity.APIKeyCreateRequest{Name: "Key1"})
	key2, _ := scenario.APIKeySvc.GenerateKey(user.ID, &entity.APIKeyCreateRequest{Name: "Key2"})

	// 验证两个 Key 都有效
	_, _, err := scenario.APIKeySvc.ValidateKey(key1.Key)
	require.NoError(t, err)
	_, _, err = scenario.APIKeySvc.ValidateKey(key2.Key)
	require.NoError(t, err)
	t.Logf("✓ 两个 Key 初始都有效")

	// 禁用用户
	user.Enabled = false
	err = scenario.UserStore.Update(user)
	require.NoError(t, err)
	t.Logf("✓ 用户已被禁用")

	// Key 验证应该失败（修复后的行为）
	_, _, err = scenario.APIKeySvc.ValidateKey(key1.Key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user disabled")
	t.Logf("✓ 用户禁用后 Key1 验证失败: %v", err)

	_, _, err = scenario.APIKeySvc.ValidateKey(key2.Key)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user disabled")
	t.Logf("✓ 用户禁用后 Key2 验证失败: %v", err)
}

// TestScenario_InvalidAPIKeyFormat
// 场景：非法格式的 API Key 被拒绝
// Given: 用户提供了一个格式错误的 Key
// When: 尝试验证
// Then: 立即返回格式错误
func TestScenario_InvalidAPIKeyFormat(t *testing.T) {
	scenario := SetupTestDB(t)
	defer scenario.Cleanup()
	scenario.InitServices()

	// 测试各种非法格式
	invalidKeys := []string{
		"invalid-key",
		"",
		"llm-", // 前缀正确但太短
		"wrong-prefix-abc123",
	}

	for _, key := range invalidKeys {
		_, _, err := scenario.APIKeySvc.ValidateKey(key)
		assert.Error(t, err, "Key '%s' 应该被拒绝", key)
	}
	t.Logf("✓ 所有非法格式 Key 都被拒绝")
}
