package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Telegram       TelegramConfig   `json:"telegram"`
	Providers      []ProviderConfig `json:"providers"`
	Store          StoreConfig      `json:"store"`
	HTTPPort       int              `json:"http_port,omitempty"`
	HistoryLimit   int              `json:"history_limit"`
	LogLevel       string           `json:"log_level"`
	AllowedUserIDs []int64          `json:"-"`

	// Raw field for env var resolution.
	AllowedUserIDsEnv string `json:"allowed_user_ids_env"`
}

type TelegramConfig struct {
	TokenEnv string `json:"token_env"`
	Token    string `json:"-"`
}

type ProviderConfig struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "claude-cli" or "openai"
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model,omitempty"`
	APIKeyEnv string `json:"api_key_env,omitempty"`
	APIKey    string `json:"-"`
}

type StoreConfig struct {
	Path string `json:"path"`
}

// Load reads config from path and resolves env vars for secrets.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if cfg.HistoryLimit == 0 {
		cfg.HistoryLimit = 50
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
	for _, part := range splitTrim(raw, ',') {
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

func splitTrim(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			part := trimSpace(s[start:i])
			parts = append(parts, part)
			start = i + 1
		}
	}
	part := trimSpace(s[start:])
	parts = append(parts, part)
	return parts
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
