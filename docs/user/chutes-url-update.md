# Chutes.ai URL Update

## What Changed

Chutes.ai moved from a single shared API endpoint to per-model subdomain URLs. Herald's configuration needs to be updated to use the new URL format.

The model has also been updated from `Qwen/Qwen2.5-VL-72B-Instruct` to `Qwen/Qwen2.5-VL-32B-Instruct`. Both are vision-language models that support text and image conversations.

## How to Update

Edit the `providers` section of your `config.json` so the Chutes.ai entry matches the following:

```json
{
  "name": "chutes",
  "type": "openai",
  "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
  "model": "Qwen/Qwen2.5-VL-32B-Instruct",
  "api_key_env": "CHUTES_API_KEY"
}
```

Then restart the service:

```bash
systemctl restart herald
```

No other changes are needed. Your API key, Telegram token, allowed user IDs, and all other settings stay the same. No binary rebuild is required.

## What Stays the Same

- All features work as before: text messages, image understanding, conversation history, and all commands (`/clear`, `/model`, `/status`).
- The Claude CLI provider is unaffected.
- The fallback chain still works: Claude CLI first, Chutes.ai as backup.
- Conversation history and memories are preserved.

## Troubleshooting

### Messages fail with "temporarily unavailable"

The old URL (`https://api.chutes.ai/v1`) may no longer work. Update `base_url` and `model` as shown above, then restart the service.

### Startup log shows "provider unreachable"

Check that the URL in your config is spelled correctly. The subdomain format is `chutes-` followed by the model name with slashes replaced by hyphens and dots removed. The correct URL for the recommended model is:

```
https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1
```

### Startup log shows "provider auth failure"

Your API key is being rejected. Verify that `CHUTES_API_KEY` in your `.env` file contains a valid Chutes.ai API key. This is unrelated to the URL change but worth checking while updating your config.

### Using a different Chutes.ai model

You can use any model available on Chutes.ai. Find the model's subdomain URL on the Chutes.ai dashboard and update both `base_url` and `model` in your config. For vision (image) support, choose a vision-language model (look for "VL" in the model name).
