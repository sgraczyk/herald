# Embed config.json.example as built-in defaults

**PR #109 | Issue #106**

## Summary

Herald now embeds `config.json.example` into the binary at build time using Go's `//go:embed` directive. When no `config.json` file is found on disk, the binary falls back to these built-in defaults instead of failing to start. Existing setups with a `config.json` file are completely unaffected -- the file on disk always takes precedence.

### Changes

- **`defaults.go`** (new) -- Root package file that embeds `config.json.example` as `herald.DefaultConfig`.
- **`internal/config/config.go`** -- New `LoadWithDefaults(path, defaults)` function. The existing `Load` function now delegates to it with `nil` defaults, preserving backward compatibility.
- **`cmd/herald/main.go`** -- Both the `serve` (default) and `ask` commands call `LoadWithDefaults` with `herald.DefaultConfig`.
- **`internal/config/config_test.go`** -- Three new tests covering fallback, disk-overrides-embedded, and nil-defaults-error cases.

### Why

Deploying Herald previously required copying `config.json.example` to `config.json` and editing it -- an extra step that was easy to forget for fresh installs. Since the example file already contains sensible defaults and all secrets come from environment variables, embedding it removes that friction without any loss of flexibility.

---

## Technical Documentation

### Architecture

The embedding is split across two layers to respect Go's `//go:embed` constraint (files must be relative to the declaring package):

1. **Root package (`herald`)** -- `defaults.go` owns the `//go:embed config.json.example` directive and exports `DefaultConfig []byte`.
2. **Internal config package** -- `LoadWithDefaults(path string, defaults []byte)` accepts the defaults as a parameter and uses them when the file at `path` does not exist.

This avoids an import cycle: `internal/config` cannot import the root package since `cmd/herald` depends on both.

### Config loading flow

```
LoadWithDefaults(path, defaults)
         |
    os.ReadFile(path)
      /          \
   OK          error
    |             |
    |       os.IsNotExist?
    |        /        \
    |      yes         no
    |       |           |
    |  defaults != nil? return error
    |   /         \
    | yes          no
    |  |            |
    | use defaults  return error
    |  |
    +--+
    |
  json.Unmarshal
    |
  apply zero-value defaults
  (HistoryLimit=50, Store.Path="herald.db", LogLevel="info")
    |
  resolve env vars
    |
  return *Config
```

### Behavior matrix

| File exists | defaults | Result |
|-------------|----------|--------|
| yes | any | Uses file on disk |
| no | non-nil | Uses embedded defaults |
| no | nil | Returns error |

### Extending defaults

To change default values, edit `config.json.example` at the repo root. The embed directive references it by name, so changes are picked up on the next build with no code changes.

To add a new config field: add it to the `Config` struct with JSON tags, add zero-value default logic in `LoadWithDefaults` if needed, update `config.json.example`, and add tests.

Adding a new provider to `config.json.example` is safe because `buildProviders` skips OpenAI-type providers whose API key env var is unset.

### Design decisions

- **`[]byte` parameter, not global access.** Keeps `internal/config` independently testable and avoids import cycles.
- **Whole-file replacement, not field-level merging.** Either the disk file is used entirely or the embedded content is used entirely. Simple and predictable.
- **Dual-purpose example file.** `config.json.example` serves as both the human-readable example and the compiled-in defaults, eliminating drift between the two.

---

## User Documentation

### What this means for you

Herald no longer requires a `config.json` file to start. The built-in defaults match `config.json.example` from the repository, including:

- Claude CLI as primary provider, Chutes.ai as fallback
- Database at `herald.db`, health endpoint on port 8080
- 50-message conversation history, log level `info`
- Secrets read from env vars: `TELEGRAM_TOKEN`, `CHUTES_API_KEY`, `ALLOWED_USER_IDS`

### Fresh install

1. Download the Herald binary.
2. Create `.env` with your secrets:
   ```
   TELEGRAM_TOKEN=your-telegram-bot-token
   CHUTES_API_KEY=your-chutes-api-key
   ALLOWED_USER_IDS=123456789
   ```
3. Run Herald. No `config.json` needed.

### Upgrading from a previous version

No action required. If you have a `config.json` file, Herald continues to use it. The embedded defaults only activate when no config file is found.

### Customizing configuration

If the defaults do not fit your needs, copy `config.json.example` to `config.json`, edit it, and restart Herald. The file on disk always takes precedence over the built-in defaults.

### Resetting to defaults

Delete or rename your `config.json` and restart Herald:

```
rm config.json
systemctl restart herald
```

### CLI flag

The `--config` / `-c` flag still works. If the specified file does not exist, Herald falls back to the built-in defaults.

```
./herald --config /etc/herald/config.json
```
