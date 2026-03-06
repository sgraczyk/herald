# OpenAI Provider: Response Body Size Limit

## Overview

The OpenAI-compatible HTTP provider (`internal/provider/openai.go`) caps response body reads at 10 MB. Prior to this change, `io.ReadAll(resp.Body)` had no upper bound, meaning a misbehaving or compromised upstream API (Chutes.ai, Groq, etc.) could return an arbitrarily large response and exhaust memory on the host.

Introduced in PR #93 (commit `be25061`), tracking issue #73.

## Technical Details

### The LimitReader+1 technique

```go
const maxResponseSize = 10 << 20 // 10 MB

respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
if len(respBody) > maxResponseSize {
    return "", fmt.Errorf("response body exceeds %d bytes limit", maxResponseSize)
}
```

`io.LimitReader` alone silently truncates at the limit and returns `io.EOF`, which would produce confusing JSON parse errors downstream. Reading one extra byte solves this: if `ReadAll` returns `maxResponseSize+1` bytes, the response was too large and the caller gets an explicit error. If the response fits within 10 MB, the extra byte is never reached and behavior is identical to an unbounded read.

### Error handling order

The size check runs immediately after the read, before any other validation:

1. Read body with size limit (I/O error on failure)
2. **Check size** (rejects bodies over 10 MB)
3. Check HTTP status code (rejects non-200 responses)
4. Unmarshal JSON
5. Validate choices array

This ordering means an oversized error response (e.g., a 500 with a multi-MB HTML body) is rejected on size grounds without the full body being logged or parsed.

### Constant definition

`maxResponseSize` is an unexported package-level constant in `openai.go`, set to `10 << 20` (10,485,760 bytes). No sentinel error type was introduced; the fallback chain treats the size-exceeded error like any other provider error and moves to the next provider.

## Architecture Impact

This change is scoped entirely to the OpenAI HTTP provider. It does not affect:

- **Claude CLI provider** (`claude.go`) -- uses `exec.CommandContext` with stdout pipe, a different code path with a different risk profile.
- **Fallback chain** (`fallback.go`) -- no special handling needed. An oversized response error causes the fallback provider to try the next provider in the chain, same as any other non-sentinel error.
- **Hub / Agent loop** -- no changes. The provider contract (`LLMProvider.Chat`) is unchanged.

If other providers need a similar limit in the future, the constant could move to `provider.go`, but keeping it in `openai.go` follows least-visibility principle.

## Configuration

The 10 MB limit is a **compile-time constant**. There is no runtime configuration, no environment variable, and no `config.json` field for it. This is intentional, consistent with the project's "don't over-engineer" rule. To change the limit, edit `maxResponseSize` in `internal/provider/openai.go` and rebuild.

## Testing

Two test cases were added in `internal/provider/openai_test.go`:

| Test | What it covers |
|------|----------------|
| `TestOpenAIOversizedResponse` | HTTP 200 with body of `maxResponseSize+1` bytes. Asserts error contains `"exceeds"`. |
| `TestOpenAIOversizedErrorResponse` | HTTP 500 with body of `maxResponseSize+1` bytes. Asserts error contains `"exceeds"` (not `"API error"`), confirming size check takes precedence over status check. |

Both use `httptest.Server` to serve synthetic oversized responses.

Run the tests:

```bash
go test ./internal/provider/...
go test -race ./internal/provider/...   # with race detector
```

Run the full suite to confirm no regressions:

```bash
go test ./...
go vet ./...
```

## Deployment Notes

Herald runs on a 512 MB RAM LXC container (CT 107, Proxmox). The Go binary uses roughly 10 MB; the rest is shared with Node.js for the Claude CLI runtime. A 10 MB per-response ceiling keeps memory allocation bounded and predictable. Without this limit, a single malformed upstream response could OOM the container and crash the service.

In practice, LLM API responses are typically in the low kilobytes. The 10 MB cap is generous enough to never interfere with normal operation while still protecting against pathological cases.
