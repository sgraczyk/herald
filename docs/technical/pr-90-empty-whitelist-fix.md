# PR #90: Fix Empty Whitelist Bypass in Telegram Adapter

**Issue:** #68
**Scope:** `internal/telegram/adapter.go`, `internal/telegram/adapter_test.go`
**Type:** Security fix (fail-open to fail-closed)

## Problem

The Telegram adapter's authorization check in `handleUpdate` used a compound
condition that short-circuited when the whitelist was empty:

```go
if len(a.allowedIDs) > 0 && !a.allowedIDs[userID] {
    // reject
}
```

When `ALLOWED_USER_IDS` was unset or empty, `a.allowedIDs` had length 0. The
`len > 0` guard evaluated to `false`, the `&&` short-circuited, and the
rejection block was skipped entirely. Any Telegram user could interact with the
bot.

### Why this happened

The `len > 0` guard was likely intended to avoid rejecting users when no
whitelist was configured (a "permissive by default" design). But for a
self-hosted bot with LLM provider costs and a single-user deployment model,
fail-open is the wrong default. An unset environment variable should block all
access, not grant universal access.

## What Changed

Two commits: `3aaa2ef` (core fix) and `d4b6926` (negative ID filtering + test
hardening).

### Phase 1: Constructor validation (adapter.go:29-54)

The `New` constructor now enforces fail-closed access control in two steps:

```go
func New(token string, h *hub.Hub, allowedUserIDs []int64) (*Adapter, error) {
    a := &Adapter{
        hub:        h,
        allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
        typing:     make(map[int64]context.CancelFunc),
    }

    for _, id := range allowedUserIDs {
        if id > 0 {
            a.allowedIDs[id] = true
        }
    }

    if len(a.allowedIDs) == 0 {
        return nil, fmt.Errorf("no valid allowed user IDs configured")
    }

    // ... bot creation follows
}
```

**Step 1 -- Filter invalid IDs.** The `id > 0` check rejects both zero values
(sentinel/default) and negative values. Negative Telegram IDs represent groups
and channels, not individual users. Only positive IDs are added to the map.

**Step 2 -- Reject empty map.** After filtering, if no valid IDs remain, the
constructor returns an error. This prevents startup with a misconfigured or
missing `ALLOWED_USER_IDS` variable. The error propagates through `main.go` and
halts the process.

### Phase 2: Simplified handler check (adapter.go:77)

With the constructor guaranteeing a non-empty `allowedIDs` map, the handler
check was simplified:

```
Before:  if len(a.allowedIDs) > 0 && !a.allowedIDs[userID]
After:   if !a.allowedIDs[userID]
```

The `len > 0` guard is no longer needed. Go's map lookup returns the zero value
(`false`) for missing keys, so `!a.allowedIDs[userID]` correctly rejects any
user not in the map. This is both simpler and eliminates the bypass vector.

## Security Model

The fix implements defense in depth across two layers:

| Layer | Location | Behavior |
|-------|----------|----------|
| Construction | `New()` | Fails with error if no valid IDs configured (prevents startup) |
| Runtime | `handleUpdate()` | Rejects any user ID not in the map (no bypass possible) |

Even if a future code change somehow created an `Adapter` with an empty map
(e.g., via direct struct initialization in tests), the handler's simplified
check would still reject all messages because `map[int64]bool{}` returns
`false` for every key lookup.

## Config Layer Boundary

The config layer (`config.go`) parses `ALLOWED_USER_IDS` permissively -- it
splits the comma-separated string and converts values to `int64` without
validating ranges. Security validation is the adapter's responsibility:

```
config.go: "123,0,-456" -> []int64{123, 0, -456}   (parse only)
adapter.go: []int64{123, 0, -456} -> map{123: true}  (filter + validate)
```

This separation keeps config parsing simple and puts domain-specific validation
where the domain knowledge lives (Telegram user IDs must be positive integers).

## Edge Cases

| Input | Filtered result | Constructor outcome |
|-------|----------------|---------------------|
| `nil` | empty map | error |
| `[]int64{}` | empty map | error |
| `[]int64{0, 0}` | empty map | error |
| `[]int64{-1, -999}` | empty map | error |
| `[]int64{0, 12345}` | `{12345: true}` | success |
| `[]int64{123, 456}` | `{123: true, 456: true}` | success |

## Test Coverage

Six tests in `internal/telegram/adapter_test.go`:

### TestNewEmptyAllowedIDs

Passes `nil` to `New`. Asserts constructor returns a non-nil error.

### TestNewEmptySliceAllowedIDs

Passes `[]int64{}` to `New`. Asserts constructor returns a non-nil error.

### TestNewZeroOnlyAllowedIDs

Passes `[]int64{0, 0}` to `New`. Asserts the zero values are filtered and
the constructor fails because no valid IDs remain.

### TestNewZeroFilteredFromAllowedIDs

Passes `[]int64{0, 12345}` to `New`. Asserts the error (if any) is NOT the
"no valid allowed user IDs" error -- the valid ID `12345` must survive
filtering. The test tolerates `bot.New` failing on the fake token.

### TestNewNegativeIDsFiltered

Passes `[]int64{-1, -999}` to `New`. Asserts the constructor fails because
negative IDs are filtered and no valid IDs remain.

### TestAllowedIDsMap

Constructs an `Adapter` directly with a pre-built map `{111: true, 222: true}`.
Asserts that `allowedIDs[111]` and `allowedIDs[222]` return `true`, and
`allowedIDs[999]` returns `false` (demonstrating Go's zero-value behavior for
missing map keys).
