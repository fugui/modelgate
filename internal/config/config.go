package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig      `yaml:"server"`
	Database     DatabaseConfig    `yaml:"database"`
	JWT          JWTConfig         `yaml:"jwt"`
	Models       []ModelConfig     `yaml:"models"`
	Policies     []PolicyConfig    `yaml:"quota_policies"`
	Admin        AdminConfig       `yaml:"admin"`
	Logs         LogConfig         `yaml:"logs"`
	Frontend     FrontendConfig    `yaml:"frontend"`
	Concurrency  ConcurrencyConfig `yaml:"concurrency"`
	SSO          SSOConfig         `yaml:"sso"`
	DefaultModel string            `yaml:"default_model"`
}

type ServerConfig struct {
	Port            int           `yaml:"port"`
	Mode            string        `yaml:"mode"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes  int           `yaml:"max_header_bytes"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret"`
	ExpireHours int    `yaml:"expire_hours"`
}

type LogConfig struct {
	Path          string `yaml:"path"`
	RetentionDays int    `yaml:"retention_days"`
}

type ModelConfig struct {
	ID          string                 `yaml:"id"`
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Enabled     bool                   `yaml:"enabled"`
	ModelParams map[string]interface{} `yaml:"model_params"`
	Backends    []BackendConfig        `yaml:"backends"`
}

type BackendConfig struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	BaseURL   string `yaml:"base_url"`
	APIKey    string `yaml:"api_key"`
	ModelName string `yaml:"model_name"`
	Weight    int    `yaml:"weight"`
	Region    string `yaml:"region"`
	Enabled   bool   `yaml:"enabled"`
}

type PolicyConfig struct {
	Name              string   `yaml:"name"`
	RateLimit         int      `yaml:"rate_limit"`
	RateLimitWindow   int      `yaml:"rate_limit_window"`
	RequestQuotaDaily int      `yaml:"request_quota_daily"`
	Models            []string `yaml:"models"`
	Description       string   `yaml:"description"`
}

type AdminConfig struct {
	DefaultEmail    string `yaml:"default_email"`
	DefaultPassword string `yaml:"default_password"`
}

type FrontendConfig struct {
	FeedbackURL  string `yaml:"feedback_url"`
	DevManualURL string `yaml:"dev_manual_url"`
}

// SSOConfig 企业 SSO 配置
type SSOConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Provider     string `yaml:"provider"`
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	IssuerURL    string `yaml:"issuer_url"`
	EmailClaim   string `yaml:"email_claim"`
}

// GetAuthorizeURL 获取授权端点
func (s SSOConfig) GetAuthorizeURL() string {
	if s.Provider == "azure" {
		return s.IssuerURL + "/oauth2/v2.0/authorize"
	}
	// 通用 OIDC
	return s.IssuerURL + "/authorize"
}

// GetTokenURL 获取 Token 端点
func (s SSOConfig) GetTokenURL() string {
	if s.Provider == "azure" {
		return s.IssuerURL + "/oauth2/v2.0/token"
	}
	return s.IssuerURL + "/token"
}

type ConcurrencyConfig struct {
	GlobalLimit int `yaml:"global_limit"` // 全局并发限制，0 表示不限制
	UserLimit   int `yaml:"user_limit"`   // 每个用户并发限制，0 表示不限制
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "release"
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 60 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 120 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 300 * time.Second
	}
	if cfg.Server.MaxHeaderBytes == 0 {
		cfg.Server.MaxHeaderBytes = 1 << 20 // 1MB
	}
	if cfg.Server.ShutdownTimeout == 0 {
		cfg.Server.ShutdownTimeout = 30 * time.Second
	}
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = "default-secret-change-in-production"
	}
	if cfg.JWT.ExpireHours == 0 {
		cfg.JWT.ExpireHours = 24
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "modelgate.db"
	}
	if cfg.Logs.Path == "" {
		cfg.Logs.Path = "logs"
	}
	if cfg.Logs.RetentionDays == 0 {
		cfg.Logs.RetentionDays = 7
	}

	// SSO 默认值
	if cfg.SSO.Enabled && cfg.SSO.EmailClaim == "" {
		cfg.SSO.EmailClaim = "email"
	}

	return &cfg, nil
}
