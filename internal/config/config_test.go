package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

const fullConfig = `{
  "telegram": {"token_env": "TEST_TG_TOKEN"},
  "providers": [
    {"name": "test-claude", "type": "claude-cli"},
    {"name": "test-openai", "type": "openai", "base_url": "https://api.example.com", "model": "gpt-4", "api_key_env": "TEST_API_KEY"}
  ],
  "store": {"path": "/tmp/test.db"},
  "http_port": 8080,
  "history_limit": 30,
  "log_level": "debug",
  "system_prompt": "You are helpful.",
  "allowed_user_ids_env": "TEST_ALLOWED_IDS"
}`

func TestLoad_FullConfig(t *testing.T) {
	t.Setenv("TEST_TG_TOKEN", "tok123")
	t.Setenv("TEST_API_KEY", "key456")
	t.Setenv("TEST_ALLOWED_IDS", "111,222")

	cfg, err := Load(writeConfig(t, fullConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Telegram.TokenEnv != "TEST_TG_TOKEN" {
		t.Errorf("TokenEnv = %q, want %q", cfg.Telegram.TokenEnv, "TEST_TG_TOKEN")
	}
	if cfg.Telegram.Token != "tok123" {
		t.Errorf("Token = %q, want %q", cfg.Telegram.Token, "tok123")
	}
	if len(cfg.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "test-claude" {
		t.Errorf("Providers[0].Name = %q, want %q", cfg.Providers[0].Name, "test-claude")
	}
	if cfg.Providers[1].APIKey != "key456" {
		t.Errorf("Providers[1].APIKey = %q, want %q", cfg.Providers[1].APIKey, "key456")
	}
	if cfg.Providers[1].BaseURL != "https://api.example.com" {
		t.Errorf("Providers[1].BaseURL = %q, want %q", cfg.Providers[1].BaseURL, "https://api.example.com")
	}
	if cfg.Providers[1].Model != "gpt-4" {
		t.Errorf("Providers[1].Model = %q, want %q", cfg.Providers[1].Model, "gpt-4")
	}
	if cfg.Store.Path != "/tmp/test.db" {
		t.Errorf("Store.Path = %q, want %q", cfg.Store.Path, "/tmp/test.db")
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("HTTPPort = %d, want 8080", cfg.HTTPPort)
	}
	if cfg.HistoryLimit != 30 {
		t.Errorf("HistoryLimit = %d, want 30", cfg.HistoryLimit)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.SystemPrompt != "You are helpful." {
		t.Errorf("SystemPrompt = %q, want %q", cfg.SystemPrompt, "You are helpful.")
	}
	if len(cfg.AllowedUserIDs) != 2 || cfg.AllowedUserIDs[0] != 111 || cfg.AllowedUserIDs[1] != 222 {
		t.Errorf("AllowedUserIDs = %v, want [111 222]", cfg.AllowedUserIDs)
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, `{"telegram": {"token_env": "X"}}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HistoryLimit != 50 {
		t.Errorf("HistoryLimit = %d, want 50", cfg.HistoryLimit)
	}
	if cfg.Store.Path != "herald.db" {
		t.Errorf("Store.Path = %q, want %q", cfg.Store.Path, "herald.db")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_EnvVarResolution(t *testing.T) {
	t.Setenv("MY_TOKEN", "secret-token")
	t.Setenv("MY_KEY", "secret-key")

	json := `{
		"telegram": {"token_env": "MY_TOKEN"},
		"providers": [{"name": "p", "type": "openai", "api_key_env": "MY_KEY"}]
	}`
	cfg, err := Load(writeConfig(t, json))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Telegram.Token != "secret-token" {
		t.Errorf("Token = %q, want %q", cfg.Telegram.Token, "secret-token")
	}
	if cfg.Providers[0].APIKey != "secret-key" {
		t.Errorf("APIKey = %q, want %q", cfg.Providers[0].APIKey, "secret-key")
	}
}

func TestLoad_LogLevelEnvOverride(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")

	cfg, err := Load(writeConfig(t, `{"telegram": {"token_env": "X"}, "log_level": "debug"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadWithDefaults_UsesEmbeddedWhenFileAbsent(t *testing.T) {
	defaults := []byte(`{
		"telegram": {"token_env": "EMBED_TG"},
		"providers": [{"name": "embed-claude", "type": "claude-cli"}],
		"history_limit": 25
	}`)

	cfg, err := LoadWithDefaults("/nonexistent/config.json", defaults)
	if err != nil {
		t.Fatalf("LoadWithDefaults: %v", err)
	}
	if cfg.Telegram.TokenEnv != "EMBED_TG" {
		t.Errorf("TokenEnv = %q, want %q", cfg.Telegram.TokenEnv, "EMBED_TG")
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "embed-claude" {
		t.Errorf("Providers = %v, want [embed-claude]", cfg.Providers)
	}
	if cfg.HistoryLimit != 25 {
		t.Errorf("HistoryLimit = %d, want 25", cfg.HistoryLimit)
	}
}

func TestLoadWithDefaults_DiskOverridesEmbedded(t *testing.T) {
	defaults := []byte(`{"telegram": {"token_env": "EMBED_TG"}, "history_limit": 25}`)
	diskConfig := `{"telegram": {"token_env": "DISK_TG"}, "history_limit": 10}`

	cfg, err := LoadWithDefaults(writeConfig(t, diskConfig), defaults)
	if err != nil {
		t.Fatalf("LoadWithDefaults: %v", err)
	}
	if cfg.Telegram.TokenEnv != "DISK_TG" {
		t.Errorf("TokenEnv = %q, want %q", cfg.Telegram.TokenEnv, "DISK_TG")
	}
	if cfg.HistoryLimit != 10 {
		t.Errorf("HistoryLimit = %d, want 10", cfg.HistoryLimit)
	}
}

func TestLoadWithDefaults_NilDefaultsMissingFile(t *testing.T) {
	_, err := LoadWithDefaults("/nonexistent/config.json", nil)
	if err == nil {
		t.Fatal("expected error when file missing and no defaults")
	}
}

func TestLoad_MalformedJSON(t *testing.T) {
	_, err := Load(writeConfig(t, `{not json`))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoad_InvalidHTTPPort(t *testing.T) {
	tt := []struct {
		json string
	}{
		{`{"telegram": {"token_env": "X"}, "http_port": -1}`},
		{`{"telegram": {"token_env": "X"}, "http_port": 70000}`},
	}
	for _, tc := range tt {
		_, err := Load(writeConfig(t, tc.json))
		if err == nil {
			t.Errorf("expected error for config %s", tc.json)
		}
	}
}

func TestLoad_AllowedUserIDsParseError(t *testing.T) {
	t.Setenv("BAD_IDS", "123,abc")

	_, err := Load(writeConfig(t, `{"telegram": {"token_env": "X"}, "allowed_user_ids_env": "BAD_IDS"}`))
	if err == nil {
		t.Fatal("expected error for invalid user IDs")
	}
}

func TestLoad_AllowedUserIDsEmptyEnv(t *testing.T) {
	t.Setenv("EMPTY_IDS", "")

	cfg, err := Load(writeConfig(t, `{"telegram": {"token_env": "X"}, "allowed_user_ids_env": "EMPTY_IDS"}`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.AllowedUserIDs) != 0 {
		t.Errorf("AllowedUserIDs = %v, want empty", cfg.AllowedUserIDs)
	}
}

func TestParseUserIDs(t *testing.T) {
	tt := []struct {
		input string
		want  []int64
		err   bool
	}{
		{"123,456,789", []int64{123, 456, 789}, false},
		{"123, 456 , 789", []int64{123, 456, 789}, false},
		{"  42  ", []int64{42}, false},
		{"123, , 456", []int64{123, 456}, false},
		{"-100,200", []int64{-100, 200}, false},
		{"abc", nil, true},
		{"123,abc,456", nil, true},
	}
	for _, tc := range tt {
		got, err := parseUserIDs(tc.input)
		if tc.err {
			if err == nil {
				t.Errorf("parseUserIDs(%q): expected error", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseUserIDs(%q): unexpected error: %v", tc.input, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseUserIDs(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseUserIDs(%q)[%d] = %d, want %d", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestParseUserIDs_EmptyString(t *testing.T) {
	got, err := parseUserIDs("")
	if err != nil {
		t.Fatalf("parseUserIDs empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("parseUserIDs empty = %v, want empty", got)
	}
}
