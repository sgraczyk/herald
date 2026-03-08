# Technical Documentation: Embedded Config Defaults

PR #109 / Issue #106

## Overview

Herald embeds `config.json.example` into the compiled binary at build time using Go's `//go:embed` directive. This allows the binary to start with sane defaults when no `config.json` file exists on disk, eliminating a hard dependency on a config file being present at the expected path.

## Architecture

### Embedding mechanism

The embedding is split across two layers:

1. **Root package (`herald`)** -- owns the embedded byte slice
2. **Internal config package (`internal/config`)** -- accepts optional defaults as a parameter

This separation exists because `//go:embed` can only reference files relative to the package directory. Since `config.json.example` lives at the repository root, the embed directive must also live in the root package.

```
defaults.go (package herald)         config.go (package config)
+--------------------------+         +----------------------------------+
| //go:embed               |         | func LoadWithDefaults(           |
|   config.json.example    |  pass   |   path string,                   |
|                          | ------> |   defaults []byte,               |
| var DefaultConfig []byte |         | ) (*Config, error)               |
+--------------------------+         +----------------------------------+
```

### Config loading flow

```
                    LoadWithDefaults(path, defaults)
                              |
                    os.ReadFile(path)
                      /            \
                   OK             error
                   |                |
                   |          os.IsNotExist?
                   |           /        \
                   |         yes         no
                   |          |           |
                   |    defaults != nil?  return error
                   |     /         \
                   |   yes          no
                   |    |            |
                   | use defaults  return error
                   |    |
                   +----+
                   |
           json.Unmarshal(data)
                   |
           apply zero-value defaults
           (HistoryLimit=50, Store.Path="herald.db", LogLevel="info")
                   |
           resolve env vars
           (tokens, API keys, allowed user IDs, LOG_LEVEL override)
                   |
           return *Config
```

The key behavior: a config file on disk always wins. Embedded defaults are only used when the file is missing AND defaults are provided.

## API Reference

### `defaults.go` (package `herald`)

```go
package herald

import _ "embed"

//go:embed config.json.example
var DefaultConfig []byte
```

Exported package-level variable containing the raw bytes of `config.json.example`. Available to any package that imports `github.com/sgraczyk/herald`.

### `config.LoadWithDefaults`

```go
func LoadWithDefaults(path string, defaults []byte) (*Config, error)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `path` | `string` | Filesystem path to config file. Attempted first. |
| `defaults` | `[]byte` | Fallback JSON bytes used when `path` does not exist. Pass `nil` to disable fallback (same behavior as `Load`). |

**Returns:** Parsed `*Config` with env vars resolved, or an error.

**Behavior matrix:**

| File exists | defaults | Result |
|-------------|----------|--------|
| yes | any | Uses file on disk |
| no | non-nil | Uses embedded defaults |
| no | nil | Returns error |

**Error conditions:**

- File exists but is unreadable (permissions, I/O) -- returns read error
- JSON is malformed -- returns parse error
- `http_port` outside 0-65535 -- returns validation error
- `allowed_user_ids_env` references env var with non-numeric values -- returns parse error

### `config.Load`

```go
func Load(path string) (*Config, error)
```

Convenience wrapper. Calls `LoadWithDefaults(path, nil)` -- no fallback, file must exist.

### Config struct

```go
type Config struct {
    Telegram       TelegramConfig   `json:"telegram"`
    Providers      []ProviderConfig `json:"providers"`
    Store          StoreConfig      `json:"store"`
    HTTPPort       int              `json:"http_port,omitempty"`
    HistoryLimit   int              `json:"history_limit"`
    LogLevel       string           `json:"log_level"`
    SystemPrompt   string           `json:"system_prompt,omitempty"`
    AllowedUserIDs []int64          `json:"-"`
    AllowedUserIDsEnv string        `json:"allowed_user_ids_env"`
}
```

Fields with implicit defaults (applied when zero-value after unmarshal):

| Field | Default | Source |
|-------|---------|--------|
| `HistoryLimit` | `50` | Hardcoded in `LoadWithDefaults` |
| `Store.Path` | `"herald.db"` | Hardcoded in `LoadWithDefaults` |
| `LogLevel` | `"info"` | Hardcoded in `LoadWithDefaults`, overridable by `LOG_LEVEL` env var |

## Call Sites

Both entry points in `cmd/herald/main.go` use the embedded defaults:

```go
// serve command (default)
cfg, err := config.LoadWithDefaults(configPath, herald.DefaultConfig)

