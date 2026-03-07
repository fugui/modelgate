package scenarios

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"modelgate/internal/apikey"
	"modelgate/internal/auth"
	"modelgate/internal/cache"
	"modelgate/internal/config"
	"modelgate/internal/models"
	"modelgate/internal/quota"

	_ "github.com/mattn/go-sqlite3"
)

// TestScenario 测试场景基座
type TestScenario struct {
	DB           *sql.DB
	CfgManager   *config.ConfigManager
	UserStore    *models.UserStore
	APIKeyStore  *models.APIKeyStore
	ModelStore   *models.ModelStore
	QuotaStore   *models.QuotaStore
	JWTManager   *auth.JWTManager
	Cache        *cache.Cache
	APIKeySvc    *apikey.Service
	QuotaSvc     *quota.Service
}

// SetupTestDB 创建内存测试数据库和配置
func SetupTestDB(t *testing.T) *TestScenario {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// 创建核心数据表（不再包含 models, backends, quota_policies）
	schema := `
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL,
    department TEXT,
    quota_policy TEXT,
    models TEXT,
    auth_source TEXT DEFAULT 'local',
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME
);

CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    models TEXT,
    rate_limit INTEGER DEFAULT 0,
    rate_limit_window INTEGER DEFAULT 60,
    enabled INTEGER DEFAULT 1,
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id)
);

CREATE TABLE quota_usage_daily (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    date DATE NOT NULL,
    model_id TEXT NOT NULL,
    request_count INTEGER DEFAULT 0,
    UNIQUE(user_id, date, model_id)
);
`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	// 创建测试配置
	testCfg := &config.Config{
		Models: []config.ModelConfig{
			{
				ID:          "gpt-4",
				Name:        "GPT-4",
				Description: "Test model",
				Enabled:     true,
				Backends: []config.BackendConfig{
					{
						ID:        "backend-1",
						Name:      "Test Backend",
						BaseURL:   "http://localhost:8080",
						ModelName: "gpt-4",
						Weight:    1,
						Enabled:   true,
					},
				},
			},
		},
		Policies: []config.PolicyConfig{
			{
				Name:              "default",
				RateLimit:         60,
				RateLimitWindow:   60,
				RequestQuotaDaily: 1000,
				Models:            []string{"*"},
			},
		},
	}

	// 创建 ConfigManager（使用临时路径）
	cfgManager := config.NewManager(testCfg, "/tmp/test-config.yaml")

	return &TestScenario{
		DB:           db,
		CfgManager:   cfgManager,
		UserStore:    models.NewUserStore(db),
		APIKeyStore:  models.NewAPIKeyStore(db),
		ModelStore:   models.NewModelStore(cfgManager),
		QuotaStore:   models.NewQuotaStore(cfgManager, db),
		JWTManager:   auth.NewJWTManager("test-secret", 24),
		Cache:        cache.New(),
	}
}

// InitServices 初始化服务（必须在 SetupTestDB 之后调用）
func (s *TestScenario) InitServices() {
	s.APIKeySvc = apikey.NewService(s.APIKeyStore, s.UserStore, s.Cache)
	s.QuotaSvc = quota.NewService(s.QuotaStore, s.ModelStore)
}

// CreateUser 辅助方法：创建测试用户
func (s *TestScenario) CreateUser(t *testing.T, email, name string, role models.Role) *models.User {
	user := &models.User{
		Email:        email,
		PasswordHash: "$2a$10$test", // 简化处理，实际应正确哈希
		Name:         name,
		Role:         role,
		Department:   "test",
		QuotaPolicy:  "default",
		Enabled:      true,
	}
	err := s.UserStore.Create(user)
	require.NoError(t, err)
	return user
}

// Cleanup 清理资源
func (s *TestScenario) Cleanup() {
	s.Cache.Stop()
	s.DB.Close()
}
