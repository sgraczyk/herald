# Logging

Herald uses Go's standard `log/slog` package for structured logging. Logs are written to stderr.

## Log Levels

| Level | What It Shows |
|-------|---------------|
| `debug` | Detailed internals -- typing indicator failures, memory extraction attempts, LLM response parsing, message validation (e.g., ignored messages with nil sender) |
| `info` | Normal operation -- startup, shutdown, provider selection. **Default.** |
| `warn` | Non-fatal issues -- invalid config values ignored, provider retries, unauthorized user rejected |
| `error` | Failures needing attention -- provider errors, storage failures |

Each level includes all messages from the levels below it.

## Configuration

### config.json

```json
{
  "log_level": "debug"
}
```

Valid values: `debug`, `info`, `warn` (or `warning`), `error`. Defaults to `info` if omitted or unrecognized.

### Environment Variable Override

```bash
LOG_LEVEL=debug ./herald
```

The environment variable always takes priority over `config.json`. Useful for temporary debugging without editing config.

### systemd Override

```bash
# Temporarily enable debug logging
systemctl set-environment LOG_LEVEL=debug
systemctl restart herald

# Revert
systemctl unset-environment LOG_LEVEL
systemctl restart herald
```

## Output Format

Herald automatically selects the format based on how it is running:

| Context | Format | Example |
|---------|--------|---------|
| Terminal (interactive) | Text (key=value) | `time=... level=INFO msg="herald starting"` |
| systemd / pipe / file redirect | JSON | `{"time":"...","level":"INFO","msg":"herald starting"}` |

Detection is automatic. To force JSON output in a terminal, redirect stderr:

```bash
./herald 2>herald.log
```

## Structured Fields

All log entries use typed key-value fields. Common fields:

| Field | Type | Used For |
|-------|------|----------|
| `chat_id` | int64 | Telegram chat identifier |
| `user_id` | int64 | Telegram user identifier (auth rejection logs) |
| `provider` | string | Provider name (`"claude"`, `"chutes"`) |
| `error` | string | Error message |
| `port` | int | HTTP port for health endpoint |
| `version` | string | Build version string |
| `messages_removed` | int | Number of old messages trimmed by token budget |
| `tokens_used` | int | Estimated tokens in the kept messages |
| `token_budget` | int | Configured token budget |
| `attempt` | int | Retry attempt number (provider retry logs) |
| `backoff` | duration | Backoff delay before next retry |

## Metrics Summary

Herald logs a `"metrics summary"` entry at INFO level every hour and once on shutdown. This line includes all runtime counters as structured fields:

| Field | Type | Meaning |
|-------|------|---------|
| `messages_received` | int64 | Total incoming messages processed |
| `responses_sent` | int64 | Total successful responses |
| `messages_failed` | int64 | Total messages where all providers failed |
| `provider_calls` | string (JSON) | Successful calls per provider |
| `provider_errors` | string (JSON) | Failed calls per provider |
| `provider_latency_ms_total` | string (JSON) | Cumulative latency in ms per provider |
| `provider_latency_ms_count` | string (JSON) | Number of latency observations per provider |
| `memory_extraction_successes` | int64 | Successful memory extractions |
| `memory_extraction_failures` | int64 | Failed memory extractions |
| `uptime_seconds` | float64 | Seconds since process start |

All counters reset on process restart.

## Troubleshooting

**Want more detail?** Set `LOG_LEVEL=debug` to see typing indicator failures, memory extraction attempts, and response parse failures.

**Logs too noisy?** Set `LOG_LEVEL=warn` for only warnings and errors, or `LOG_LEVEL=error` for failures only.

**Changed log level but nothing happened?** Check if `LOG_LEVEL` is set in your environment (it overrides `config.json`). Run `echo $LOG_LEVEL` to verify. Restart Herald after changes.
