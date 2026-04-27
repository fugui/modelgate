package config

import (
	"fmt"
	"os"
	"time"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

var validate = validator.New()

type Config struct {
	Server      ServerConfig      `yaml:"server" validate:"required"`
	Database    DatabaseConfig    `yaml:"database" validate:"required"`
	JWT         JWTConfig         `yaml:"jwt" validate:"required"`
	Models      []ModelConfig     `yaml:"models" validate:"dive"`
	Policies    []PolicyConfig    `yaml:"quota_policies" validate:"dive"`
	Admin       AdminConfig       `yaml:"admin"`
	Logs        LogConfig         `yaml:"logs"`
	Frontend    FrontendConfig    `yaml:"frontend"`
	Concurrency ConcurrencyConfig `yaml:"concurrency"`
	SSO         SSOConfig         `yaml:"sso"`
}

type ServerConfig struct {
	Port            int           `yaml:"port" validate:"required,min=1,max=65535"`
	Mode            string        `yaml:"mode" validate:"oneof=debug release test"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	IdleTimeout     time.Duration `yaml:"idle_timeout"`
	MaxHeaderBytes  int           `yaml:"max_header_bytes"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type DatabaseConfig struct {
	Path string `yaml:"path" validate:"required"`
}

type JWTConfig struct {
	Secret      string `yaml:"secret" validate:"required,min=8"`
	ExpireHours int    `yaml:"expire_hours" validate:"required,min=1"`
}

type LogConfig struct {
	Path             string `yaml:"path"`
	RetentionDays    int    `yaml:"retention_days" validate:"min=0"`
	LogPayloads      bool   `yaml:"log_payloads"`
	DebugRawPayloads string `yaml:"debug_raw_payloads" validate:"oneof=none error full"`
}

type ModelConfig struct {
	ID            string                 `yaml:"id" validate:"required"`
	Name          string                 `yaml:"name" validate:"required"`
	Description   string                 `yaml:"description"`
	Enabled       bool                   `yaml:"enabled"`
	ContextWindow int                    `yaml:"context_window" validate:"min=0"`
	ModelParams   map[string]interface{} `yaml:"model_params"`
	Backends      []BackendConfig        `yaml:"backends" validate:"dive"`
}

type BackendConfig struct {
	ID        string `yaml:"id" validate:"required"`
	BaseURL   string `yaml:"base_url" validate:"required,url"`
	APIKey    string `yaml:"api_key"`
	ModelName string `yaml:"model_name"`
	Weight    int    `yaml:"weight" validate:"min=0"`
	Enabled   bool   `yaml:"enabled"`
}

type TimeRangeConfig struct {
	Start string `yaml:"start" validate:"omitempty,datetime=15:04"` // "HH:MM" 格式
	End   string `yaml:"end" validate:"omitempty,datetime=15:04"`   // "HH:MM" 格式
}

type PolicyConfig struct {
	Name                string            `yaml:"name" validate:"required"`
	RateLimit           int               `yaml:"rate_limit" validate:"min=0"`
	RateLimitWindow     int               `yaml:"rate_limit_window" validate:"min=0"`
	RequestQuotaDaily   int               `yaml:"request_quota_daily" validate:"min=0"`
	AvailableTimeRanges []TimeRangeConfig `yaml:"available_time_ranges" validate:"dive"`
	Models              []string          `yaml:"models"`
	Description         string            `yaml:"description"`
	DefaultModel        string            `yaml:"default_model"`
}

type AdminConfig struct {
	DefaultEmail    string `yaml:"default_email" validate:"omitempty,email"`
	DefaultPassword string `yaml:"default_password" validate:"omitempty,min=6"`
}

type FrontendConfig struct {
	FeedbackURL         string `yaml:"feedback_url" json:"feedback_url" validate:"omitempty,url"`
	DevManualURL        string `yaml:"dev_manual_url" json:"dev_manual_url" validate:"omitempty,url"`
	RegistrationEnabled bool   `yaml:"registration_enabled" json:"registration_enabled"`
}

type SSOConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Provider     string `yaml:"provider" validate:"required_if=Enabled true"`
	ClientID     string `yaml:"client_id" validate:"required_if=Enabled true"`
	ClientSecret string `yaml:"client_secret" validate:"required_if=Enabled true"`
	IssuerURL    string `yaml:"issuer_url" validate:"required_if=Enabled true"`
	EmailClaim   string `yaml:"email_claim"`
}

func (s SSOConfig) GetAuthorizeURL() string {
	if s.Provider == "azure" {
		return s.IssuerURL + "/oauth2/v2.0/authorize"
	}
	return s.IssuerURL + "/authorize"
}

func (s SSOConfig) GetTokenURL() string {
	if s.Provider == "azure" {
		return s.IssuerURL + "/oauth2/v2.0/token"
	}
	return s.IssuerURL + "/token"
}

type ConcurrencyConfig struct {
	GlobalLimit int `yaml:"global_limit" validate:"min=0"`
	UserLimit   int `yaml:"user_limit" validate:"min=0"`
}

// Validate 校验配置
func (c *Config) Validate() error {
	return validate.Struct(c)
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
	setDefaults(&cfg)

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func setDefaults(cfg *Config) {
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
		cfg.Server.WriteTimeout = 30 * time.Minute
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
	if cfg.Logs.DebugRawPayloads == "" {
		cfg.Logs.DebugRawPayloads = "none"
	}
	if cfg.SSO.Enabled && cfg.SSO.EmailClaim == "" {
		cfg.SSO.EmailClaim = "email"
	}
}
