# Unknown Commands

Herald recognizes a set of built-in commands that start with a slash (`/`). When
you type a slash command that Herald does not recognize, the full message is
passed to the AI as a regular message. The AI then interprets your intent and
responds accordingly.

Previously, unrecognized commands lost the slash prefix and command word. For
example, typing `/summarize this document` would only send `this document` to
the AI, making it impossible for the AI to understand what you wanted. This was
fixed in PR #91.

## Recognized Commands

Herald handles the following commands directly, without sending them to the AI:

| Command | What It Does |
|---|---|
| `/clear` | Resets conversation history for the current chat |
| `/model` | Switches between LLM providers (Claude CLI, Chutes.ai) |
| `/status` | Shows bot uptime, active provider, and message count |
| `/remember` | Saves a fact to long-term memory |
| `/forget` | Removes a specific memory |
| `/memories` | Lists all stored memories for the current chat |

These commands are processed by Herald itself and produce a direct response.

## Unrecognized Commands

Any slash command not in the list above is treated as a regular message. The
full text, including the slash and command word, is sent to the AI. This means
you can type things like:

- `/summarize this document` -- the AI receives the entire message and can
  interpret "summarize" as your intent.
- `/translate hello world to French` -- the AI sees the full request and can
  act on it.
- `/foo bar baz` -- the AI receives `/foo bar baz` and responds as best it can.

The AI does not execute these as bot commands. It simply reads your message and
responds like it would to any other text.

## Quick Reference

| What You Type | What Happens |
|---|---|
| `/clear` | Herald clears conversation history (recognized command) |
| `/status` | Herald shows bot status (recognized command) |
| `/summarize the last few messages` | AI receives the full text and responds |
| `/explain quantum computing` | AI receives the full text and responds |
| `/xyz` | AI receives `/xyz` and responds |
