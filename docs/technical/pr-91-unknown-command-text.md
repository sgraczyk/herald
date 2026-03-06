# PR #91: Preserve Full Text for Unknown Commands

**Issue:** #74
**Scope:** `internal/telegram/adapter.go`, `internal/telegram/adapter_test.go`, `internal/agent/loop.go`
**Type:** Bug fix (data loss in message routing)

## Problem

When a user sent an unknown command like `/foo bar`, the Telegram adapter
stripped the command prefix and passed only `"bar"` as `InMessage.Text` to the
hub. The agent loop's `handle` method found no matching case in its command
switch and fell through to `default`, calling `handleMessage` -- which sent just
`"bar"` to the LLM provider. The user's actual intent (`/foo bar`) was lost.

### Why this happened

The old parsing logic in `handleUpdate` unconditionally stripped the command
prefix from ALL messages starting with `/`. It split on the first space,
extracted the command, and set `Text` to the remainder. This was correct for
known commands like `/model claude` (where the handler expects just `"claude"`),
but wrong for unrecognized commands that should pass through to the LLM with
their full text intact.

## What Changed

Two commits: `80a9fe7` (core fix) and `9f36df2` (cross-reference comments).

### Phase 1: knownCommands map and parseMessage extraction (adapter.go:17-127)

The fix introduces a package-level `knownCommands` map and extracts message
parsing into a standalone `parseMessage` function:

```go
var knownCommands = map[string]bool{
    "/clear":    true,
    "/model":    true,
    "/status":   true,
    "/remember": true,
    "/forget":   true,
    "/memories": true,
}
```

The `parseMessage` function implements a three-way classification:

```go
func parseMessage(chatID, userID int64, text string) hub.InMessage {
    in := hub.InMessage{
        ChatID: chatID,
        UserID: userID,
        Text:   text,
    }

    if strings.HasPrefix(text, "/") {
        parts := strings.SplitN(text, " ", 2)
        cmd := strings.SplitN(parts[0], "@", 2)[0]
        in.Command = cmd
        if knownCommands[cmd] && len(parts) > 1 {
            in.Text = strings.TrimSpace(parts[1])
        }
    }

    return in
}
```

**The critical fix is the condition on line 121:** `knownCommands[cmd] && len(parts) > 1`. Previously, all commands had their prefix stripped unconditionally. Now, only known commands with arguments get the argument-only text. Everything else retains the full original input.

### Phase 2: Cross-reference comments (adapter.go:17-19, loop.go:56)

Bidirectional comments were added to link the two locations that must stay in
sync:

- `adapter.go:17-19` -- comment above `knownCommands` pointing to the `handle`
  switch in `loop.go`
- `loop.go:56` -- comment above `handle` pointing to `knownCommands` in
  `adapter.go`

## Message Classification

The `parseMessage` function classifies input into one of four cases:

| Input | Command | Text | Rationale |
|-------|---------|------|-----------|
| `hello world` | `""` | `"hello world"` | Regular text, no command |
| `/clear` | `"/clear"` | `"/clear"` | Known command, no args (Text = full original) |
| `/model claude` | `"/model"` | `"claude"` | Known command with args (Text = args only) |
| `/foo bar` | `"/foo"` | `"/foo bar"` | Unknown command (Text = full original for LLM) |
| `/foo` | `"/foo"` | `"/foo"` | Unknown command, no args (Text = full original) |
| `/clear@herald_bot` | `"/clear"` | `"/clear@herald_bot"` | Bot mention stripped from Command only |

The key behavioral difference: known commands with arguments receive just the
argument portion in `Text` because their handlers expect it (e.g., `handleModel`
reads `msg.Text` as the provider name). Unknown commands retain the full text
because they fall through to `handleMessage`, which sends `msg.Text` to the LLM
-- the model needs the complete input to understand what the user meant.

## Routing Through the Agent Loop

When `handleMessage` receives an unknown command like `/foo bar`, the full text
flows through unchanged:

```
parseMessage("/foo bar")
    -> InMessage{Command: "/foo", Text: "/foo bar"}
        -> handle() switch: no match for "/foo", falls to default
            -> handleMessage(): sends "/foo bar" to LLM provider
```

The LLM sees `"/foo bar"` and can respond meaningfully. Previously it saw only
`"bar"` -- a fragment with no context.

## knownCommands / handle Switch Sync

The `knownCommands` map in `adapter.go` and the `switch` statement in
`loop.go:handle` must list the same commands. They are linked by convention with
cross-reference comments, not by a shared data structure. This is a deliberate
simplicity trade-off:

- **If a command is in `knownCommands` but not in the switch:** The command's
  argument text gets stripped, then `handleMessage` sends just the args to the
  LLM. Minor UX issue, no crash.
- **If a command is in the switch but not in `knownCommands`:** The command
  reaches the handler with full original text instead of just args. The handler
  may misinterpret it. Also a minor UX issue, no crash.

Both failure modes degrade gracefully. A shared constant or code generation was
considered unnecessary for 6 commands.

## Test Coverage

Six tests in `internal/telegram/adapter_test.go` (lines 55-120), all
exercising the `parseMessage` function directly:

### TestParseMessageUnknownCommandWithArgs

Passes `"/foo bar"`. Asserts `Command == "/foo"` and `Text == "/foo bar"`. This
is the primary regression test -- the exact scenario from issue #74.

### TestParseMessageUnknownCommandNoArgs

Passes `"/foo"`. Asserts `Command == "/foo"` and `Text == "/foo"`. Verifies that
an unknown command without arguments also retains its full text.

### TestParseMessageKnownCommandWithArgs

Table-driven test covering three known commands with arguments:

| Input | Expected Command | Expected Text |
|-------|-----------------|---------------|
| `/model claude` | `/model` | `claude` |
| `/remember I prefer Go` | `/remember` | `I prefer Go` |
| `/forget python` | `/forget` | `python` |

Confirms known commands still have their prefix stripped correctly.

### TestParseMessageKnownCommandNoArgs

Passes `"/clear"`. Asserts `Command == "/clear"` and `Text == "/clear"`. A known
command without arguments keeps its full text (the handler ignores `Text` in this
case anyway).

### TestParseMessageRegularText

Passes `"hello world"`. Asserts `Command == ""` and `Text == "hello world"`.
Verifies non-command messages pass through untouched.

### TestParseMessageBotMentionSuffix

Passes `"/clear@herald_bot"`. Asserts `Command == "/clear"`. Verifies the
`@botname` suffix stripping still works with the new parsing logic.

## Design Notes

**Why a pure function?** Extracting `parseMessage` from `handleUpdate` makes the
parsing logic testable without constructing a full `Adapter` or mocking the
Telegram bot. All seven parsing tests call `parseMessage` directly with chat ID,
user ID, and text -- no setup required.

**Why not route unknown commands to a dedicated handler?** The `default` case in
`handle` already sends `msg.Text` to the LLM via `handleMessage`. The fix
ensures that `msg.Text` contains the right content. Adding a separate
`handleUnknownCommand` would duplicate the `handleMessage` logic for no benefit.
