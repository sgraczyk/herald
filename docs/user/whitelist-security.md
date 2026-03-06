# User Whitelist Security

Herald restricts access to a list of allowed Telegram users. Only users whose
IDs appear in the whitelist can interact with the bot. All other messages are
silently ignored.

Starting with the fix in PR #90, Herald **refuses to start** if no valid user
IDs are configured. This prevents an accidental open-access deployment where
anyone on Telegram could use your bot.

## Configuring Allowed Users

Set the `ALLOWED_USER_IDS` variable in your environment file
(`/etc/herald/.env`) to a comma-separated list of Telegram user IDs:

```
ALLOWED_USER_IDS=123456789,987654321
```

Each ID must be a **positive integer**. Telegram assigns a unique numeric ID to
every user account, and these IDs are always positive.

### Finding Your Telegram User ID

Message [@userinfobot](https://t.me/userinfobot) on Telegram. It will reply
with your numeric user ID. Use that value in `ALLOWED_USER_IDS`.

## What Counts as a Valid ID

Herald applies the following rules when parsing the whitelist:

- **Positive integers** (e.g., `123456789`) are accepted.
- **Zero and negative values** are silently filtered out. These are not valid
  Telegram user IDs.
- **Whitespace around IDs** is trimmed automatically.
- **Empty or missing values** result in a startup failure if no valid IDs
  remain after filtering.

For example, `ALLOWED_USER_IDS=0,-100,123456` is treated as a single-entry
whitelist containing only `123456`.

## What Happens Without Valid IDs

If `ALLOWED_USER_IDS` is unset, empty, or contains only invalid values (zero
or negative numbers), Herald will not start. You will see this error:

```
create telegram adapter: no valid allowed user IDs configured
```

This is intentional. Running without a whitelist would allow any Telegram user
to send messages to your bot and consume LLM resources.

## Verifying Your Configuration

After editing `/etc/herald/.env`, restart Herald and check that it starts
successfully:

```bash
sudo systemctl restart herald
sudo systemctl status herald
```

If the service is active and running, your whitelist is valid. If it failed to
start, check the logs for the error message above:

```bash
journalctl -u herald --no-pager -n 20
```

## Quick Reference

| Scenario | Outcome |
|---|---|
| `ALLOWED_USER_IDS=123456,789012` | Bot starts, only those two users can interact |
| `ALLOWED_USER_IDS=123456` | Bot starts, single authorized user |
| `ALLOWED_USER_IDS=` (empty) | Bot refuses to start |
| `ALLOWED_USER_IDS` not set | Bot refuses to start |
| `ALLOWED_USER_IDS=0,-5` | Bot refuses to start (no valid IDs after filtering) |
| `ALLOWED_USER_IDS=0,123456` | Bot starts, only `123456` is authorized |
