# internal/config Package

Package `config` handles configuration loading for Herald. It reads a JSON file, applies defaults, validates constraints, and resolves secrets from environment variables.

**Location:** `internal/config/config.go`
**Dependencies:** `encoding/json`, `fmt`, `os` (stdlib only, no external deps)

## Types

### Config

Top-level configuration struct.

| Field | JSON Key | Type | Description |
|-------|----------|------|-------------|
| Telegram | `telegram` | TelegramConfig | Telegram bot settings |
| Providers | `providers` | []ProviderConfig | LLM providers in fallback order |
| Store | `store` | StoreConfig | bbolt database settings |
| HTTPPort | `http_port` | int | Health endpoint port (0 = disabled) |
| HistoryLimit | `history_limit` | int | Max messages per chat (default: 50) |
| LogLevel | `log_level` | string | Log level (default: "info") |
| SystemPrompt | `system_prompt` | string | Custom system prompt (optional) |
| AllowedUserIDsEnv | `allowed_user_ids_env` | string | Env var name containing allowed user IDs |
| AllowedUserIDs | - | []int64 | Resolved user IDs (not serialized) |

### TelegramConfig

| Field | JSON Key | Type | Description |
|-------|----------|------|-------------|
| TokenEnv | `token_env` | string | Env var name containing the bot token |
| Token | - | string | Resolved token value (not serialized) |

### ProviderConfig

| Field | JSON Key | Type | Description |
|-------|----------|------|-------------|
| Name | `name` | string | Display name for the provider |
| Type | `type` | string | Provider type: `"claude-cli"` or `"openai"` |
| BaseURL | `base_url` | string | API base URL (openai type only) |
| Model | `model` | string | Model identifier (openai type only) |
| APIKeyEnv | `api_key_env` | string | Env var name containing the API key |
| APIKey | - | string | Resolved API key (not serialized) |

### StoreConfig

| Field | JSON Key | Type | Description |
|-------|----------|------|-------------|
| Path | `path` | string | Path to bbolt database file (default: "herald.db") |

## Functions

### Load

```go
func Load(path string) (*Config, error)
```

Reads the config file at `path`, applies defaults, validates, and resolves environment variables. Returns the fully populated `Config` or an error.

**Processing order:**

1. Read file from disk
2. Parse JSON
3. Apply defaults (`HistoryLimit`, `Store.Path`, `LogLevel`)
4. Validate `HTTPPort` (must be 0-65535)
5. Resolve `Telegram.Token` from env var named in `TokenEnv`
6. Resolve `APIKey` for each provider from env var named in `APIKeyEnv`
7. Apply `LOG_LEVEL` env override (if set, overrides JSON value)
8. Parse `AllowedUserIDs` from env var named in `AllowedUserIDsEnv`

**Errors returned:**

| Condition | Error message prefix |
|-----------|---------------------|
| File not found / unreadable | `read config file:` |
| Invalid JSON | `parse config file:` |
| Port out of range | `invalid http_port:` |
| Non-numeric user ID | `parse allowed user IDs:` |

## Configuration Format

### JSON Schema

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  },
  "providers": [
    {
      "name": "claude",
      "type": "claude-cli"
    },
    {
      "name": "chutes",
      "type": "openai",
      "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
      "model": "Qwen/Qwen2.5-VL-32B-Instruct",
      "api_key_env": "CHUTES_API_KEY"
    }
  ],
  "store": {
    "path": "herald.db"
  },
  "http_port": 8080,
  "history_limit": 50,
  "log_level": "info",
  "system_prompt": "You are helpful.",
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

## Environment Variable Resolution

Secrets are never stored in the JSON file. Instead, the JSON contains the **name** of the environment variable that holds the secret. At load time, `os.Getenv` resolves the actual value.

```
config.json:  "token_env": "TELEGRAM_TOKEN"
.env file:    TELEGRAM_TOKEN=bot123456:ABC-DEF
Result:       cfg.Telegram.Token == "bot123456:ABC-DEF"
```

This pattern applies to:
- `telegram.token_env` -> `Telegram.Token`
- `providers[].api_key_env` -> `Providers[].APIKey`
- `allowed_user_ids_env` -> `AllowedUserIDs` (parsed as comma-separated int64 list)

