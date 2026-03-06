# Health Endpoint Shows Active Provider

Herald exposes an HTTP health endpoint that returns status information as JSON.
One of the fields in that response is `provider`, which tells you which LLM
provider is currently handling requests.

Previously, the `provider` field always showed the provider that was active when
Herald started, even if you switched providers at runtime using the `/model`
Telegram command. Now the `/health` endpoint always reflects the provider that is
**currently active**.

## Checking the Health Endpoint

If you have `http_port` configured in your `config.json`, you can query the
health endpoint directly:

```bash
curl http://192.168.0.107:8080/health
```

The response includes several fields:

```json
{
  "status": "ok",
  "version": "0.2.1",
  "uptime": "3h25m12s",
  "provider": "claude",
  "claude_status": "ok",
  "token_expires": "2026-04-01"
}
```

The `provider` field shows whichever provider is actively serving your requests
right now.

## Switching Providers

You can switch providers at any time using the `/model` command in Telegram.
After switching, the health endpoint immediately reports the new provider name.

For example, if you start Herald with the default provider (`claude`) and then
switch to `openai` via `/model`, a subsequent call to `/health` will show
`"provider": "openai"` instead of the old `"provider": "claude"`.

## Quick Reference

| Scenario | `provider` field shows |
|---|---|
| Herald starts with default provider | The default provider name |
| You switch via `/model` in Telegram | The newly selected provider name |
| You switch again | The latest provider name |
| You restart Herald | The default provider name (resets to startup default) |
