# Streaming Responses Design

**Issue:** [#84](https://github.com/sgraczyk/herald/issues/84)
**Milestone:** v0.6.0
**Date:** 2026-03-17

## Problem

Responses are fully buffered before sending to Telegram. For long responses, the user sees only a typing indicator until the entire response is ready. Streaming would improve perceived latency by showing text as it arrives from the LLM.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Display mode | Progressive edit-in-place | User watches the response build up via `EditMessageText` |
| Config gating | `"streaming": true` in config.json | Opt-in, with per-provider graceful degradation |
| Mid-stream failure | Discard partial, retry buffered | Clean fallback — delete in-progress message, fall to next provider via `Chat()` |
| Edit throttle | ~1 second | Smooth typing feel, safe for single-user bot |
| In-progress indicator | Append `...` during streaming | Removed on final edit |
| Streaming during drain | Disabled — use buffered `Chat()` only | Avoid long-running streams blocking shutdown |

## Architecture

### Provider Layer

New optional interface in `provider.go`:

```go
// StreamingProvider is an optional interface for providers that support
// incremental response delivery.
type StreamingProvider interface {
    LLMProvider
    // ChatStream sends a conversation and calls fn with text deltas as they arrive.
    // Providers must check ctx.Done() between processing each event/line to allow
    // prompt cancellation. Returns the complete response string on success.
    ChatStream(ctx context.Context, messages []Message, fn func(delta string)) (string, error)
}
```

- `fn` receives text deltas as they arrive from the LLM
- Returns the complete response string on success
- Providers that don't implement this interface fall back to `Chat()`
- **Contract:** Providers must check `ctx.Done()` between processing each SSE event or stdout line to support prompt cancellation by the agent loop

#### Claude Provider (`claude.go`)

Switch from `cmd.Output()` to reading stdout line by line via `cmd.StdoutPipe()`.

Use `--output-format stream-json --verbose`. The stream emits NDJSON objects. Relevant types:

```json
{"type":"assistant","message":{"content":[{"type":"text","text":"partial response..."}],...}}
{"type":"result","subtype":"success","is_error":false,"result":"full response text",...}
```

Implementation:
- Read stdout line by line with `bufio.Scanner`
- Check `ctx.Done()` between each line
- Filter for `type: "assistant"` — extract text from `message.content[0].text`, compute delta vs previous text, call `fn(delta)`
- On `type: "result"` — return `result` field as complete response
- Ignore all other types (`system`, `rate_limit_event`, etc.)
- Implements `StreamingProvider`

#### OpenAI Provider (`openai.go`)

Set `"stream": true` in the request body. Read SSE lines from the response body.

**HTTP client timeout:** The existing 60-second `http.Client.Timeout` covers the entire request lifecycle including body reads, which will kill long streaming responses. For streaming requests, use a client with no timeout and rely on the context deadline instead:

```go
// streamClient has no Timeout — streaming lifecycle is controlled by ctx.
streamClient *http.Client
```

Implementation:
- Parse `data: {...}` lines, extract `choices[0].delta.content`, call `fn(delta)`
- Check `ctx.Done()` between each SSE event
- On `data: [DONE]`, return the accumulated full text
- Implements `StreamingProvider`

#### Fallback Chain (`fallback.go`)

**No changes to `Fallback`.** The fallback chain does not implement `StreamingProvider`.

The agent loop owns the streaming-vs-buffered decision:
1. Get the active provider from the fallback chain (new `Fallback.Active() LLMProvider` method)
2. Type-assert to `StreamingProvider`
3. If supported, call `ChatStream` directly on that provider
4. On failure, fall back to `Fallback.Chat()` which handles the full retry/fallback chain in buffered mode

This keeps `Fallback` simple and avoids mixing streaming and retry logic.

### Hub Layer

New type and channel in `hub.go`:

```go
// StreamUpdate represents a partial response update for in-place editing.
type StreamUpdate struct {
    ChatID int64
    Text   string // accumulated text so far (not just the delta)
    Done   bool   // final update — apply formatting, clean up state
}
```

```go
Stream chan StreamUpdate // buffered (64)
```

Accumulated text (not deltas) is sent because `EditMessageText` replaces the full message. The agent loop accumulates; the adapter just forwards what it receives.

### Agent Loop

Changes to `handleMessage` in `loop.go`:

**Streaming path** (when `l.streaming == true` and provider implements `StreamingProvider`, and not draining):

1. Send typing indicator
2. Get active provider via `Fallback.Active()`, type-assert to `StreamingProvider`
3. Call `sp.ChatStream(ctx, messages, fn)` where the callback:
   - Accumulates text into a `strings.Builder`
   - Checks if >= 1 second since last emit
   - If so, sends `StreamUpdate{ChatID, accumulated + "...", false}` to `hub.Stream`
4. On success, send final `StreamUpdate{ChatID, fullResponse, true}` (no `...`, done=true)
5. Save user + assistant messages to store (unchanged)
6. Trigger memory extraction + summarization (unchanged)

**Failure path:**

1. Send `StreamUpdate{ChatID, "", true}` to signal adapter to delete the in-progress message
2. Fall through to `Fallback.Chat()` which handles retry/fallback in buffered mode

**During drain:** Skip streaming, use buffered `Chat()` only to avoid long-running streams blocking shutdown.

**Non-streaming fallback:** If streaming disabled or provider doesn't implement `StreamingProvider`, existing `Chat()` path runs unchanged.

**Config:** New field `Streaming bool` on `Loop`, wired from config.json:

```json
{
  "streaming": true
}
```

Note: The `NewLoop` constructor already has 8 parameters. This adds a 9th. Consider migrating to an options struct in a future cleanup, but for now the boolean is acceptable.

### Telegram Adapter

**New dispatch goroutine:** `dispatchStream(ctx)`, started alongside `dispatchOut` and `dispatchTyping` in `Start()`.

**State tracking:** New field `streamMsgs map[int64]int` (chatID -> Telegram message ID).

**`dispatchStream` logic:**

1. Read from `hub.Stream`
2. Stop typing indicator for this chat
3. If `update.Text == "" && update.Done` — error case: delete in-progress message via `DeleteMessage`, clean up map entry
4. If no entry in `streamMsgs[chatID]` — first update: `SendMessage` with plain text (no `ParseMode`), store returned message ID
5. If entry exists — `EditMessageText` with plain text (no `ParseMode`)
6. If `update.Done` — final edit: convert through `format.TelegramHTML()` + `format.Split()`, call `EditMessageText` with `ParseMode: HTML`. If HTML edit fails, retry as plain text (same fallback pattern as `dispatchOut`). If formatted text splits into multiple chunks, delete stream message and send fresh `SendMessage` chunks. Clean up map entry.

**ParseMode handling:**
- Mid-stream edits: **no `ParseMode`** (plain text) — partial markdown cannot be safely converted to HTML
- Final `Done` edit: `ParseMode: HTML` with plain-text fallback on error (matches existing `dispatchOut` pattern)

**Rate limiting:** The agent loop handles the 1-second throttle, so the adapter doesn't need its own.

**Message length during streaming:** If `EditMessageText` fails because text exceeds 4096, log and skip. The final `Done` edit handles proper splitting.

## Error Handling & Edge Cases

| Scenario | Behavior |
|----------|----------|
| Concurrent messages from same chat | Can't happen — agent loop is single-goroutine sequential |
| Context cancellation (shutdown) | `ChatStream` returns ctx error, loop sends `Done` update to clean up |
| Shutdown drain | Streaming disabled — buffered `Chat()` only to avoid blocking |
| Telegram API error during editing | Log and skip — next edit overwrites. Final `Done` edit falls back to plain text if HTML fails |
| Empty stream (zero deltas) | Loop sends one `Done` update — adapter sends single `SendMessage`, degrades to buffered |
| Image messages | No change — images are in the `messages` slice, `ChatStream` handles them the same |
| Callback backpressure | Low risk for single-user bot. If `hub.Stream` is full, the callback blocks, which back-pressures the provider. Acceptable. |

## Testing

**Unit tests:**
- `provider`: Mock stdout/HTTP for `ChatStream` on both Claude and OpenAI. Verify delta ordering, final string, mid-stream error handling, ctx cancellation between events.
- `hub`: Verify `StreamUpdate` flows through channel.
- `agent/loop`: Streaming path used when provider implements `StreamingProvider` and config on. Fallback to `Chat()` on failure. Buffered-only during drain. Mock dual-interface provider.
- `telegram/adapter`: `dispatchStream` — first update sends (no ParseMode), subsequent edits (no ParseMode), done formats with HTML + plain-text fallback, empty-text done deletes.

**Integration test:** Manual — run bot with `"streaming": true`, verify progressive response in Telegram.

No new external dependencies. No CI changes beyond new test files.
