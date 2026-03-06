# Logging

Herald uses structured logging to help you monitor its operation and troubleshoot issues. Logs are written to stderr.

## Log Levels

Herald supports four log levels, from most verbose to least:

| Level   | What it shows                                                    |
|---------|------------------------------------------------------------------|
| `debug` | Detailed internal events -- typing indicator failures, memory extraction attempts, LLM response parsing. Useful when troubleshooting a specific problem. |
| `info`  | Normal operation events -- startup, shutdown, provider selection. This is the default. |
| `warn`  | Non-fatal issues -- invalid config values ignored, provider retries. Herald keeps running. |
| `error` | Failures that need attention -- provider errors, storage failures. |

Each level includes all messages from the levels below it. Setting `warn` shows both `warn` and `error` messages.

## Configuration

### config.json

Set the `log_level` field in your `config.json`:

```json
{
  "log_level": "debug"
}
```

Valid values: `debug`, `info`, `warn`, `error`. Defaults to `info` if omitted.

### Environment variable

Set `LOG_LEVEL` to override the config file value:

```bash
LOG_LEVEL=debug ./herald
```

Or add it to your `.env` file:

```
LOG_LEVEL=debug
```

The environment variable always takes priority over `config.json`.

## Log Output Format

Herald automatically picks the output format based on how it is running:

| Context                      | Format         | Example                                                        |
|------------------------------|----------------|----------------------------------------------------------------|
| Terminal (interactive)       | Text (key=value) | `time=2026-03-06T10:00:00Z level=INFO msg="herald starting"` |
| systemd / pipe / file redirect | JSON           | `{"time":"2026-03-06T10:00:00Z","level":"INFO","msg":"herald starting"}` |

This is detected automatically. No configuration is needed.

If you want JSON output while running in a terminal, redirect stderr:

```bash
./herald 2>herald.log
```

## Troubleshooting

**I want to see more detail about what Herald is doing.**
Set `LOG_LEVEL=debug`. This shows typing indicator failures, memory extraction attempts, and LLM response parse failures.

**Logs are too noisy.**
Set `LOG_LEVEL=warn` to see only warnings and errors, or `LOG_LEVEL=error` for failures only.

**I want JSON logs during development.**
Pipe stderr to a file or another program. Herald switches to JSON automatically when stderr is not a terminal:

```bash
./herald 2>herald.log
# or
./herald 2>&1 | jq .
```

**I changed the log level but nothing happened.**
Check if `LOG_LEVEL` is set in your environment. It overrides `config.json`. Run `echo $LOG_LEVEL` to verify. Also confirm you restarted Herald after the change.
