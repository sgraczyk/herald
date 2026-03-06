# Long Responses Now Arrive in Full

Previously, if Herald's response was too long, it could fail to send entirely. That is fixed now. Long responses are automatically split into multiple messages so nothing gets lost.

## What changed

Telegram has a size limit on individual messages. When Herald generated a response that exceeded that limit, the message would fail to deliver. Now, Herald detects oversized responses and splits them into a series of shorter messages before sending. You always get the complete answer.

## How messages are split

Herald splits responses at natural reading points so the conversation still flows normally:

- **Paragraph breaks** are preferred -- Herald will try to end a message between paragraphs first.
- **Line breaks** are used next if a paragraph break does not fit.
- **Word boundaries** are the last resort before any hard cut.

This means you will not see words or sentences sliced in half. Each message reads as a coherent piece of the full response.

## Formatting is preserved

Bold, italic, code blocks, links, and other formatting carry over correctly between messages. If a code block or bold section spans a split point, Herald closes the formatting at the end of one message and reopens it at the start of the next. You will not see broken or missing formatting.

## What to expect in practice

- **Messages arrive in order.** If a response is split into three parts, they appear as message 1, 2, 3 in your chat.
- **Each part stays within Telegram's limit.** Every message chunk is at most 4096 bytes, which is the maximum Telegram allows.
- **Fallback on errors.** If a chunk fails to send with formatting, Herald retries it as plain text. Already-delivered chunks are not affected.
- **Short responses are unchanged.** If a response fits in a single message, nothing is different from before.

## Do I need to do anything?

No. This works automatically. There is nothing to configure or enable. Just chat with Herald as usual and long answers will arrive as multiple messages.

## Technical details

For implementation details on how the message splitter works, see [format.Split Technical Documentation](../format-split.md).
