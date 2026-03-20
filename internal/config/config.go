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
	Telegram           TelegramConfig      `json:"telegram"`
	Providers          []ProviderConfig    `json:"providers"`
	ImageProviders []ImageProviderConfig `json:"image_providers,omitempty"`
	Store              StoreConfig          `json:"store"`
	HTTPPort           int                  `json:"http_port,omitempty"`
	HistoryLimit       int                  `json:"history_limit"`
	HistoryTokenBudget int                  `json:"history_token_budget,omitempty"`
	MaxDocumentTokens  int                  `json:"max_document_tokens,omitempty"`
	MaxRetries               *int `json:"max_retries,omitempty"`
	MaxArchivedConversations *int `json:"max_archived_conversations,omitempty"`
	LogLevel           string               `json:"log_level"`
	Summarize          bool                 `json:"summarize,omitempty"`
	Streaming          bool                 `json:"streaming,omitempty"`
	SystemPrompt       string               `json:"system_prompt,omitempty"`
	StatusMessages     *StatusMessages      `json:"status_messages,omitempty"`
	AllowedUserIDs     []int64              `json:"-"`

	// Raw field for env var resolution.
	AllowedUserIDsEnv string `json:"allowed_user_ids_env"`
}

// StatusMessages holds user-facing status and error message templates.
// When nil, English defaults are used.
type StatusMessages struct {
	ImageGenerating string `json:"image_generating,omitempty"`
	ImageTimeout    string `json:"image_timeout,omitempty"`
	ImageAuthError  string `json:"image_auth_error,omitempty"`
	ImageGenericErr string `json:"image_generic_error,omitempty"`
	ImageTooLarge   string `json:"image_too_large,omitempty"`
	ProvTimeout     string `json:"provider_timeout,omitempty"`
	ProvAuthError   string `json:"provider_auth_error,omitempty"`
	ProvGenericErr  string `json:"provider_generic_error,omitempty"`
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

// ImageProviderConfig describes an image generation provider entry.
type ImageProviderConfig struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "chutes" or "none"
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

	if cfg.MaxDocumentTokens == 0 {
		cfg.MaxDocumentTokens = 4000
	}

	if cfg.MaxRetries == nil {
		one := 1
		cfg.MaxRetries = &one
	}

	if cfg.MaxArchivedConversations == nil {
		fifty := 50
		cfg.MaxArchivedConversations = &fifty
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

	for i, ip := range cfg.ImageProviders {
		if ip.Name == "" {
			return nil, fmt.Errorf("image_providers[%d].name is required", i)
		}
		if ip.Type == "chutes" && ip.BaseURL == "" {
			return nil, fmt.Errorf("image_providers[%d].base_url is required when type is \"chutes\"", i)
		}
		if ip.APIKeyEnv != "" {
			cfg.ImageProviders[i].APIKey = os.Getenv(ip.APIKeyEnv)
		}
	}

	if err := applyStatusMessageDefaults(&cfg); err != nil {
		return nil, err
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

// applyStatusMessageDefaults fills in missing status message fields with
// English defaults. If status_messages is nil, a fully-populated default
// struct is created. If any explicitly-set field is empty, validation fails.
func applyStatusMessageDefaults(cfg *Config) error {
	defaults := StatusMessages{
		ImageGenerating: "Generating image...",
		ImageTimeout:    "Image generation took too long. Try a simpler prompt or try again shortly.",
		ImageAuthError:  "Image service configuration issue. The admin has been notified.",
		ImageGenericErr: "Failed to generate image. Please try again.",
		ImageTooLarge:   "Generated image is too large for Telegram.",
		ProvTimeout:     "Response took too long. Try a simpler question or try again shortly.",
		ProvAuthError:   "Service configuration issue. The admin has been notified.",
		ProvGenericErr:  "I'm temporarily unavailable. Please try again shortly.",
	}

	if cfg.StatusMessages == nil {
		cfg.StatusMessages = &defaults
		return nil
	}

	sm := cfg.StatusMessages
	// Apply defaults for unset fields, reject empty strings for set fields.
	type field struct {
		val  *string
		def  string
		name string
	}
	fields := []field{
		{&sm.ImageGenerating, defaults.ImageGenerating, "image_generating"},
		{&sm.ImageTimeout, defaults.ImageTimeout, "image_timeout"},
		{&sm.ImageAuthError, defaults.ImageAuthError, "image_auth_error"},
		{&sm.ImageGenericErr, defaults.ImageGenericErr, "image_generic_error"},
		{&sm.ImageTooLarge, defaults.ImageTooLarge, "image_too_large"},
		{&sm.ProvTimeout, defaults.ProvTimeout, "provider_timeout"},
		{&sm.ProvAuthError, defaults.ProvAuthError, "provider_auth_error"},
		{&sm.ProvGenericErr, defaults.ProvGenericErr, "provider_generic_error"},
	}
	for _, f := range fields {
		if *f.val == "" {
			*f.val = f.def
		}
	}
	return nil
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
