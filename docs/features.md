# Features

Herald's capabilities beyond basic chat: memory, image understanding, response handling, custom personality, and commands.

## Automatic Memory Extraction

Herald learns about you as you chat. After each exchange, it reviews the conversation for durable facts and saves them automatically in the background. You get your response immediately -- extraction happens concurrently.

### What Gets Remembered

- **Preferences** -- favorite tools, languages, foods, workflows
- **Personal details** -- name, job, location, timezone
- **Habits and routines** -- code formatting preferences, work schedule
- **Background info** -- projects, skills

Automatic memories are labeled "auto" to distinguish them from explicit ones.

### What Gets Skipped

Very short messages (under 10 characters or single words like "ok", "thanks") are skipped entirely. Herald also deduplicates -- it won't store facts it already knows.

### Memory Commands

| Command | What It Does |
|---------|-------------|
| `/remember <fact>` | Explicitly save a memory (e.g., `/remember I prefer dark mode`) |
| `/forget <fact>` | Remove a stored memory by keyword |
| `/memories` | List everything Herald remembers about you |

Explicit memories (from `/remember`) are always prioritized in conversations.

### Implementation Notes

Extraction runs in a background goroutine with a 30-second timeout. On shutdown, in-flight extractions get up to 10 seconds to complete. The extraction provider prefers OpenAI-compatible over Claude CLI to avoid spawning concurrent Node.js processes on the constrained deployment target.

## Image Understanding

Send a photo to Herald in Telegram. No commands needed -- Herald automatically detects images and responds.

**With a caption:** Herald uses your caption as the prompt (e.g., "What does this sign say?").

**Without a caption:** Herald defaults to "What's in this image?" and gives a general description.

### Requirements

A vision-capable OpenAI-compatible provider must be configured (e.g., a model with `VL` suffix). Claude CLI cannot handle images -- Herald automatically routes photos to a compatible provider.

### Limitations

- **Images are not saved.** Only a `[image] <caption>` placeholder is stored in history. Send the photo again for follow-up questions.
- **Large images are resized.** Images over 2000px are scaled down automatically.
- **WEBP passthrough.** WEBP images are not resized (no Go stdlib decoder without CGO) but are still subject to the 4 MB size cap.
- **Single image per message.** Telegram sends one photo per message.

## Response Handling

### Response Size Limit (10 MB)

Herald limits responses from providers to 10 MB. Oversized responses are rejected and the next provider in the fallback chain is tried. A typical response is a few kilobytes -- the threshold exists as a safety net against abnormal upstream behavior.

### Message Splitting

Telegram limits individual messages to 4096 bytes. When a response exceeds this, Herald automatically splits it into multiple messages.

**Split strategy** (in order of preference):

1. **Paragraph breaks** -- preferred split point
2. **Line breaks** -- used if no paragraph break fits
3. **Word boundaries** -- last resort before hard cut

Bold, italic, code blocks, links, and other formatting carry over between split messages. If a formatted span crosses a split point, Herald closes the formatting at the end of one message and reopens it at the start of the next. See [Formatting](formatting.md) for the full list of supported Markdown elements and how they map to Telegram HTML.

Each chunk has independent error handling -- if HTML rendering fails, that chunk is retried as plain text.

## Custom Personality

You can change how Herald talks by setting `system_prompt` in `config.json`. No rebuilding needed -- edit and restart.

```json
{
  "system_prompt": "You are a friendly cooking assistant. Help with recipes and meal planning. Be warm and concise. Format for Telegram: no markdown tables, no headings, use bold/italic."
}
```

If you omit `system_prompt`, Herald uses its built-in personality: concise, direct, responds in your language, follows Telegram formatting rules.

### Tips for Writing Prompts

- **Keep it short.** The prompt is sent with every message. A few sentences is ideal.
- **Include Telegram formatting rules.** The built-in prompt tells Herald to avoid markdown tables and headings (Telegram doesn't render them). Add similar instructions to your custom prompt.
- **Think about mobile.** Most Telegram users read on their phones. Encourage brief, scannable responses.
- **Be specific.** "Answer cooking questions with ingredient lists and step-by-step instructions" is better than "be helpful."

### Interaction with Memories

Memories are appended to whatever system prompt is active. You don't need to account for memories in your prompt text.

### Prompt Length Warning

Herald logs a warning at startup if the prompt exceeds 4000 characters. It still works, but reduces space for conversation history in each request.

## Commands

### Recognized Commands

| Command | What It Does |
|---------|-------------|
| `/clear` | Reset conversation history for the current chat |
| `/model` | Switch between LLM providers |
| `/status` | Show uptime, active provider, and message count |
| `/remember` | Save a fact to long-term memory |
| `/forget` | Remove a specific memory |
| `/memories` | List all stored memories |

### Unrecognized Commands

Any slash command not listed above is treated as a regular message. The full text, including the slash and command word, is sent to the AI:

- `/summarize this document` -- the AI receives the entire message
- `/translate hello world to French` -- the AI sees the full request
- `/foo bar baz` -- the AI receives `/foo bar baz` and responds
