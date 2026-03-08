# Provider Validation

Herald validates configured LLM providers at startup, logging warnings for unreachable or misconfigured services. Validation is advisory only -- it never blocks startup or returns errors.

## API

```go
// internal/provider/validate.go
func ValidateProviders(ctx context.Context, providers []LLMProvider)
```

Dispatches to per-type validators via type switch. All outcomes are communicated through `slog` at Info (success) or Warn (failure) level.

### OpenAI-Compatible (`*OpenAI`)

Sends `GET {baseURL}/models` with `Authorization: Bearer {apiKey}`, 10-second timeout. The `/models` endpoint is read-only, free, and available on all OpenAI-compatible APIs.

| Response | Log level | Message |
|----------|-----------|---------|
| 200 | Info | `provider reachable` |
| 401/403 | Warn | `provider auth failure` |
| Other | Warn | `provider returned unexpected status` |
| Network error | Warn | `provider unreachable` |

### Claude CLI (`*Claude`)

Calls `exec.LookPath("claude")` to check PATH resolution. Lighter than `claude --version` (avoids spawning Node.js). Authentication is deferred to the first `Chat` call.

### Timeout

Each provider gets its own 10-second deadline. Worst-case startup delay with two providers is 20 seconds.

## Integration

Called in `cmd/herald/main.go` after `buildProviders` and the empty-providers guard, before `NewFallback`. The `ask` subcommand skips validation (errors propagate immediately to the terminal).

## Operator Guide

### Startup Log Messages

**Healthy:**
```
INFO  provider reachable  provider=chutes
INFO  provider reachable  provider=claude  path=/usr/local/bin/claude
```

**Auth failure:**
```
WARN  provider auth failure  provider=chutes  status=401
```
Fix: Check `CHUTES_API_KEY` in `/etc/herald/.env`.

**Unreachable:**
```
WARN  provider unreachable  provider=chutes  url=...  error=...
```
Fix: Check network. Try `curl {baseURL}/models` from the container. Herald retries on each message.

**Claude CLI not found:**
```
WARN  claude CLI not found on PATH  error=...
```
Fix: Install Claude Code CLI or ignore if not using that provider.

### Troubleshooting

| Problem | Fix |
|---------|-----|
| `provider auth failure status=401` | Update API key in `.env`, restart |
| `provider unreachable` | Wait for recovery; Herald retries per message |
| `claude CLI not found on PATH` | Install Claude Code CLI (requires Node.js) |
| Photos fail after update | Confirm vision-capable model in config (look for `VL` suffix) |
| `no providers configured` | At least one provider must be in config |

### Vision Support

| Provider | Images | Notes |
|----------|:------:|-------|
| OpenAI-compatible | Yes | Requires vision-capable model (`VL` suffix) |
| Claude CLI | No | Pipe mode is text-only; images fall back to OpenAI provider |

## Adding a New Provider

1. Implement `LLMProvider` interface (`Name()`, `Chat()`)
2. Add a case to the type switch in `validate.go`
3. Write an unexported validate function with an appropriate health check

The `default` branch logs a warning if you skip this. The bot still starts and works.
