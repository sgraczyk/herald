# Provider Validation

Technical reference for Herald's startup provider validation system, introduced in PR #103.

**Related:** Issue #102, PR #98 (vision support)

---

## Overview

Herald validates configured LLM providers at startup, logging warnings for unreachable or misconfigured services. Validation is advisory only -- it never blocks startup or returns errors.

## API

```go
// internal/provider/validate.go

const validateTimeout = 10 * time.Second

func ValidateProviders(ctx context.Context, providers []LLMProvider)
```

`ValidateProviders` iterates the provider slice and dispatches to a per-type validator via type switch:

```go
switch v := p.(type) {
case *OpenAI:
    validateOpenAI(ctx, v)
case *Claude:
    validateClaude()
default:
    slog.Warn("unknown provider type, skipping validation", ...)
}
```

All outcomes are communicated through `slog` at Info (success) or Warn (failure) level.

## Validation Strategies

### OpenAI-compatible (`*OpenAI`)

Sends `GET {baseURL}/models` using the provider's own `http.Client`:

- Sets `Authorization: Bearer {apiKey}` header.
- Applies an independent 10-second `context.WithTimeout`, separate from the client's 60-second request timeout.
- Drains the response body to `io.Discard` for connection reuse.
- Response classification:
  - **200** -- logs info `"provider reachable"`
  - **401/403** -- logs warn `"provider auth failure"` with status code
  - **Other non-200** -- logs warn `"provider returned unexpected status"`
  - **Network error** -- logs warn `"provider unreachable"` with URL and error

The `/models` endpoint was chosen because it is read-only, free (no token cost), and available on all OpenAI-compatible APIs (Chutes.ai, OpenRouter, Groq).

### Claude CLI (`*Claude`)

Calls `exec.LookPath("claude")` to check PATH resolution:

- Found -- logs info with resolved binary path.
- Not found -- logs warn with the error.

This is lighter than `claude --version` (which spawns Node.js) and sufficient to detect "binary not installed." Authentication is deferred to the first `Chat` call, where `isAuthError` already handles it.

### Timeout Behavior

Each provider gets its own 10-second deadline. Worst-case startup delay with two providers is 20 seconds (both slow/unreachable). Claude validation via `LookPath` is effectively instant.

## Integration Point

In `cmd/herald/main.go`:

```go
providers := buildProviders(cfg)
if len(providers) == 0 {
    return fmt.Errorf("no providers configured")
}
provider.ValidateProviders(context.Background(), providers)
chain := provider.NewFallback(providers)
```

The ordering is deliberate:

1. **After `buildProviders`** -- concrete instances must exist before validation can access their fields.
2. **After the empty-providers guard** -- no point validating an empty slice.
3. **Before `NewFallback`** -- validation logs appear before the chain is assembled.
4. Uses `context.Background()` because signal handlers are set up later.

The `ask` subcommand does not call `ValidateProviders`. This is intentional -- `ask` is for quick local testing and errors propagate immediately to the terminal.

## Adding a New Provider

1. Implement the `LLMProvider` interface (`Name()`, `Chat()`).
2. Add a case to the type switch in `validate.go`:

   ```go
   case *MyNewProvider:
       validateMyNewProvider(ctx, v)
   ```

3. Write an unexported `validateMyNewProvider` function with an appropriate health check.

If you skip this step, the `default` branch logs a warning. The bot still starts and works; you lose startup probing for that provider.

## Testing

Tests are in `internal/provider/validate_test.go`, inside `package provider` (white-box access).

| Test | Scenario | Technique |
|------|----------|-----------|
| `TestValidateOpenAISuccess` | 200 OK; verifies path, method, auth header | `httptest.NewServer` with assertion handler |
| `TestValidateOpenAIAuthError` | 401 Unauthorized | `httptest.NewServer` returning 401 |
| `TestValidateOpenAIUnreachable` | Connection refused | Points at `127.0.0.1:1` |
| `TestValidateOpenAIServerError` | 500 Internal Server Error | `httptest.NewServer` returning 500 |
| `TestValidateProvidersCallsBoth` | Type switch dispatches to both providers | Mixed `*Claude` + `*OpenAI` list |

Tests assert "no panic" rather than log output content. `TestValidateProvidersCallsBoth` accepts that `validateClaude` may warn in CI where the CLI is not installed.

### Known Gaps

- No test for the `default` type switch branch (unknown provider type).
- No assertions on exact log messages or severity levels.
- Timeout behavior is not tested (would require a slow test server or mock clock).

## Design Decisions

### Type switch vs. interface method

Uses `switch v := p.(type)` rather than adding `Validate() error` to `LLMProvider`. This avoids changing existing code and keeps validation logic co-located in one file. The trade-off (updating the switch for new providers) is acceptable for a two-provider system.

### Log-only, no error return

`ValidateProviders` returns nothing. A provider unreachable at startup may recover before the first user message. The fallback chain already handles runtime failures. Hard-failing on a transient outage would reduce resilience.

### Shallow probes

Both strategies are intentionally shallow:

- **OpenAI**: `/models` confirms auth and reachability but does not verify the specific configured model exists.
- **Claude**: `LookPath` confirms the binary is on PATH but does not verify auth tokens.

Deeper validation was excluded to keep startup fast and avoid side effects (token consumption, rate limit hits).

### Config-only vision fix

No code changes were needed to restore vision. The `OpenAI` provider already built correct multi-part content arrays with `image_url` blocks (PR #98). The failure was a dead upstream model endpoint, fixed by swapping to `Qwen/Qwen2.5-VL-72B-Instruct` in `config.json.example`.
