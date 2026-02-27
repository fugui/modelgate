package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Models   []ModelConfig  `yaml:"models"`
	Policies []PolicyConfig `yaml:"quota_policies"`
	Admin    AdminConfig    `yaml:"admin"`
	Logs     LogConfig      `yaml:"logs"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
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
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Backend string `yaml:"backend"`
	Enabled bool   `yaml:"enabled"`
	Weight  int    `yaml:"weight"`
}

type PolicyConfig struct {
	Name            string   `yaml:"name"`
	RateLimit       int      `yaml:"rate_limit"`
	RateLimitWindow int      `yaml:"rate_limit_window"`
	TokenQuotaDaily int64    `yaml:"token_quota_daily"`
	Models          []string `yaml:"models"`
	Description     string   `yaml:"description"`
}

type AdminConfig struct {
	DefaultEmail    string `yaml:"default_email"`
	DefaultPassword string `yaml:"default_password"`
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
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = "default-secret-change-in-production"
	}
	if cfg.JWT.ExpireHours == 0 {
		cfg.JWT.ExpireHours = 24
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "llmgate.db"
	}
	if cfg.Logs.Path == "" {
		cfg.Logs.Path = "logs"
	}
	if cfg.Logs.RetentionDays == 0 {
		cfg.Logs.RetentionDays = 7
	}

	return &cfg, nil
}
