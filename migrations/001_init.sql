-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'manager', 'user')),
    department TEXT,
    quota_policy TEXT NOT NULL DEFAULT 'default',
    models TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_login_at DATETIME,
    enabled BOOLEAN DEFAULT 1
);

-- API Key 表
CREATE TABLE IF NOT EXISTS api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    key_prefix TEXT NOT NULL,
    models TEXT,
    rate_limit INTEGER,
    rate_limit_window INTEGER DEFAULT 60,
    enabled BOOLEAN DEFAULT 1,
    expires_at DATETIME,
    last_used_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 模型配置表
CREATE TABLE IF NOT EXISTS models (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    backend_url TEXT NOT NULL,
    enabled BOOLEAN DEFAULT 1,
    weight INTEGER DEFAULT 1,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 配额策略表
CREATE TABLE IF NOT EXISTS quota_policies (
    name TEXT PRIMARY KEY,
    rate_limit INTEGER NOT NULL DEFAULT 60,
    rate_limit_window INTEGER NOT NULL DEFAULT 60,
    token_quota_daily INTEGER NOT NULL DEFAULT 100000,
    models TEXT,
    description TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 配额使用统计表（按天汇总）
CREATE TABLE IF NOT EXISTS quota_usage_daily (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    date TEXT NOT NULL,
    model_id TEXT,
    request_count INTEGER DEFAULT 0,
    token_count INTEGER DEFAULT 0,
    input_tokens INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    UNIQUE(user_id, date, model_id)
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_enabled ON users(enabled);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_api_keys_enabled ON api_keys(enabled);
CREATE INDEX IF NOT EXISTS idx_quota_usage_user_date ON quota_usage_daily(user_id, date);

-- 插入默认配额策略
INSERT OR IGNORE INTO quota_policies (name, rate_limit, rate_limit_window, token_quota_daily, models, description)
VALUES ('default', 60, 60, 100000, '["llama3-70b", "qwen-72b", "deepseek-67b"]', '默认用户配额');

INSERT OR IGNORE INTO quota_policies (name, rate_limit, rate_limit_window, token_quota_daily, models, description)
VALUES ('vip', 300, 60, 1000000, '["*"]', 'VIP用户配额');

-- 插入默认模型
INSERT OR IGNORE INTO models (id, name, backend_url, enabled, weight, description)
VALUES ('llama3-70b', 'Llama 3 70B', 'http://localhost:8001', 1, 1, 'Llama 3 70B 模型');

INSERT OR IGNORE INTO models (id, name, backend_url, enabled, weight, description)
VALUES ('qwen-72b', 'Qwen 72B', 'http://localhost:8002', 1, 1, 'Qwen 72B 模型');

INSERT OR IGNORE INTO models (id, name, backend_url, enabled, weight, description)
VALUES ('deepseek-67b', 'DeepSeek 67B', 'http://localhost:8003', 1, 1, 'DeepSeek 67B 模型');