// ask subcommand
cfg, err := config.LoadWithDefaults(configPath, herald.DefaultConfig)
```

The `--config` / `-c` flag defaults to `"config.json"`. If that file does not exist, the embedded `config.json.example` content is used transparently.

## How to Extend or Modify Defaults

### Changing default values

Edit `config.json.example` at the repository root. The embed directive references this file by name, so changes are picked up automatically on the next build. No code changes required.

### Adding new config fields

1. Add the field to the `Config` struct (or a nested struct) in `internal/config/config.go` with appropriate JSON tags.
2. Add any zero-value default logic after the `json.Unmarshal` call in `LoadWithDefaults`.
3. If the field references an env var, add resolution logic in the env var resolution block.
4. Update `config.json.example` with a sensible default value.
5. Add test cases in `internal/config/config_test.go`.

### Adding a new provider to defaults

Add the provider entry to `config.json.example`:

```json
{
  "providers": [
    {"name": "claude", "type": "claude-cli"},
    {"name": "new-provider", "type": "openai", "base_url": "https://...", "model": "...", "api_key_env": "NEW_API_KEY"}
  ]
}
```

The `buildProviders` function in `cmd/herald/ask.go` skips OpenAI-type providers whose API key env var is unset, so adding providers to the embedded defaults is safe -- they are only activated when the corresponding env var is present.

## Testing Approach

The test suite in `internal/config/config_test.go` covers the feature with three dedicated tests:

**`TestLoadWithDefaults_UsesEmbeddedWhenFileAbsent`** -- Verifies that when the config file path does not exist and defaults are provided, the defaults are parsed and used correctly. Checks that provider config, telegram token env, and history limit all come from the defaults byte slice.

**`TestLoadWithDefaults_DiskOverridesEmbedded`** -- Verifies that when a config file exists on disk, it takes precedence over the provided defaults. Both defaults and a disk file are provided; assertions confirm disk values win.

**`TestLoadWithDefaults_NilDefaultsMissingFile`** -- Verifies that passing `nil` defaults with a nonexistent file path returns an error, preserving backward compatibility with the original `Load` behavior.

These tests do not depend on the actual `config.json.example` file. They pass synthetic `[]byte` defaults to `LoadWithDefaults`, making them hermetic and independent of the embedded content.

## Design Decisions

**Root package for embed, internal package for logic.** Go's `//go:embed` can only embed files relative to the declaring package's directory. Since `config.json.example` is at the repo root, the embed must be in the root package. The config parsing logic stays in `internal/config` to maintain the project's `internal/` convention. The boundary is a `[]byte` parameter -- simple, testable, no coupling.

**`[]byte` parameter instead of global access.** `LoadWithDefaults` accepts defaults as a parameter rather than importing the root package directly. This avoids an import cycle (`internal/config` cannot import `herald` since `herald` transitively depends on `internal/config` through `cmd/herald`). It also makes the function independently testable with arbitrary default content.

**Whole-file replacement, not field-level merging.** When the config file is missing, the entire embedded JSON is used as-is. There is no deep merge between embedded defaults and a partial config file. This keeps the logic simple and predictable: either the file on disk is used completely, or the embedded content is used completely.

**`config.json.example` serves dual purpose.** The same file acts as both the human-readable example for new deployments and the compiled-in defaults. This eliminates drift between "what the example says" and "what the binary defaults to."

**`Load` delegates to `LoadWithDefaults`.** The original `Load` function now calls `LoadWithDefaults(path, nil)`, maintaining full backward compatibility. Existing callers that do not want fallback behavior are unaffected.
