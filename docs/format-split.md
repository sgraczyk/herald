# format.Split -- Splitting Long Telegram HTML Messages

## Overview

Telegram's Bot API enforces a hard limit of **4096 bytes** per `sendMessage` call. When the LLM produces a long response, the formatted HTML can exceed this limit and the API rejects it.

`format.Split` divides an HTML string into chunks that each fit within the byte limit while preserving valid HTML structure. Tags that span a split boundary are closed at the end of one chunk and reopened at the start of the next.

Package: `internal/format` | File: `split.go` | PR: #64 | Issue: #57

## API Reference

```go
func Split(html string, maxLen int) []string
```

**Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `html` | `string` | Telegram HTML string (output of `format.TelegramHTML()`). |
| `maxLen` | `int` | Maximum byte length per chunk. If <= 0, defaults to 4096. |

**Returns:** `[]string` -- slice of HTML chunks. Each chunk is at most `maxLen` bytes.

**Guarantees:**

- Always returns at least one element, even for empty input.
- Every chunk is at most `maxLen` bytes. No exceptions.
- All content from the input is present across the returned chunks (no data loss).
- Blank/whitespace-only chunks are filtered out.
- HTML tags from the tracked set are properly closed and reopened across boundaries.

## Algorithm

Each iteration of the main loop produces one chunk:

### 1. Prefix reconstruction

Render carried-over open tags as a prefix string. For example, if the previous chunk ended inside `<pre><code>`, the prefix is `<pre><code>`. The full opening tag text is preserved, including attributes (e.g., `<a href="...">`).

### 2. Budget calculation

```
budget = maxLen - len(prefix)
```

Clamped to a minimum of 1 byte to guarantee forward progress.

### 3. Boundary detection

`bestBoundary(html, budget)` finds the best split position using a priority cascade:

| Priority | Delimiter | Behavior |
|----------|-----------|----------|
| 1 (best) | `\n\n` | Split after paragraph break |
| 2 | `\n` | Split after newline |
| 3 | `" "` | Split after space |
| 4 (worst) | Hard cut | Split at byte limit, adjusted for tag/entity safety |

`lastSafeIndex` scans forward through the text, tracking `<`/`>` nesting, and ignores delimiters found inside HTML tags. This prevents splitting on whitespace that is part of tag attributes.

### 4. Safety adjustments (hard cut only)

When no whitespace boundary is found:

- **`avoidTagSplit`** -- scans backward up to 500 bytes from the cut point. If it finds `<` before `>`, it moves the cut to before the `<`.
- **`avoidEntitySplit`** -- scans backward up to 12 bytes. If it finds `&` before any terminator (`;`, ` `, `\n`, or `<`), it moves the cut to before the `&`.

### 5. Tag stack computation

`tagStack(carry, content)` parses all HTML tags in the chunk content, maintaining a stack:
- Opening tags (`<b>`, `<pre>`, etc.) push onto the stack.
- Closing tags (`</b>`, `</pre>`) pop by name, scanning from the top.
- Only Telegram-supported tags are tracked (see below). Others pass through unmodified.

### 6. Suffix generation

Open tags remaining on the stack are closed in **reverse order** (innermost first):

```
stack: [pre, code]  -->  suffix: "</code></pre>"
```

### 7. Shrink loop

If `len(prefix) + splitAt + len(suffix) > maxLen`, the algorithm reduces `splitAt` by the overage and re-runs boundary detection and tag stack computation. This repeats until the assembled chunk fits or `splitAt` reaches 1.

### 8. Emit and advance

The chunk (`prefix + content[:splitAt] + suffix`) is appended to results. The input is advanced past `splitAt`. The tag stack becomes the carry for the next iteration.

## Integration

The Telegram adapter calls `Split` in `dispatchOut` after converting markdown to HTML:

```go
formatted := format.TelegramHTML(msg.Text)
for _, chunk := range format.Split(formatted, 4096) {
    _, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
        ChatID:    msg.ChatID,
        Text:      chunk,
        ParseMode: models.ParseModeHTML,
    })
    if err != nil {
        // fallback: send chunk as plain text
    }
}
```

- `Split` operates on HTML output, not raw markdown.
- Each chunk has independent error handling with plain-text fallback.
- Chunks are sent sequentially to preserve message order.
- The typing indicator is stopped once before sending all chunks.
- Delivery is best-effort: if chunk N fails, previously sent chunks are not retracted.

## Supported Tags

Tags tracked for repair across chunk boundaries:

| Tag | Description |
|-----|-------------|
| `b` | Bold |
| `i` | Italic |
| `u` | Underline |
| `s` | Strikethrough |
| `code` | Inline code |
| `pre` | Preformatted block |
| `a` | Hyperlink (attributes preserved) |
| `blockquote` | Block quote |

Any other HTML tags in the input are left as-is and not tracked or repaired.

## Edge Cases

| Input | Behavior |
|-------|----------|
| `maxLen <= 0` | Defaults to 4096 |
| Input shorter than `maxLen` | Returns immediately as a single-element slice, no allocation |
| Empty string | Returns `[""]` |
| Chunk is blank after split | Filtered out by `appendNonBlank`; never appears in output |
| Result slice is empty after filtering | Returns `[]string{html}` as fallback (at this point `html` is the remaining unconsumed input, typically empty) |
| Budget squeezed to zero by long prefix | Clamped to 1 byte minimum |
| `splitAt` resolves to 0 | Clamped to 1 to prevent infinite loops |
| Tag with attributes (e.g., `<a href="...">`) | Full opening tag string stored in `htmlTag.open` and replayed verbatim |
| Tag names with mixed case | Lowercased by `extractName` for matching |
| Non-Telegram tags (e.g., `<div>`) | Ignored by tag tracker, pass through without repair |
| Split lands inside `<a href="...">` | `avoidTagSplit` scans back up to 500 bytes to move before `<` |
| Split lands inside `&amp;` | `avoidEntitySplit` scans back up to 12 bytes to move before `&` |

## Examples

### Short message (no split)

```
Input:  "Hello <b>world</b>"  (maxLen=4096)
Output: ["Hello <b>world</b>"]
```

### Paragraph boundary split

```
Input:  "AAAA\n\nBBBB"  (maxLen=8)
Output: ["AAAA\n\n", "BBBB"]
```

The `\n\n` delimiter stays with the first chunk.

### Tag repair across boundary

```
Input:  "<b>AAAA BBBB</b>"  (maxLen=12)
Output: ["<b>AAAA </b>", "<b>BBBB</b>"]
```

The `<b>` tag is closed at the end of chunk 1 and reopened at the start of chunk 2.

### Nested tag repair

```
Input:  "<pre><code>line1\nline2\nline3</code></pre>"  (maxLen=30)
Output: ["<pre><code>line1\n</code></pre>", "<pre><code>line2\nline3</code></pre>"]
```

Both `<pre>` and `<code>` are closed and reopened, maintaining correct nesting order.

### Hard cut (no whitespace)

```
Input:  "AAAAABBBBB"  (maxLen=5)
Output: ["AAAAA", "BBBBB"]
```

With no whitespace or tag boundaries, the split falls back to exact byte position.

## Related Documentation

For a user-facing explanation of this feature, see [Response Handling](response-handling.md).
