# Formatting

How Herald converts Markdown from AI providers into Telegram messages.

## How It Works

Herald receives Markdown responses from AI providers (Claude CLI, OpenAI-compatible APIs) and converts them to Telegram's supported HTML format before sending. This happens automatically -- no configuration needed.

Telegram only supports a small set of HTML tags. Herald's converter maps standard Markdown to these tags, and converts unsupported elements (tables, images, headings) into readable alternatives.

## Supported Formatting

| Markdown | Telegram Result |
|----------|----------------|
| `**bold**` | **bold** |
| `*italic*` | *italic* |
| `` `code` `` | `code` |
| `~~strikethrough~~` | ~~strikethrough~~ |
| `[link](url)` | clickable link |
| `` ```code block``` `` | formatted code block |
| `> quote` | blockquote |
| `- item` | bullet (•) |
| `1. item` | numbered list |

Fenced code blocks with a language tag (e.g., `` ```python ``) include the language class, which Telegram renders with syntax highlighting on supported clients.

## Converted Elements

Some Markdown features don't have Telegram equivalents. Herald converts them to readable alternatives:

- **Headings** (`# Title`) → bold text
- **Images** (`![alt](url)`) → clickable links
- **Horizontal rules** (`---`) → em dashes (———)
- **2-column tables** → bullet-point key-value lists
- **Wide tables** (3+ columns) → pre-formatted text blocks
- **Raw HTML** → HTML-escaped plain text

## Custom System Prompts

The built-in system prompt already instructs providers to use Telegram-friendly formatting. If you use a [custom system prompt](features.md#custom-personality), include formatting guidance:

```json
{
  "system_prompt": "... Format for Telegram: use bold, italic, and code. Avoid markdown tables and headings when possible."
}
```

## Internals

The converter lives in `internal/format/telegram.go` and uses [goldmark](https://github.com/yuin/goldmark) with GFM (GitHub Flavored Markdown) extensions. A custom `telegramRenderer` maps each AST node type to Telegram-compatible HTML.

### Singleton goldmark instance

The goldmark parser/renderer is initialized once as a package-level variable and shared across all calls. This is safe because:

1. The goldmark `Markdown` type is stateless and concurrent-safe
2. The custom renderer (`telegramRenderer`) is an empty struct with no mutable fields
3. All per-call state (source bytes, output buffer) is stack-local

### Performance

Benchmark baseline (mixed Markdown input with heading, bold, italic, list, blockquote, table, and link):

```
BenchmarkTelegramHTML-12    129770    7708 ns/op    16612 B/op    138 allocs/op
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./internal/format/...
```

### Extending

To support additional Markdown node types:

1. Add a render method to `telegramRenderer`
2. Register it in `RegisterFuncs`
3. If the node comes from a goldmark extension, add the extension to the `md` initializer

See `internal/format/telegram.go` for the full renderer implementation.

## Troubleshooting

### Response looks like raw Markdown

The AI provider sent Markdown that Herald couldn't parse. On parse failure, Herald falls back to HTML-escaped plain text. Check that your custom system prompt doesn't instruct the AI to use non-standard Markdown.

### Tables look wrong

Telegram doesn't support HTML tables. Herald converts 2-column tables to bullet lists and wider tables to pre-formatted text. For cleaner output, ask the AI to format data as lists instead.
