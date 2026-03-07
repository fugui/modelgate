package scenarios

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"modelgate/internal/apikey"
	"modelgate/internal/auth"
	"modelgate/internal/cache"
	"modelgate/internal/models"
	"modelgate/internal/quota"

	_ "github.com/mattn/go-sqlite3"
)

// TestScenario 测试场景基座
type TestScenario struct {
	DB          *sql.DB
	UserStore   *models.UserStore
	APIKeyStore *models.APIKeyStore
	ModelStore  *models.ModelStore
	QuotaStore  *models.QuotaStore
	JWTManager  *auth.JWTManager
	Cache       *cache.Cache
	APIKeySvc   *apikey.Service
	QuotaSvc    *quota.Service
}

// SetupTestDB 创建内存测试数据库
func SetupTestDB(t *testing.T) *TestScenario {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	// 创建表结构
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

CREATE TABLE quota_policies (
    name TEXT PRIMARY KEY,
    rate_limit INTEGER NOT NULL,
    rate_limit_window INTEGER NOT NULL,
    request_quota_daily INTEGER DEFAULT 1000,
    models TEXT,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE models (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE backends (
    id TEXT PRIMARY KEY,
    model_id TEXT NOT NULL,
    name TEXT,
    base_url TEXT NOT NULL,
    api_key TEXT,
    model_name TEXT,
    weight INTEGER DEFAULT 1,
    region TEXT,
    enabled INTEGER DEFAULT 1,
    healthy INTEGER DEFAULT 1,
    last_check_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (model_id) REFERENCES models(id)
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

	// 创建默认配额策略
	_, err = db.Exec(`
		INSERT INTO quota_policies (name, rate_limit, rate_limit_window, request_quota_daily, models)
		VALUES ('default', 60, 60, 1000, '["*"]')
	`)
	require.NoError(t, err)

	return &TestScenario{
		DB:          db,
		UserStore:   models.NewUserStore(db),
		APIKeyStore: models.NewAPIKeyStore(db),
		ModelStore:  models.NewModelStore(db),
		QuotaStore:  models.NewQuotaStore(db),
		JWTManager:  auth.NewJWTManager("test-secret", 24),
		Cache:       cache.New(),
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
		Models:       []string{"*"},
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
