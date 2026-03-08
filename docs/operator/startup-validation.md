# Startup Validation -- Operator Guide

Guide for deploying PR #103, which restores image understanding and adds startup provider validation.

**Related:** Issue #102

---

## What Changed

Herald v0.3.0 shipped with a dead AI model on Chutes.ai. The configured model was removed upstream, and its replacement was text-only, causing all photo messages to fail.

This update:

1. **Replaces the dead model** with `Qwen/Qwen2.5-VL-32B-Instruct`, a vision-language model that handles both text and images.
2. **Adds startup validation** so Herald checks provider reachability at boot and logs the results.

No changes to message processing, conversation handling, or history management.

## Configuration Update

Update the `model` field in the OpenAI provider section of `/etc/herald/config.json`:

```json
{
  "openai": {
    "name": "chutes",
    "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
    "api_key_env": "CHUTES_API_KEY",
    "model": "Qwen/Qwen2.5-VL-32B-Instruct"
  }
}
```

The previous model (`Qwen/Qwen3-235B-A22B-Instruct-2507`) was text-only. The new model (`VL` = Vision-Language) accepts image payloads.

## Deployment Steps

1. Build or download the new binary:

   ```bash
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o herald ./cmd/herald
   ```

2. Copy the binary to CT 107:

   ```bash
   scp herald-linux-amd64 root@192.168.0.107:/usr/local/bin/herald
   ```

3. Update config on CT 107:

   ```bash
   ssh root@192.168.0.107
   vi /etc/herald/config.json
   # Change model to: Qwen/Qwen2.5-VL-32B-Instruct
   # Change base_url to: https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1
   ```

4. Restart the service:

   ```bash
   systemctl restart herald
   ```

5. Verify startup:

   ```bash
   journalctl -u herald -n 20 --no-pager
   ```

6. Test vision by sending a photo to the bot in Telegram.

## Startup Log Messages

Herald now logs a status line per provider at startup.

### Healthy startup

```
INFO  provider reachable  provider=chutes
INFO  provider reachable  provider=claude  path=/usr/local/bin/claude
```

No action needed.

### Warning: auth failure

```
WARN  provider auth failure  provider=chutes  status=401
```

The API key is invalid or expired. Check `CHUTES_API_KEY` in `/etc/herald/.env` and verify it in your Chutes.ai account.

### Warning: provider unreachable

```
WARN  provider unreachable  provider=chutes  url=https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1  error=...
```

Network issue or service outage. Try `curl https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1/models` from the container. Herald will still start and retry on each message.

### Warning: Claude CLI not found

```
WARN  claude CLI not found on PATH  error=...
```

The `claude` binary is not installed or not on PATH. Verify with `which claude`. If you do not use the Claude CLI provider, this warning can be ignored.

### Important

Warnings never prevent Herald from starting. The bot always starts, even if all providers are unreachable. If a provider recovers before the first message, it will work normally.

## Troubleshooting

| Problem | Log message | Fix |
|---------|-------------|-----|
| API key expired | `provider auth failure status=401` | Update `CHUTES_API_KEY` in `/etc/herald/.env`, restart service |
| Chutes.ai is down | `provider unreachable` | Wait for recovery. Herald retries on each message. |
| Claude CLI missing | `claude CLI not found on PATH` | Install Claude Code CLI (requires Node.js). Run `which claude` to verify. |
| Photos still fail after update | No error at startup | Confirm `config.json` has the new model name. Confirm service was restarted after config edit. |
| Bot does not start | `no providers configured` | At least one provider must be in `config.json`. Verify file exists and is valid JSON. |

## Vision Support Matrix

| Provider | Image support | Notes |
|----------|:---:|-------|
| OpenAI-compatible (Chutes.ai) | Yes | Requires a vision-capable model (`VL` suffix) |
| Claude CLI (`claude -p`) | No | Pipe mode is text-only. Image messages fall back to the OpenAI provider. |

If the OpenAI-compatible provider is down, image messages will fail because Claude CLI cannot handle them. Text messages continue to work through either provider via the fallback chain.
