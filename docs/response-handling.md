# Response Handling

Herald handles two layers of response size management: an upstream size cap on provider responses, and automatic message splitting for Telegram's per-message limit.

## Response Size Limit (10 MB)

Herald limits responses from external AI providers to 10 MB. If a provider sends a response larger than this, Herald rejects it and tries the next provider in the fallback chain. If all providers fail, you receive a short error message in Telegram.

A typical AI chat response is a few kilobytes -- well under 1% of this limit. The threshold exists as a safety net against abnormal upstream behavior on Herald's 512 MB container. No configuration needed; the protection is automatic. For implementation details (LimitReader+1 technique, error ordering), see [OpenAI Response Size Limit](openai-response-size-limit.md).

## Message Splitting

Telegram limits individual messages to 4096 bytes. When Herald generates a response exceeding this limit, it automatically splits the response into multiple messages.

### Split Strategy

Messages are split at natural reading points, in order of preference:

1. **Paragraph breaks** -- preferred split point
2. **Line breaks** -- used if no paragraph break fits
3. **Word boundaries** -- last resort before hard cut

### Formatting Preservation

Bold, italic, code blocks, links, and other formatting carry over between split messages. If a formatted span crosses a split point, Herald closes the formatting at the end of one message and reopens it at the start of the next.

### Behavior

- Messages arrive in order (part 1, 2, 3, ...)
- Each chunk is at most 4096 bytes
- If a chunk fails to send with formatting, Herald retries as plain text
- Short responses (under 4096 bytes) are sent as a single message, unchanged

For implementation details on the splitter, see [format.Split Technical Documentation](format-split.md).
