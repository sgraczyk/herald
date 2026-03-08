# Herald Configuration Guide

Herald uses two files for configuration: a JSON config file for structure and settings, and a `.env` file for secrets. This guide walks through setting up both.

## Quick Start

To get Herald running with minimal configuration, you need two files.

**config.json** -- the main configuration file:

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

**.env** -- your secrets:

```
TELEGRAM_TOKEN=your_bot_token_from_botfather
ALLOWED_USER_IDS=your_telegram_user_id
```

Then start Herald:

```bash
./herald
```

Herald will look for `config.json` in the current directory by default. Everything else (database path, history limit, log level) uses sensible defaults.

## Configuration File

### Location and Format

Herald expects a JSON file. By default it reads `config.json` from the working directory. The file must be valid JSON -- no trailing commas, no comments.

### Complete Field Reference

Here is every field you can set in the config file:

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
  "log_level": "info",
  "system_prompt": "You are a helpful assistant.",
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `telegram.token_env` | string | Yes | Name of the environment variable that holds your Telegram bot token. |
| `providers` | array | Yes | List of LLM providers. Herald tries them in order and falls back to the next one on failure. |
| `providers[].name` | string | Yes | A label for this provider (used in logs and the `/status` command). |
| `providers[].type` | string | Yes | Either `"claude-cli"` or `"openai"`. |
| `providers[].base_url` | string | For openai type | The API endpoint URL for OpenAI-compatible providers. |
| `providers[].model` | string | For openai type | The model identifier to request from the API. |
| `providers[].api_key_env` | string | For openai type | Name of the environment variable that holds the API key. |
| `store.path` | string | No | Path to the bbolt database file. Defaults to `"herald.db"`. |
| `http_port` | integer | No | Port for the health check HTTP endpoint. Must be 0--65535. Omit or set to 0 to disable. |
| `history_limit` | integer | No | Maximum number of messages to keep per chat. Defaults to `50`. |
| `log_level` | string | No | Logging verbosity. Defaults to `"info"`. Can be overridden by the `LOG_LEVEL` environment variable. |
| `system_prompt` | string | No | Custom system prompt sent to the LLM with every request. |
| `allowed_user_ids_env` | string | Yes | Name of the environment variable that holds the comma-separated list of allowed Telegram user IDs. |

## Environment Variables

Herald never stores secrets directly in the config file. Instead, the config file contains the *names* of environment variables, and Herald reads their values at startup.

### Required Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `TELEGRAM_TOKEN` | Your Telegram bot token from BotFather. | `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11` |
| `ALLOWED_USER_IDS` | Comma-separated Telegram user IDs that are allowed to interact with the bot. | `123456789,987654321` |

### Optional Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `CHUTES_API_KEY` | API key for Chutes.ai (or any OpenAI-compatible provider). Only needed if you configure an `openai` type provider. | `sk-abc123...` |
| `CLAUDE_TOKEN_EXPIRES` | Expiry date for your Claude CLI token. Shown in the `/health` endpoint response. | `2027-03-05` |
| `LOG_LEVEL` | Overrides the `log_level` value from the config file. Useful for temporary debugging without editing config. | `debug` |

### How Config References Environment Variables

The mapping works like this. In your config file, you write:

```json
{
  "telegram": {
    "token_env": "TELEGRAM_TOKEN"
  }
}
```

This tells Herald: "Read the Telegram token from the environment variable called `TELEGRAM_TOKEN`." You then set that variable in your `.env` file or your system environment:

```
TELEGRAM_TOKEN=your_actual_token_here
```

The same pattern applies to `api_key_env` for providers and `allowed_user_ids_env` for the user whitelist. The variable names are yours to choose -- the config file just tells Herald where to look.

### Using a .env File

For local development or simple deployments, create a `.env` file alongside your binary. For systemd deployments (like the default LXC setup), load environment variables through the service unit's `EnvironmentFile` directive, typically pointing to `/etc/herald/.env`.

Example `.env` file:

```
TELEGRAM_TOKEN=your_bot_token
CHUTES_API_KEY=your_api_key
ALLOWED_USER_IDS=123456789,987654321
CLAUDE_TOKEN_EXPIRES=2027-03-05
```

## Allowed Users

Herald only responds to Telegram users whose IDs appear in the whitelist. This prevents unauthorized access to your bot.

### Finding Your Telegram User ID

