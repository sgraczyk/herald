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

## Architecture

### Provider Layer

New optional interface in `provider.go`:

```go
type StreamingProvider interface {
    LLMProvider
    ChatStream(ctx context.Context, messages []Message, fn func(delta string)) (string, error)
}
```

- `fn` receives text deltas as they arrive from the LLM
- Returns the complete response string on success
- Providers that don't implement this interface fall back to `Chat()`

#### Claude Provider (`claude.go`)

- Switch from `cmd.Output()` to reading stdout line by line
- Use `--output-format stream-json --verbose`
- Filter for `type: "assistant"` messages, extract text delta from `content[0].text`, call `fn(delta)`
- On `type: "result"`, return the complete `result` field
- Implements `StreamingProvider`

#### OpenAI Provider (`openai.go`)

- Set `"stream": true` in the request body
- Read SSE lines from the response body
- Parse `data: {...}` lines, extract `choices[0].delta.content`, call `fn(delta)`
- On `data: [DONE]`, return the accumulated full text
- Implements `StreamingProvider`

#### Fallback Chain (`fallback.go`)

- Add `ChatStream` method — `Fallback` implements `StreamingProvider`
- Check if active provider implements `StreamingProvider`
- If yes, call `ChatStream` — on success, return
- On failure (or if provider doesn't support it), signal caller to delete partial message, fall back to next provider's `Chat()` in buffered mode

### Hub Layer

New type and channel in `hub.go`:

```go
type StreamDelta struct {
    ChatID int64
    Text   string // accumulated text so far (not just the delta)
    Done   bool   // final update — apply formatting, clean up state
}
```

```go
Stream chan StreamDelta // buffered (64)
```

Accumulated text (not deltas) is sent because `EditMessageText` replaces the full message. The agent loop accumulates; the adapter just forwards what it receives.

### Agent Loop

Changes to `handleMessage` in `loop.go`:

**Streaming path** (when `l.streaming == true` and provider implements `StreamingProvider`):

1. Send typing indicator
2. Call `sp.ChatStream(ctx, messages, fn)` where the callback:
   - Accumulates text into a `strings.Builder`
   - Checks if >= 1 second since last emit
   - If so, sends `StreamDelta{ChatID, accumulated + "...", false}` to `hub.Stream`
3. On success, send final `StreamDelta{ChatID, fullResponse, true}` (no `...`, done=true)
4. Save user + assistant messages to store (unchanged)
5. Trigger memory extraction + summarization (unchanged)

**Failure path:**

1. Send `StreamDelta{ChatID, "", true}` to signal adapter to delete the in-progress message
2. Fall through to buffered `Chat()` path with next provider in fallback chain

**Non-streaming fallback:** If streaming disabled or provider doesn't implement `StreamingProvider`, existing `Chat()` path runs unchanged.

**Config:** New field `Streaming bool` on `Loop`, wired from config.json:

```json
{
  "streaming": true
}
```

### Telegram Adapter

**New dispatch goroutine:** `dispatchStream(ctx)`, started alongside `dispatchOut` and `dispatchTyping` in `Start()`.

**State tracking:** New field `streamMsgs map[int64]int` (chatID -> Telegram message ID).

**`dispatchStream` logic:**

1. Read from `hub.Stream`
2. Stop typing indicator for this chat
3. If `delta.Text == "" && delta.Done` — error case: delete in-progress message via `DeleteMessage`, clean up map entry
4. If no entry in `streamMsgs[chatID]` — first delta: `SendMessage` with plain text, store returned message ID
5. If entry exists — `EditMessageText` with plain text
6. If `delta.Done` — final edit: convert through `format.TelegramHTML()` + `format.Split()`, edit with `ParseMode: HTML`. If formatted text exceeds 4096 or splits into multiple chunks, delete stream message and send fresh `SendMessage` chunks. Clean up map entry.

**Rate limiting:** The agent loop handles the 1-second throttle, so the adapter doesn't need its own.

**Message length during streaming:** If `EditMessageText` fails because text exceeds 4096, log and skip. The final `Done` edit handles proper splitting.

## Error Handling & Edge Cases

| Scenario | Behavior |
|----------|----------|
| Concurrent messages from same chat | Can't happen — agent loop is single-goroutine sequential |
| Context cancellation (shutdown) | `ChatStream` returns ctx error, loop sends `Done` delta to clean up |
| Telegram API error during editing | Log and skip — next edit overwrites. Final `Done` edit falls back to plain text if HTML fails |
| Empty stream (zero deltas) | Loop sends one `Done` delta — adapter sends single `SendMessage`, degrades to buffered |
| Image messages | No change — images are in the `messages` slice, `ChatStream` handles them the same |

## Testing

**Unit tests:**
- `provider`: Mock stdout/HTTP for `ChatStream` on both Claude and OpenAI. Verify delta ordering, final string, mid-stream error handling.
- `hub`: Verify `StreamDelta` flows through channel.
- `agent/loop`: Streaming path used when provider implements `StreamingProvider` and config on. Fallback to `Chat()` on failure. Mock dual-interface provider.
- `telegram/adapter`: `dispatchStream` — first delta sends, subsequent edits, done formats+cleans up, empty-text done deletes.

**Integration test:** Manual — run bot with `"streaming": true`, verify progressive response in Telegram.

No new external dependencies. No CI changes beyond new test files.
