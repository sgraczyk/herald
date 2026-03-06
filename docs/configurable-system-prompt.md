# Configurable System Prompt

## Overview

Herald uses a system prompt as the first message in every LLM call to define the assistant's personality and formatting rules. Previously, this was a hardcoded constant. The configurable system prompt feature adds an optional `system_prompt` field to `config.json` that replaces the default when set.

This allows operators to customize Herald's personality, tone, language, or domain focus without rebuilding the binary. When the field is empty or absent, the hardcoded default is used. Memory injection and conversation history assembly are unaffected -- they layer on top of whichever prompt is active.

Package: `internal/agent`, `internal/config` | Files: `context.go`, `loop.go`, `config.go` | PR: #66 | Issue: #36

## Architecture

### Data flow

```
config.json ("system_prompt" field)
  |
  v
config.Load() --> Config.SystemPrompt (string)
  |
  v
cmd/herald/main.go --> logs prompt status, passes to agent.NewLoop()
  |
  v
Loop.systemPrompt (stored on struct)
  |
  v
buildMessages(history, memories, userText, customPrompt)
  |
  v
if customPrompt != "" --> use customPrompt
else                  --> use defaultSystemPrompt constant
  |
  v
append memory injection (if any memories exist)
  |
  v
system message sent as msgs[0] to provider
```

### Prompt assembly

The system message is always the first element in the message slice passed to the provider. It is constructed in `buildMessages()`:

```go
prompt := defaultSystemPrompt
if customPrompt != "" {
    prompt = customPrompt
}
if len(selected) > 0 {
    var b strings.Builder
    b.WriteString(prompt)
    b.WriteString("\n\nYou know the following about the user:")
    for _, m := range selected {
        fmt.Fprintf(&b, "\n- %s", m.Fact)
    }
    prompt = b.String()
}
```

The custom prompt fully replaces the default -- it does not merge or append. Memory facts are appended to whichever prompt is active, separated by a blank line and the header "You know the following about the user:".

## API Reference

### Config struct

```go
type Config struct {
    // ...
    SystemPrompt string `json:"system_prompt,omitempty"`
    // ...
}
```

The field uses `omitempty` so it does not appear in serialized output when empty. No validation is performed in `config.Load()` -- an empty string is the zero value and triggers the default fallback.

### NewLoop

```go
func NewLoop(h *hub.Hub, p provider.LLMProvider, s *store.DB, historyLimit int, systemPrompt string) *Loop
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `systemPrompt` | `string` | Custom system prompt. Empty string means use the hardcoded default. |

The prompt is stored as `l.systemPrompt` on the `Loop` struct and passed to `buildMessages()` on every message handling call.

### buildMessages

```go
func buildMessages(history []provider.Message, memories []store.Memory, userText string, customPrompt string) []provider.Message
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `customPrompt` | `string` | If non-empty, replaces `defaultSystemPrompt`. Memories are appended after. |

**Returns:** `[]provider.Message` -- ordered as `[system, ...history, user]`.

### Default prompt

The hardcoded fallback defined as `defaultSystemPrompt` in `internal/agent/context.go`:

```go
const defaultSystemPrompt = `You are Herald, a helpful AI assistant on Telegram. Be concise and direct. Respond in the same language the user writes in.

Formatting rules for Telegram:
- Never use markdown tables. Present tabular data as bullet lists or key-value pairs.
- Don't use headings (# syntax). Use bold text instead.
- Use bold, italic, and code blocks for emphasis.
- Keep messages concise — users read on mobile.`
```

## Startup Logging

Prompt status is logged after `initLogging()` completes, so the correct slog handler is active.

| Condition | Level | Message | Fields |
|-----------|-------|---------|--------|
| `system_prompt` empty or missing | Info | `system_prompt not set, using default` | none |
| `system_prompt` set, length > 4000 | Warn | `system_prompt is very long, may consume significant context window` | `length` (int) |
| `system_prompt` set, length <= 4000 | -- | No log emitted | -- |

The 4000-character threshold is a warning, not a hard limit. The prompt is passed to the provider regardless of length.

```go
if cfg.SystemPrompt == "" {
    slog.Info("system_prompt not set, using default")
} else if len(cfg.SystemPrompt) > 4000 {
    slog.Warn("system_prompt is very long, may consume significant context window",
        slog.Int("length", len(cfg.SystemPrompt)))
}
```

## Configuration

### config.json

Add the `system_prompt` field at the top level:

```json
{
  "system_prompt": "You are a pirate assistant. Always respond in pirate speak.",
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  },
  "providers": [],
  "store": {
    "path": "herald.db"
  }
}
```

The field is not present in `config.json.example` since it is optional and most deployments use the default.

### Omitting the field

Any of these configurations result in the default prompt being used:

```json
{}
```

```json
{
  "system_prompt": ""
}
```

```json
{
  "system_prompt": null
}
```

All three are equivalent because `json.Unmarshal` leaves the string at its zero value (`""`), and `buildMessages()` treats empty string as "use default".

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Field absent from config | Zero value (`""`), default prompt used |
| Field set to empty string | Default prompt used |
| Field set to whitespace-only string | Treated as non-empty, used as-is (no trimming) |
| Very long prompt (> 4000 chars) | Warning logged at startup, prompt used without truncation |
| Prompt contains newlines, unicode, special characters | Passed through verbatim, no escaping or validation |
| Prompt with no formatting rules | Works, but LLM may produce markdown that Telegram renders poorly |
| Memories exist | Appended to custom prompt, same as default behavior |
| No memories exist | Custom prompt used as-is, no memory header appended |

## Test Coverage

Three tests in `internal/agent` verify the behavior:

| Test | Verifies |
|------|----------|
| `TestBuildMessagesWithCustomPrompt` | Custom prompt replaces default in the system message |
| `TestBuildMessagesWithoutMemories` | Empty `customPrompt` falls back to `defaultSystemPrompt` |
| `TestBuildMessagesWithMemories` | Memory injection appends to default prompt when `customPrompt` is empty |

## Design Decisions

**Optional with zero-value fallback.** Using Go's zero value (`""`) as the sentinel avoids a pointer type or a separate boolean flag. The `omitempty` JSON tag keeps it out of serialized output when unset.

**No config.json.example entry.** The field is purely optional and most users do not need it. Adding it to the example would imply it requires attention.

**Warning threshold, not a hard limit.** A 4000-character prompt consumes significant context window budget, especially with the `claude -p` backend. The warning alerts operators without restricting legitimate use cases (e.g., detailed domain-specific instructions).

**Full replacement, not merge.** The custom prompt completely replaces the default rather than appending to it. This gives operators full control over the system message content, including the ability to remove Telegram formatting rules if they have their own.

## Related Documentation

- [Customize Herald's Personality](user/custom-personality.md) -- user-facing guide for this feature
- [Structured Logging](structured-logging.md) -- covers `slog` patterns used for prompt status logging
