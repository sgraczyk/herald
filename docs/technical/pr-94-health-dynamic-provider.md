# PR #94: Return Dynamic Provider Name from /health Endpoint

**Issue:** #72
**Scope:** `internal/health/handler.go`, `internal/health/handler_test.go`, `cmd/herald/main.go`
**Type:** Bug fix (stale data in health response)

## Problem

The `/health` endpoint returned the provider name that was captured once at
startup. The `Server.provider` field was a `string`, set by calling
`chain.Name()` in `main.go` when constructing the health server. After a user
switched providers via the `/model` command (which calls `Fallback.SetActive`),
the health endpoint continued reporting the original provider name.

### Why this happened

`chain.Name()` returns the current active provider name as a string value. When
passed to `NewServer` as a `string`, the value was copied and never updated.
The `Fallback` struct tracks the active provider behind a `sync.RWMutex`, but
the health server had no reference to the `Fallback` instance -- only the
snapshot string.

## What Changed

### New `NameProvider` interface (handler.go:15-17)

A single-method interface decouples the health package from the provider
package while still allowing dynamic name resolution:

```go
type NameProvider interface {
    Name() string
}
```

This is called on every `/health` request, so the response always reflects the
current active provider.

### `Server.provider` field type change (handler.go:39)

```
Before:  provider string
After:   provider NameProvider
```

The constructor signature at `handler.go:45` changed accordingly:

```
Before:  func NewServer(port int, version string, startTime time.Time, provider string, ...) *Server
After:   func NewServer(port int, version string, startTime time.Time, provider NameProvider, ...) *Server
```

### Health response assembly (handler.go:96)

```
Before:  Provider: s.provider,
After:   Provider: s.provider.Name(),
```

Each request calls `Name()` at response time rather than reading a static
string.

### Wiring in main.go (main.go:135)

```
Before:  srv := health.NewServer(cfg.HTTPPort, version, loop.StartTime(), chain.Name(), claude, tokenExpires)
After:   srv := health.NewServer(cfg.HTTPPort, version, loop.StartTime(), chain, claude, tokenExpires)
```

The `*provider.Fallback` value is passed directly. It satisfies
`health.NameProvider` through its `Name() string` method at
`fallback.go:32-36`, which reads the `active` field under `sync.RWMutex`.

## Thread Safety

The health HTTP handler runs in a separate goroutine from the Telegram adapter
and agent loop. The provider name can change at any time via `/model` (which
calls `Fallback.SetActive`). Both paths are safe:

| Operation | Lock | Location |
|-----------|------|----------|
| `Fallback.Name()` (read) | `RLock` | `fallback.go:33` |
| `Fallback.SetActive()` (write) | `Lock` | `fallback.go:82` |
| `Fallback.Chat()` success (write) | `Lock` | `fallback.go:50` |

Multiple concurrent `/health` requests can read the name simultaneously via
`RLock`. Writes from `SetActive` or a successful `Chat` fallback acquire the
exclusive `Lock`.

## Test Coverage

Five tests in `internal/health/handler_test.go`. All tests that construct a
`Server` now use the `stubName` wrapper instead of a raw string.

### stubName test helper (handler_test.go:11-13)

```go
type stubName struct{ name string }
func (s *stubName) Name() string { return s.name }
```

Minimal struct satisfying `NameProvider`. The `name` field is exported to tests
in the same package, allowing mutation between requests.

### TestHandleHealthDynamicProviderName (handler_test.go:105-136)

The core regression test. Creates a `stubName` with `"provider-a"`, issues a
`/health` request, asserts the response contains `"provider-a"`. Then mutates
`np.name` to `"provider-b"`, issues a second request, and asserts the response
now contains `"provider-b"`. This verifies that the handler reads the name on
each request rather than caching it.

### Existing tests updated

`TestHandleHealth`, `TestHandleHealthWithTokenExpiry`,
`TestHandleHealthWithClaudeAuthError`, and `TestHandleHealthWithClaudeOK` all
changed from passing a string to passing `&stubName{"..."}`. No behavioral
change -- these tests verify the same properties as before with the new
interface.
