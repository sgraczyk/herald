# Embedded Config Defaults (PR #109, Issue #106)

## What Changed

Starting with this release, Herald ships with built-in configuration defaults compiled directly into the binary. You no longer need a `config.json` file on disk to run Herald. Just set your environment variables (via `.env`) and start the binary -- it works out of the box.

Previously, Herald required a `config.json` file alongside the binary. If the file was missing, Herald would fail to start. This was an extra step during deployment and an easy thing to forget when setting up a fresh instance.

Now, the example configuration (`config.json.example`) is embedded into the Herald binary at build time. If no `config.json` is found, Herald automatically uses these sensible defaults.

## How Config Loading Works

Herald loads configuration in this order:

1. **Check for `config.json` on disk** (or whatever path you pass with `--config`).
2. **If the file exists**, use it -- exactly as before. Nothing changes for existing setups.
3. **If the file does not exist**, fall back to the built-in defaults that are compiled into the binary.

The built-in defaults include:

- Telegram token read from the `TELEGRAM_TOKEN` environment variable
- Claude CLI as the primary provider
- Chutes.ai as the fallback provider (API key from `CHUTES_API_KEY` environment variable)
- Database stored at `herald.db`
- Health endpoint on port 8080
- 50 messages of conversation history
- Log level set to `info`
- Allowed user IDs read from the `ALLOWED_USER_IDS` environment variable

Secrets (API keys, tokens, user IDs) are never embedded. They are always resolved from environment variables at startup, as before.

## Common Scenarios

### Fresh Install

1. Download the Herald binary from the GitHub release.
2. Create your `.env` file with the required secrets:
   ```
   TELEGRAM_TOKEN=your-telegram-bot-token
   CHUTES_API_KEY=your-chutes-api-key
   ALLOWED_USER_IDS=123456789
   ```
3. Run Herald. No `config.json` needed.

That is it. Herald starts with the built-in defaults and reads secrets from your environment.

### Upgrading from a Previous Version

**No action required.** This change is fully backward compatible.

If you already have a `config.json` file, Herald continues to use it. The embedded defaults only activate when no config file is found on disk. Your existing setup works exactly as before.

### Customizing Configuration

If the defaults do not fit your needs, create a `config.json` file and Herald will use it instead:

1. Find the example config. You can get it from:
   - The `config.json.example` file in the GitHub repository
   - The release assets on the GitHub Releases page
2. Copy it to `config.json` in the same directory as the Herald binary (or wherever you point `--config`).
3. Edit it to match your needs.
4. Restart Herald.

For example, to change the history limit from 50 to 100, or to add a custom system prompt, create a `config.json` with those values.

### Resetting to Defaults

If you have customized your config and want to go back to the built-in defaults, simply delete (or rename) your `config.json` file and restart Herald. It will pick up the embedded defaults automatically.

```
rm config.json
systemctl restart herald
```

### Using a Custom Config Path

You can point Herald at a config file in any location using the `--config` flag:

```
./herald --config /etc/herald/config.json
```

If the file at that path does not exist, Herald falls back to the built-in defaults.

## Migration Notes

- **Backward compatible.** Existing `config.json` files are used as before.
- **No changes to `.env` handling.** Secrets are still read from environment variables.
- **No changes to CLI flags.** The `--config` / `-c` flag still works.
- **No changes to behavior.** If you already have a config file, nothing changes for you.

The only difference is that Herald no longer fails on startup when `config.json` is absent. Everything else stays the same.
