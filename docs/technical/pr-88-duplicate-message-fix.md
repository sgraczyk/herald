# PR #88: Fix Duplicate User Message in LLM Context

**Issue:** #67
**Scope:** `internal/agent/loop.go`, `internal/agent/loop_test.go`
**Type:** Bug fix (pure reorder, no logic changes)

## Problem

`handleMessage` in `internal/agent/loop.go` appended the user message to bbolt
**before** loading history. When `buildMessages` (in `internal/agent/context.go`)
assembled the provider payload, it concatenated the full history (which already
contained the just-appended user message) with `userText` as a separate trailing
message. The LLM received the user message twice per turn.

### Execution order (before)

```
store.Append(userMsg)       // user message now in bbolt
history = store.List()      // history includes userMsg
msgs = buildMessages(history, ..., userText)
  -> [system, ...history(includes userMsg), userMsg]   // DUPLICATE
provider.Chat(msgs)
store.Append(assistantMsg)
```

### Execution order (after)

```
history = store.List()      // history does NOT include current message
msgs = buildMessages(history, ..., userText)
  -> [system, ...history, userMsg]                     // correct, single occurrence
provider.Chat(msgs)
store.Append(userMsg)       // saved only after successful provider call
store.Append(assistantMsg)
```

## What Changed

The fix moves the `store.Append` call for the user message from **before**
`store.List` / `buildMessages` to **after** `provider.Chat`. This is the same
10 lines of code relocated within `handleMessage` -- no new logic, no deleted
logic.

### `handleMessage` final structure (loop.go:179-235)

```go
func (l *Loop) handleMessage(ctx context.Context, msg hub.InMessage) {
    // 1. Load history and memories
    history, err := l.store.List(msg.ChatID)
    memories, err := l.store.ListMemories(msg.ChatID)

    // 2. Typing indicator
    l.hub.Typing <- msg.ChatID

    // 3. Build messages and call provider
    messages := buildMessages(history, memories, msg.Text, l.systemPrompt)
    response, err := l.provider.Chat(ctx, messages)
    // ... error handling returns early (user message NOT saved)

    // 4. Save user message (only on success)
    userMsg := provider.Message{Role: "user", Content: msg.Text, Timestamp: time.Now()}
    l.store.Append(msg.ChatID, userMsg, l.historyLimit)

    // 5. Save assistant message
    assistantMsg := provider.Message{Role: "assistant", Content: response, Timestamp: time.Now()}
    l.store.Append(msg.ChatID, assistantMsg, l.historyLimit)

    // 6. Send response and extract memories
    l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: response}
    l.extractMemories(ctx, msg.ChatID, msg.Text, response)
}
```

### How `buildMessages` assembles the payload (context.go:26-57)

`buildMessages` always appends `userText` as the final message:

```
[system prompt (with memories)] + [history...] + [user message]
```

The bug occurred because the user message was already inside `history` when
loaded from the store, then appended again by `buildMessages`. Moving
`store.Append` after the provider call ensures `history` never contains the
current turn's user message.

## Side Effect: Transactional Consistency

With the new ordering, if `provider.Chat` returns an error, `handleMessage`
returns early **before** `store.Append` for the user message. This means failed
turns leave no trace in bbolt. Previously, the user message was persisted even
when the provider failed, creating orphan messages with no assistant response.

This matches the assistant message behavior, which was already only saved on
success.

## Test Coverage

Three new tests in `internal/agent/loop_test.go` use `capturingProvider`, a mock
that records all `[]provider.Message` slices passed to `Chat()`.

### capturingProvider

```go
type capturingProvider struct {
    responses []string
    callCount int
    captured  [][]provider.Message
}
```

Each call to `Chat` copies the message slice into `captured` and returns the
next response from `responses`. The second response (`"[]"`) satisfies the
memory extraction call that follows every successful conversation turn.

### TestHandleMessageNoDuplicateUserMessage

Fresh chat (empty store). Sends `"hello"`, then asserts:
- `captured[0]` contains `"hello"` with role `"user"` exactly once.
- `db.List(1)` contains exactly one user message with content `"hello"`.

### TestHandleMessageNoDuplicateWithHistory

Pre-populates the store with a prior user/assistant exchange. Sends
`"new message"`, then asserts:
- `captured[0]` contains `"new message"` exactly once (prior history is present
  but does not include the new message).

### TestHandleMessageErrorDoesNotSaveUserMessage

Uses `mockProvider` configured to return an error. Sends `"hello"`, then
asserts:
- `db.List(1)` returns zero messages -- the user message was not persisted.

## Key Interfaces

### provider.LLMProvider

```go
type LLMProvider interface {
    Name() string
    Chat(ctx context.Context, messages []Message) (string, error)
}
```

### store.DB (relevant methods)

```go
func (db *DB) Append(chatID int64, msg provider.Message, limit int) error
func (db *DB) List(chatID int64) ([]provider.Message, error)
func (db *DB) Count(chatID int64) (int, error)
func (db *DB) Clear(chatID int64) error
```

### buildMessages

```go
func buildMessages(
    history []provider.Message,
    memories []store.Memory,
    userText string,
    customPrompt string,
) []provider.Message
```

Assembles: `[system] + history + [current user message]`. The system message
includes injected memories when present (up to 50, with explicit memories
prioritized over auto-extracted ones).
