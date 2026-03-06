# Structured Logging

Herald uses Go's standard `log/slog` package for structured logging. This replaced the previous `log.Printf` calls (PR #65) to provide consistent key-value output suitable for both local development and production log aggregation.

## Overview

Every `log.Printf` call was replaced with a typed `slog` call at the appropriate level. There are no custom loggers or wrappers -- all code calls `slog.Debug`, `slog.Info`, `slog.Warn`, or `slog.Error` directly via the package-level default logger.

Key properties:
- Zero dependencies beyond the Go stdlib
- TTY-aware handler selection (human-readable vs. JSON)
- Configurable log level via `config.json` or environment variable
- All fields use typed `slog` helpers, never string interpolation

## Architecture

### Initialization

Logging is configured in `cmd/herald/main.go` via `initLogging(levelStr string)`. It runs once during `serve()`, after config is loaded but before any other component starts.

```go
func initLogging(levelStr string) {
    // Parse level string ("debug", "info", "warn", "error") -> slog.Level
    // Default: slog.LevelInfo

    opts := &slog.HandlerOptions{Level: level}

    // TTY detection: check if stderr is a character device
    fi, err := os.Stderr.Stat()
    if err == nil && fi.Mode()&os.ModeCharDevice != 0 {
        handler = slog.NewTextHandler(os.Stderr, opts)
    } else {
        handler = slog.NewJSONHandler(os.Stderr, opts)
    }
    slog.SetDefault(slog.New(handler))
}
```

### Handler selection

| Condition | Handler | Output | When |
|-----------|---------|--------|------|
| stderr is a TTY | `slog.TextHandler` | `time=... level=INFO msg="..."` | Local dev, `./herald` in terminal |
| stderr is not a TTY | `slog.JSONHandler` | `{"time":"...","level":"INFO","msg":"..."}` | systemd/journald, piped output |

This is detected automatically. No flag or config option is needed.

### Config precedence

1. `config.json` field `"log_level"` sets the base level (default: `"info"`)
2. `LOG_LEVEL` environment variable overrides the config file value if set
3. The level string is case-insensitive; `"warning"` is accepted as an alias for `"warn"`

Resolved in `config.Load()`:

```go
if cfg.LogLevel == "" {
    cfg.LogLevel = "info"
}
if env := os.Getenv("LOG_LEVEL"); env != "" {
    cfg.LogLevel = env
}
```

## Log Level Guide

### Error -- operation failures that lost data or broke a user-visible flow

Used when an operation failed and the user gets a degraded experience.

- Store read/write failures (history, memories)
- Message send failures after retry exhaustion
- Provider call failures (surfaced to the user as an error message)

### Warn -- recoverable issues, fallbacks, security events

Used when something went wrong but the system recovered or the event needs attention.

- Provider fallback (auth failure, timeout on one provider before trying the next)
- HTML send failure with plain-text retry
- Unauthorized user rejected
- Invalid config values ignored (e.g., malformed `CLAUDE_TOKEN_EXPIRES`)
- Memory persistence failures during auto-extraction

### Info -- lifecycle events and operational milestones

Used sparingly for events an operator would want to see in normal production logs.

- `herald starting` (with version and provider)
- `herald stopped`
- `health endpoint started` (with port)

### Debug -- verbose, routine, or high-frequency operations

Used for diagnostics that would be noisy at higher levels.

- Typing action send failures (transient, expected during cancellation)
- Memory extraction failures (non-critical background task)
- Memory parse failures (LLM returned bad JSON)

## Structured Fields Convention

All log calls use typed `slog` attribute helpers. Never use `fmt.Sprintf` inside a log message -- put variable data in fields.

### Standard fields

| Field | Type | Used for |
|-------|------|----------|
| `chat_id` | `slog.Int64` | Telegram chat identifier, present on all per-chat operations |
| `user_id` | `slog.Int64` | Telegram user identifier, used in auth rejection logs |
| `provider` | `slog.String` | Provider name (`"claude"`, `"chutes"`) |
| `error` | `slog.String` | Error message via `err.Error()` |
| `port` | `slog.Int` | HTTP port for health endpoint |
| `version` | `slog.String` | Build version string |
| `value` | `slog.String` | Raw config/input value that failed validation |
| `response` | `slog.String` | Raw LLM response (only in debug-level parse failure logs) |

### Correct patterns

```go
// Operation failure with context
slog.Error("save user message failed",
    slog.Int64("chat_id", msg.ChatID),
    slog.String("error", err.Error()),
)

// Provider fallback warning
slog.Warn("provider auth failure",
    slog.String("provider", p.Name()),
)

// Lifecycle event
slog.Info("herald starting",
    slog.String("version", version),
    slog.String("provider", chain.Name()),
)

// Verbose diagnostic
slog.Debug("send typing action failed",
    slog.Int64("chat_id", chatID),
    slog.String("error", err.Error()),
)
```

### What to avoid

```go
// Do not interpolate values into the message string
slog.Info(fmt.Sprintf("started on port %d", port))  // wrong
slog.Info("started", slog.Int("port", port))          // correct

// Do not pass errors with %v in message
slog.Error(fmt.Sprintf("failed: %v", err))                        // wrong
slog.Error("operation failed", slog.String("error", err.Error())) // correct

// Do not use untyped args
slog.Info("event", "key", value)                  // wrong (untyped)
slog.Info("event", slog.String("key", value))     // correct (typed)
```

## Configuration

### config.json

Add or change the `log_level` field:

```json
{
  "log_level": "debug"
}
```

Valid values: `"debug"`, `"info"`, `"warn"` (or `"warning"`), `"error"`. Unrecognized values default to `"info"`.

### Environment variable override

```bash
LOG_LEVEL=debug ./herald
```

This takes precedence over the config file value. Useful for temporary debugging without editing config.

### systemd override

```bash
# Temporarily enable debug logging
systemctl set-environment LOG_LEVEL=debug
systemctl restart herald

# Revert
systemctl unset-environment LOG_LEVEL
systemctl restart herald
```

## Output Examples

### TTY (local development)

```
time=2026-03-06T10:00:00.000Z level=INFO msg="herald starting" version=dev provider=claude
time=2026-03-06T10:00:00.001Z level=INFO msg="health endpoint started" port=8080
time=2026-03-06T10:00:05.123Z level=WARN msg="rejected message from unauthorized user" user_id=999999
time=2026-03-06T10:00:10.456Z level=ERROR msg="provider call failed" chat_id=12345 error="all providers failed: claude: timeout"
```

### JSON (systemd/journald)

```json
{"time":"2026-03-06T10:00:00.000Z","level":"INFO","msg":"herald starting","version":"v0.1.0","provider":"claude"}
{"time":"2026-03-06T10:00:05.123Z","level":"WARN","msg":"rejected message from unauthorized user","user_id":999999}
```
