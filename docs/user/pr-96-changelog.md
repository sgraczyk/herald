# PR #96 -- Test Coverage for Claude CLI Prompt Assembly

**Type:** Internal quality improvement
**User impact:** None -- no action required.

## What changed

Added 11 unit tests for the internal function that assembles conversation
messages into the prompt string sent to the Claude CLI. These tests cover
all assembly paths, including:

- System prompt inclusion
- Single and multi-turn conversations
- Mixed message roles (user, assistant, system)
- Edge cases such as empty message lists

## Why it matters

The prompt assembly logic is the bridge between Herald's conversation
history and the Claude CLI. Incorrect assembly could cause malformed
prompts, lost context, or unexpected responses. This test suite verifies
that messages are combined correctly under every supported scenario,
increasing confidence in the reliability of the Claude CLI provider.

## User action

None. This change is entirely internal. There are no new features,
behavior changes, or configuration updates. Herald continues to work
exactly as before.
