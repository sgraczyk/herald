# Deployment

Operator guide for running Herald in production: user whitelist, systemd service, and credential management.

## User Whitelist

Herald restricts access to a list of allowed Telegram users. Only users whose IDs appear in the whitelist can interact with the bot. All other messages are silently ignored.

Herald **refuses to start** if no valid user IDs are configured, preventing accidental open-access deployment.

### Configuring Allowed Users

Set `ALLOWED_USER_IDS` in your environment file (`/etc/herald/.env`):

```
ALLOWED_USER_IDS=123456789,987654321
```

Each ID must be a positive integer. To find your Telegram user ID, message [@userinfobot](https://t.me/userinfobot) on Telegram.

### Validation Rules

- **Positive integers** are accepted
- **Zero and negative values** are silently filtered out (not valid Telegram user IDs)
- **Whitespace around IDs** is trimmed automatically
- **Empty or all-invalid** results in a startup failure

| Scenario | Outcome |
|----------|---------|
| `ALLOWED_USER_IDS=123456,789012` | Bot starts, two authorized users |
| `ALLOWED_USER_IDS=123456` | Bot starts, single authorized user |
| `ALLOWED_USER_IDS=` (empty) | Bot refuses to start |
| `ALLOWED_USER_IDS` not set | Bot refuses to start |
| `ALLOWED_USER_IDS=0,-5` | Bot refuses to start (no valid IDs) |
| `ALLOWED_USER_IDS=0,123456` | Bot starts, only `123456` is authorized |

### Verifying Configuration

```bash
sudo systemctl restart herald
sudo systemctl status herald
```

If the service failed to start, check logs:

```bash
journalctl -u herald --no-pager -n 20
```

Look for: `create telegram adapter: no valid allowed user IDs configured`
