# Configuration

Herald uses a JSON config file for structure and settings, and a `.env` file for secrets. When no config file exists on disk, built-in defaults (embedded from `config.json.example`) are used automatically.

## Quick Start

**config.json** (optional -- built-in defaults work without it):

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  },
  "providers": [
    {
      "name": "claude",
      "type": "claude-cli"
    }
  ],
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

**.env** (required -- secrets are always from environment variables):

```
TELEGRAM_TOKEN=your_bot_token_from_botfather
ALLOWED_USER_IDS=your_telegram_user_id
```

Then run `./herald`. Herald looks for `config.json` in the current directory by default.

## Field Reference

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  },
  "providers": [
    {
      "name": "claude",
      "type": "claude-cli"
    },
    {
      "name": "chutes",
      "type": "openai",
      "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
      "model": "Qwen/Qwen2.5-VL-32B-Instruct",
      "api_key_env": "CHUTES_API_KEY"
    }
  ],
  "store": {
    "path": "herald.db"
  },
  "http_port": 8080,
  "history_limit": 50,
  "history_token_budget": 8000,
  "max_retries": 1,
  "log_level": "info",
  "system_prompt": "You are a helpful assistant.",
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `telegram.token_env` | string | Yes | -- | Env var name holding the Telegram bot token |
| `providers` | array | Yes | -- | LLM providers in fallback order |
| `providers[].name` | string | Yes | -- | Display label (used in logs and `/status`) |
| `providers[].type` | string | Yes | -- | `"claude-cli"` or `"openai"` |
| `providers[].base_url` | string | For openai | -- | API endpoint URL (must include `/v1`) |
| `providers[].model` | string | For openai | -- | Model identifier |
| `providers[].api_key_env` | string | For openai | -- | Env var name holding the API key |
| `store.path` | string | No | `"herald.db"` | Path to the bbolt database file |
| `http_port` | integer | No | `0` (disabled) | Health check HTTP endpoint port (0--65535) |
| `history_limit` | integer | No | `50` | Max messages per chat |
| `history_token_budget` | integer | No | `8000` | Estimated token budget for conversation history. Oldest messages are dropped when history exceeds this budget. Negative value disables token trimming. |
| `max_retries` | integer | No | `1` | Retries per provider for transient errors (timeouts, server errors). Set to `0` to disable. |
| `log_level` | string | No | `"info"` | Logging verbosity (see [Logging](logging.md)) |
| `system_prompt` | string | No | (built-in) | Custom system prompt sent to the LLM |
| `allowed_user_ids_env` | string | Yes | -- | Env var name holding comma-separated allowed Telegram user IDs |

## Environment Variables

Secrets are never stored in the config file. The config contains env var **names**; Herald reads their values at startup via `os.Getenv`.

| Variable | Required | Purpose |
|----------|----------|---------|
| `TELEGRAM_TOKEN` | Yes | Telegram bot token from BotFather |
| `ALLOWED_USER_IDS` | Yes | Comma-separated Telegram user IDs (spaces around commas are fine) |
| `CHUTES_API_KEY` | If using openai provider | API key for the OpenAI-compatible provider |
| `CLAUDE_TOKEN_EXPIRES` | No | Expiry date shown in `/health` endpoint |
| `LOG_LEVEL` | No | Overrides `log_level` from config (useful for temporary debugging) |

For systemd deployments, load env vars via the service unit's `EnvironmentFile` directive (typically `/etc/herald/.env`).

## Providers

### Claude CLI

Uses the `claude` CLI in pipe mode. No API key needed -- uses your existing Claude subscription.

```json
{ "name": "claude", "type": "claude-cli" }
```

Requires `claude` CLI installed, authenticated, and on PATH. Node.js must be available.

### OpenAI-Compatible

Works with any OpenAI chat completions API: Chutes.ai, Groq, OpenRouter, local Ollama, etc.

```json
{
  "name": "chutes",
  "type": "openai",
  "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
  "model": "Qwen/Qwen2.5-VL-32B-Instruct",
  "api_key_env": "CHUTES_API_KEY"
}
```

### Vision Support

| Provider | Images | Notes |
|----------|:------:|-------|
| OpenAI-compatible | Yes | Requires vision-capable model (`VL` suffix) |
| Claude CLI | No | Pipe mode is text-only; images fall back to OpenAI provider |

### Recommended Setup

Claude CLI as primary, OpenAI-compatible as fallback. If Claude fails or times out, Herald automatically tries the next provider.

### Startup Validation

Herald validates providers at startup, logging warnings for unreachable or misconfigured services. Validation is advisory only -- it never blocks startup.

**Healthy:**
```
INFO  provider reachable  provider=chutes
INFO  provider reachable  provider=claude  path=/usr/local/bin/claude
```

**Auth failure:**
```
WARN  provider auth failure  provider=chutes  status=401
```
Fix: Check `CHUTES_API_KEY` in `/etc/herald/.env`.

**Unreachable:**
```
WARN  provider unreachable  provider=chutes  url=...  error=...
```
Fix: Check network. Try `curl {baseURL}/models` from the container. Herald retries on each message.

## System Prompt

The optional `system_prompt` field replaces the built-in default prompt. When empty or absent, Herald uses a hardcoded default that includes Telegram formatting rules. See [Custom Personality](features.md#custom-personality) for usage guide.

- **Full replacement:** The custom prompt completely replaces the default (does not merge).
- **Memory injection:** User memories are appended to whichever prompt is active.
- **Length warning:** Prompts longer than 4000 characters log a warning at startup but are not rejected.

## Embedded Defaults

Herald embeds `config.json.example` into the binary at build time via `//go:embed`. When no config file is found on disk, these defaults are used automatically. The file on disk always takes precedence.

- **Fresh install:** Just create `.env` with secrets and run Herald. No `config.json` needed.
- **Existing setup:** Completely unaffected. Your `config.json` is used as before.
- **Reset to defaults:** Delete `config.json` and restart Herald.
- **`--config` flag:** If the specified file doesn't exist, falls back to embedded defaults.

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `read config file: open config.json: no such file or directory` | Config file not found | Ensure `config.json` exists in the working directory, or use embedded defaults |
| `parse config file: invalid character...` | Malformed JSON | Check for trailing commas, missing quotes, or comments |
| `invalid http_port: -1` | Port out of range | Must be 0--65535 (0 disables health endpoint) |
| `parse allowed user IDs: invalid user ID "abc"` | Non-numeric user ID | User IDs must be integers |
| Bot ignores messages | User ID not whitelisted | Verify your ID is in `ALLOWED_USER_IDS` and the env var is set |
| Provider auth errors | Secret not resolving | `token_env` holds the var **name**, not the secret itself |
| `provider auth failure status=401` | API key invalid or expired | Update API key in `.env`, restart |
| `provider unreachable` | Network issue or API down | Wait for recovery; Herald retries per message |
| `claude CLI not found on PATH` | Claude Code CLI not installed | Install Claude Code CLI (requires Node.js) |
| `no providers configured` | Empty providers array | At least one provider must be in config |
| Photos fail after update | Model lacks vision support | Confirm vision-capable model in config (look for `VL` suffix) |
