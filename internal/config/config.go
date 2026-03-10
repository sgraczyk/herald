// Package config handles loading and validating the Herald configuration file.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config holds all runtime configuration for Herald.
type Config struct {
	Telegram           TelegramConfig   `json:"telegram"`
	Providers          []ProviderConfig `json:"providers"`
	Store              StoreConfig      `json:"store"`
	HTTPPort           int              `json:"http_port,omitempty"`
	HistoryLimit       int              `json:"history_limit"`
	HistoryTokenBudget int              `json:"history_token_budget,omitempty"`
	LogLevel           string           `json:"log_level"`
	SystemPrompt       string           `json:"system_prompt,omitempty"`
	AllowedUserIDs     []int64          `json:"-"`

	// Raw field for env var resolution.
	AllowedUserIDsEnv string `json:"allowed_user_ids_env"`
}

// TelegramConfig holds Telegram Bot API connection settings.
type TelegramConfig struct {
	TokenEnv string `json:"token_env"`
	Token    string `json:"-"`
}

// ProviderConfig describes an LLM provider entry in the configuration file.
type ProviderConfig struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "claude-cli" or "openai"
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	APIKeyEnv string `json:"api_key_env,omitempty"`
	APIKey    string `json:"-"`
}

// StoreConfig holds the bbolt database path.
type StoreConfig struct {
	Path string `json:"path"`
}

// Load reads config from path and resolves env vars for secrets.
func Load(path string) (*Config, error) {
	return LoadWithDefaults(path, nil)
}

// LoadWithDefaults reads config from path. If the file does not exist and
// defaults is non-nil, the embedded defaults are used instead.
func LoadWithDefaults(path string, defaults []byte) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if defaults != nil && os.IsNotExist(err) {
			data = defaults
		} else {
			return nil, fmt.Errorf("read config file: %w", err)
		}
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 50
	}

	if cfg.HistoryTokenBudget == 0 {
		cfg.HistoryTokenBudget = 8000
	}

	if cfg.Store.Path == "" {
		cfg.Store.Path = "herald.db"
	}

	if cfg.HTTPPort < 0 || cfg.HTTPPort > 65535 {
		return nil, fmt.Errorf("invalid http_port: %d", cfg.HTTPPort)
	}

	// Resolve env vars.
	if cfg.Telegram.TokenEnv != "" {
		cfg.Telegram.Token = os.Getenv(cfg.Telegram.TokenEnv)
	}

	for i := range cfg.Providers {
		if cfg.Providers[i].APIKeyEnv != "" {
			cfg.Providers[i].APIKey = os.Getenv(cfg.Providers[i].APIKeyEnv)
		}
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if env := os.Getenv("LOG_LEVEL"); env != "" {
		cfg.LogLevel = env
	}

	if cfg.AllowedUserIDsEnv != "" {
		raw := os.Getenv(cfg.AllowedUserIDsEnv)
		if raw != "" {
			cfg.AllowedUserIDs, err = parseUserIDs(raw)
			if err != nil {
				return nil, fmt.Errorf("parse allowed user IDs: %w", err)
			}
		}
	}

	return &cfg, nil
}

func parseUserIDs(raw string) ([]int64, error) {
	var ids []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int64
		if _, err := fmt.Sscanf(part, "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid user ID %q: %w", part, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
