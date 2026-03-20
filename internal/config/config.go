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

	// presentKeys tracks which top-level JSON keys were explicitly set.
	presentKeys map[string]bool `json:"-"`
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

	// Capture which top-level keys are present before unmarshalling
	// into Config, so Validate can distinguish explicit values from defaults.
	var rawKeys map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawKeys); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}
	cfg.presentKeys = make(map[string]bool, len(rawKeys))
	for k := range rawKeys {
		cfg.presentKeys[k] = true
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
// struct is created. Empty fields are replaced with their defaults.
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

// ValidationResult holds the output of config validation.
type ValidationResult struct {
	// Warnings lists feature-level issues and potential misconfigurations.
	Warnings []string
	// Defaults lists fields where built-in defaults were applied.
	Defaults []string
}

// Validate inspects the loaded config and returns warnings about missing
// feature configuration and info about fields where defaults were applied.
func (c *Config) Validate() ValidationResult {
	var r ValidationResult

	presentKeys := c.presentKeys
	if presentKeys == nil {
		presentKeys = make(map[string]bool)
	}

	// --- Warnings ---

	if len(c.Providers) == 0 {
		r.Warnings = append(r.Warnings, "no providers configured — herald will not start")
	}

	if c.Telegram.Token == "" {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("telegram token not set (env var: %s) — herald will not start", c.Telegram.TokenEnv))
	}

	if len(c.ImageProviders) == 0 {
		r.Warnings = append(r.Warnings,
			"image_providers not configured — image generation will be unavailable")
	}

	for _, p := range c.Providers {
		if p.Type == "openai" && p.APIKey == "" {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("provider %q: API key not set (env var: %s)", p.Name, p.APIKeyEnv))
		}
	}

	if c.AllowedUserIDsEnv == "" {
		r.Warnings = append(r.Warnings,
			"allowed_user_ids_env not configured — no user whitelist set")
	} else if len(c.AllowedUserIDs) == 0 {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("allowed user IDs env var %q is empty — all messages will be rejected",
				c.AllowedUserIDsEnv))
	}

	if len(c.SystemPrompt) > 4000 {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("system_prompt is very long (%d chars), may consume significant context window",
				len(c.SystemPrompt)))
	}

	// --- Defaults ---

	type defaultField struct {
		key   string
		value any
	}
	fields := []defaultField{
		{"history_limit", c.HistoryLimit},
		{"history_token_budget", c.HistoryTokenBudget},
		{"max_document_tokens", c.MaxDocumentTokens},
	}
	if c.MaxRetries != nil {
		fields = append(fields, defaultField{"max_retries", *c.MaxRetries})
	}
	if c.MaxArchivedConversations != nil {
		fields = append(fields, defaultField{"max_archived_conversations", *c.MaxArchivedConversations})
	}
	// Only report log_level as default if not overridden by LOG_LEVEL env var.
	if os.Getenv("LOG_LEVEL") == "" {
		fields = append(fields, defaultField{"log_level", c.LogLevel})
	}
	for _, f := range fields {
		if !presentKeys[f.key] {
			r.Defaults = append(r.Defaults,
				fmt.Sprintf("using default %s: %v", f.key, f.value))
		}
	}

	return r
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