1. Open Telegram and start a chat with [@userinfobot](https://t.me/userinfobot).
2. Send any message. The bot replies with your user ID (a numeric value like `123456789`).

### Setting Up the Whitelist

In your config file, specify which environment variable holds the user IDs:

```json
{
  "allowed_user_ids_env": "ALLOWED_USER_IDS"
}
```

Then set the environment variable with a comma-separated list of IDs:

```
ALLOWED_USER_IDS=123456789,987654321
```

Spaces around commas are fine. Herald trims whitespace automatically:

```
ALLOWED_USER_IDS=123456789, 987654321, 555555555
```

If the environment variable is empty or unset, no users will be allowed to interact with the bot.

## Providers

Herald supports two types of LLM providers. You can configure multiple providers, and Herald will try them in the order they appear in your config file. If the first provider fails, it falls back to the next one.

### Claude CLI Provider

Uses the `claude` CLI tool in pipe mode. This is the primary provider and requires no API key (it uses your existing Claude subscription via the CLI).

```json
{
  "name": "claude",
  "type": "claude-cli"
}
```

Requirements:
- The `claude` CLI must be installed and authenticated on the machine running Herald.
- Node.js must be available (the Claude CLI runs on Node.js).

### OpenAI-Compatible Provider

Works with any API that follows the OpenAI chat completions format: Chutes.ai, Groq, OpenRouter, a local Ollama instance, and others.

```json
{
  "name": "chutes",
  "type": "openai",
  "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
  "model": "Qwen/Qwen2.5-VL-32B-Instruct",
  "api_key_env": "CHUTES_API_KEY"
}
```

Fields:
- `name` -- A label you choose. Shown in logs and `/status`.
- `base_url` -- The API base URL. Must include the path up to `/v1`.
- `model` -- The model identifier to send in requests.
- `api_key_env` -- Name of the environment variable holding your API key.

### Recommended Setup: Primary with Fallback

The most common configuration uses Claude CLI as the primary provider with an OpenAI-compatible service as a fallback:

```json
{
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
  ]
}
```

If Claude CLI fails or times out, Herald automatically tries Chutes.ai. If both fail, an error message is returned to the user.

## Defaults

When you omit optional fields, Herald uses these defaults:

| Field | Default Value |
|-------|---------------|
| `store.path` | `"herald.db"` (created in the working directory) |
| `history_limit` | `50` messages per chat |
| `log_level` | `"info"` |
| `http_port` | `0` (health endpoint disabled) |
| `system_prompt` | Empty (no custom system prompt) |

A minimal config file only needs `telegram`, at least one provider, and `allowed_user_ids_env`. Everything else is optional.

## Log Level

Herald supports a three-tier precedence system for log level:

1. **Lowest priority:** Built-in default (`"info"`).
2. **Medium priority:** The `log_level` field in your config file.
3. **Highest priority:** The `LOG_LEVEL` environment variable.

This means you can set a baseline in your config file and temporarily override it with an environment variable for debugging, without editing the config:

```bash
LOG_LEVEL=debug ./herald
```

Or in your `.env` file:

```
LOG_LEVEL=debug
```

To return to normal logging, remove the `LOG_LEVEL` variable and Herald falls back to whatever is in your config file (or `"info"` if that is also unset).

## Troubleshooting

### "read config file" error

```
read config file: open config.json: no such file or directory
```

Herald cannot find the config file. Make sure `config.json` exists in the directory where you run the binary, or specify the correct path.

### "parse config file" error

```
parse config file: invalid character '}' looking for beginning of value
```

Your JSON is malformed. Common causes:
- Trailing commas after the last item in an array or object.
- Missing quotes around string values.
- Comments in the JSON (JSON does not support comments).

Use a JSON validator to check your file.

### "invalid http_port" error

```
invalid http_port: -1
```

The `http_port` value must be between 0 and 65535. Set it to 0 or remove it entirely to disable the health endpoint.

### "parse allowed user IDs" error

```
parse allowed user IDs: invalid user ID "abc": expected integer
```

The `ALLOWED_USER_IDS` environment variable contains a value that is not a valid number. User IDs must be integers. Check for typos or stray characters.

### Bot does not respond to messages

If Herald starts successfully but ignores your messages:

- Verify your Telegram user ID is in the `ALLOWED_USER_IDS` list.
- Check that the `ALLOWED_USER_IDS` environment variable is actually set (not just defined in the config file -- the config file only names the variable).
- Make sure the variable name in `allowed_user_ids_env` matches the actual environment variable name exactly.

### Provider failures

If Herald cannot reach your LLM provider:

- For `claude-cli`: confirm that `claude` is installed, on the PATH, and authenticated. Try running `claude -p "hello"` manually.
- For `openai` type: verify the `base_url` is correct, the API key environment variable is set, and the model name is valid for that provider.
- Check Herald's logs (set `LOG_LEVEL=debug` for more detail).

### Secrets not resolving

If Herald starts but providers fail with authentication errors:

- Remember that `token_env` and `api_key_env` hold the *name* of the environment variable, not the secret itself. Writing `"token_env": "123456:ABC..."` is a common mistake. It should be `"token_env": "TELEGRAM_TOKEN"` with `TELEGRAM_TOKEN=123456:ABC...` in your environment.
- Verify the environment variable is exported and visible to the Herald process. For systemd services, use `systemctl show herald -p Environment` to check.