Fields resolved from env vars use the `json:"-"` tag, so they are excluded from JSON serialization.

### LogLevel Three-Tier Precedence

LogLevel has a special override mechanism:

1. **Default:** `"info"` (if omitted from JSON)
2. **JSON value:** whatever is set in `log_level`
3. **Environment override:** `LOG_LEVEL` env var (always wins if set)

This is the only field where an env var directly overrides the JSON value. All other env-resolved fields use the indirection pattern described above.

## Defaults

| Field | Default Value | Applied When |
|-------|---------------|-------------|
| HistoryLimit | 50 | JSON value is 0 or omitted |
| Store.Path | "herald.db" | JSON value is empty or omitted |
| LogLevel | "info" | JSON value is empty or omitted |
| HTTPPort | 0 | Omitted (disabled) |

## Validation

| Rule | Behavior |
|------|----------|
| HTTPPort range | Must be 0-65535; returns error otherwise |
| AllowedUserIDs format | Each entry must be a valid int64; returns error on parse failure |

Notable things that are **not** validated:
- LogLevel is not checked against a set of known values
- Telegram token is not checked for non-empty after resolution
- Provider `type` is not validated against known types

## Testing

### Running Tests

```bash
go test ./internal/config/...
```

The test suite has 100% statement coverage (11 tests).

### Test Strategy

- **File isolation:** Each test writes a temp config file via `writeConfig(t, jsonString)` using `t.TempDir()`.
- **Env var management:** `t.Setenv()` sets env vars that are automatically restored after the test.
- **Same-package tests:** Tests are in `package config` (not `config_test`), giving access to unexported functions like `parseUserIDs`.
- **Table-driven tests:** Used for `parseUserIDs` (7 cases) and `InvalidHTTPPort` (2 cases).

### Test Coverage

| Test | What It Covers |
|------|---------------|
| TestLoad_FullConfig | All fields populated and resolved correctly |
| TestLoad_Defaults | Default values for HistoryLimit, Store.Path, LogLevel |
| TestLoad_EnvVarResolution | Token and API key resolution from env vars |
| TestLoad_LogLevelEnvOverride | LOG_LEVEL env overrides JSON log_level |
| TestLoad_MissingFile | Error on non-existent file path |
| TestLoad_MalformedJSON | Error on invalid JSON |
| TestLoad_InvalidHTTPPort | Error on port -1 and 70000 |
| TestLoad_AllowedUserIDsParseError | Error on non-numeric user IDs |
| TestLoad_AllowedUserIDsEmptyEnv | Empty env var produces empty slice |
| TestParseUserIDs | 7 cases: normal, whitespace, empty parts, negatives, errors |
| TestParseUserIDs_EmptyString | Empty string returns empty slice |

## Examples

### Minimal Config

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  }
}
```

With defaults applied: `HistoryLimit=50`, `Store.Path="herald.db"`, `LogLevel="info"`, `HTTPPort=0`.

### Full Production Config

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  },
  "providers": [
    {
      "name": "claude",
      "type": "claude-cli"
    },
    {
      "name": "chutes",
      "type": "openai",
      "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
      "model": "Qwen/Qwen2.5-VL-32B-Instruct",
      "api_key_env": "CHUTES_API_KEY"
    }
  ],
  "store": {
    "path": "herald.db"
  },
  "http_port": 8080,
  "history_limit": 50,
  "log_level": "info",
  "system_prompt": "You are a helpful assistant.",
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

Corresponding `.env` file:

```bash
TELEGRAM_TOKEN=bot123456:ABC-DEF
CHUTES_API_KEY=your_api_key
ALLOWED_USER_IDS=123456789,987654321
```

### Usage in Code

```go
cfg, err := config.Load("config.json")
if err != nil {
    log.Fatalf("load config: %v", err)
}

// Resolved secrets are available directly:
fmt.Println(cfg.Telegram.Token)       // "bot123456:ABC-DEF"
fmt.Println(cfg.Providers[0].APIKey)  // "" (claude-cli has no API key)
fmt.Println(cfg.Providers[1].APIKey)  // "your_api_key"
fmt.Println(cfg.AllowedUserIDs)       // [123456789 987654321]
```
