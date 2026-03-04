package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Telegram       TelegramConfig   `json:"telegram"`
	Providers      []ProviderConfig `json:"providers"`
	Store          StoreConfig      `json:"store"`
	HistoryLimit   int              `json:"history_limit"`
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

	// Resolve env vars.
	if cfg.Telegram.TokenEnv != "" {
		cfg.Telegram.Token = os.Getenv(cfg.Telegram.TokenEnv)
	}

	for i := range cfg.Providers {
		if cfg.Providers[i].APIKeyEnv != "" {
			cfg.Providers[i].APIKey = os.Getenv(cfg.Providers[i].APIKeyEnv)
		}
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
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID %q: %w", part, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
