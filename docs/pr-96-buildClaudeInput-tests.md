# buildClaudeInput: Function Reference and Test Documentation

PR #96 | Closes #71 | File: `internal/provider/claude_test.go`

## Overview

`buildClaudeInput` is an unexported pure function in the `provider` package that transforms a slice of structured `Message` values into a single prompt string suitable for the Claude CLI pipe mode (`claude -p`).

It sits in the critical path of the Claude provider pipeline:

```
Agent Loop
  -> Claude.Chat(ctx, []Message)
    -> buildClaudeInput(messages)  // <-- this function
    -> exec claude -p --output-format json (stdin = prompt string)
    -> parse JSON response
```

The function is defined at `internal/provider/claude.go:101-131`.

## Function Signature

```go
func buildClaudeInput(messages []Message) string
```

**Input:** A slice of `Message` structs (`Role`, `Content`, `Timestamp` fields).
Only `Role` and `Content` are used; `Timestamp` is ignored.

**Output:** A single string ready to be piped to `claude -p` via stdin.

## Transformation Rules

The function applies three rules in order to assemble the prompt:

### 1. System message hoisting

All messages with `Role == "system"` are extracted and placed at the top of the output, each followed by `\n\n`. Their position in the input slice does not matter -- a system message at index 0 and one at index 3 both end up as prefix context.

### 2. Conversation history formatting

Non-system messages (user, assistant) are collected into a history slice. If there are 2 or more history messages, all messages except the last are formatted under a `Conversation so far:` header using the pattern `[role]: content\n`. A blank line separates the history block from the final message.

### 3. Last message as raw content

The last non-system message is appended as raw content without any role prefix. This is the "actual question" that Claude responds to. When there is only one non-system message, no history header is generated -- the content appears directly (after any system context).

### Example

Given input:
```go
[]Message{
    {Role: "system", Content: "ctx"},
    {Role: "user", Content: "a"},
    {Role: "assistant", Content: "b"},
    {Role: "user", Content: "c"},
}
```

Output string:
```
ctx

Conversation so far:
[user]: a
[assistant]: b

c
```

## Test Coverage Matrix

All tests are table-driven subtests under `TestBuildClaudeInput` using `t.Run`.

| Test Name | Input | Category | Verifies |
|-----------|-------|----------|----------|
| `empty slice` | `nil` | Boundary | Returns empty string, no panic |
| `single user` | 1 user msg | Boundary | No history header, raw content only |
| `empty content` | 1 user msg, `Content: ""` | Boundary | Returns empty string for empty content |
| `system only` | 1 system msg | System handling | System context with trailing `\n\n`, no history |
| `system + user` | system + user | System handling | Context prefix followed by raw question |
| `multiple systems` | 2 system + user | System handling | Both system messages hoisted, each with `\n\n` |
| `system mid-conversation` | user, system, user | System handling | System hoisted regardless of position in slice |
| `multi-turn 2 msgs` | user + assistant | Multi-turn | History header present, last msg is raw |
| `multi-turn 3 msgs` | user + assistant + user | Multi-turn | All prior messages in history, last is raw |
| `system + multi-turn` | system + 3 non-system | Multi-turn | Full pipeline: context + history + question |
| `content with newlines` | user with `\n` in content | Edge case | Newlines in content preserved as-is |

## Running the Tests

Run all provider tests:

```bash
go test ./internal/provider/...
```

Run only the `buildClaudeInput` tests:

```bash
go test ./internal/provider/ -run TestBuildClaudeInput
```

Run a specific subtest:

```bash
go test ./internal/provider/ -run TestBuildClaudeInput/system_mid-conversation
```

With verbose output:

```bash
go test -v ./internal/provider/ -run TestBuildClaudeInput
```

## Design Decisions

**System message hoisting.** System messages are unconditionally moved to the top of the prompt regardless of where they appear in the input slice. This matches how the agent loop injects system context (personality, memory) -- it always comes first in the Claude CLI prompt even if the store returns messages in interleaved order.

**Last message treatment.** The final non-system message is written as raw content without a `[role]:` prefix. This is intentional: `claude -p` treats its entire stdin as the user's question. By stripping the role prefix from the last message, the prompt reads naturally to the CLI. Prior messages get the prefix to distinguish speakers in the conversation history block.

**Package-level test.** The test file uses `package provider` (not `package provider_test`) to access the unexported `buildClaudeInput` function directly. This is appropriate because the function is a critical internal transformation that warrants direct unit testing beyond what integration tests through `Chat()` would cover.

**Pure function, no mocks needed.** `buildClaudeInput` has no side effects, no I/O, and no dependencies. Every test case is a straightforward input/output assertion, making the tests fast, deterministic, and easy to extend.
