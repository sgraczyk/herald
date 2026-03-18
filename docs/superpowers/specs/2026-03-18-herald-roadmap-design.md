# Herald Roadmap: v0.7 - v0.9

Design spec for Herald's next three milestones covering UX improvements, AI capabilities, code quality, and lightweight operations.

## Context

Herald is at v0.6.0 with streaming, summarization, and conversation archival. The codebase is ~8,300 lines of Go with solid test coverage and clean architecture. This spec defines the path to v1.0.

### User priorities (in order)

1. **UX** — document handling (PDFs), better formatting, message reactions, voice messages
2. **AI capabilities** — tool use (Home Assistant, calendar), RAG, multi-modal output (MCP deferred to post-v0.9)
3. **Code quality** — fix known issues, improve tests
4. **Operations** — lightweight monitoring (Uptime Kuma compatible, no Prometheus)

### Design principles

- KISS, YAGNI — build foundations only when they serve 2+ features
- Pure Go, no CGO — all dependencies must be pure Go
- Single user, single node — no multi-tenancy complexity
- Lightweight — homelab runs Uptime Kuma + Homepage, no heavy monitoring stacks

---

## v0.7.0 — Documents & Formatting

Milestone theme: make Herald more useful for everyday interactions.

### 1. PDF Document Handling

**Goal:** User sends a PDF to Herald via Telegram, Herald extracts text and discusses it.

**Architecture:**

```
Telegram (document message)
  │
  ├── adapter.go: detect document, download file
  │
  ├── document pipeline (new package: internal/document/)
  │     ├── extract.go: Extractor interface + PDF implementation
  │     └── types.go: Document struct {Name, Text, Pages, SizeBytes}
  │
  ├── hub.InMessage gains Documents []Document field
  │
  └── agent/context.go: inject document text into message context
        └── format: "--- Document: {name} ({pages} pages) ---\n{text}\n---"
```

**PDF extraction:** Use a pure-Go PDF text extraction library. Candidates:
- `github.com/ledongthuc/pdf` — simple, read-only, pure Go
- `github.com/dslipak/pdf` — similar, lightweight

No OCR for scanned PDFs in v0.7 — text-based PDFs only. Scanned PDF support can come later via an OCR tool.

**Telegram integration:**
- Handle `Message.Document` with MIME type `application/pdf`
- Download via Bot API `GetFile` + HTTP fetch
- Size limit: 10 MB (Telegram's file limit is 20 MB, but we're conservative for memory)
- Inject extracted text into the conversation as a system-adjacent message
- Store document text in history so follow-up questions work

**Token budget consideration:**
- PDF text can be very long — truncate to fit within provider context window
- Add a `max_document_tokens` config field (default: 4000 estimated tokens, ~16KB text)
- Token count is estimated at ~4 characters per token, consistent with the existing `history_token_budget` estimation
- If truncated, append "[Document truncated — showing first N pages]"

**Error handling:**
- Encrypted PDFs → "Sorry, I can't read encrypted PDFs"
- Empty/no-text PDFs → "This PDF appears to be scanned/image-based. Text extraction isn't supported yet."
- Garbled output (non-standard font encodings) → detect via minimum text density check (ratio of printable ASCII to total characters); if below threshold, treat as unsupported format
- Download failure → "Couldn't download the file, please try again"

### 2. Formatting Improvements

**Goal:** Better Telegram HTML output for common LLM response patterns.

**Changes to `internal/format/telegram.go`:**

| Issue | Fix |
|-------|-----|
| Code blocks lose language info | Render as `<pre><code class="language-X">` — Telegram ignores the class but it's valid HTML and future-proof |
| Nested lists render flat | Track list nesting depth, indent with spaces in output |
| Long code blocks unreadable | No fix needed — Telegram renders `<pre>` with horizontal scroll |

**Changes to `internal/format/split.go`:**
- Avoid splitting inside code blocks when possible (prefer splitting at block boundaries)

### 3. Message Reactions

**Goal:** Visual feedback that Herald received the message and is working on it.

**Implementation:**
- On message receipt (before LLM call): set reaction ⏳ on the user's message
- On successful response: replace with ✅
- On error: replace with ❌
- Use Telegram Bot API `setMessageReaction` method

**Telegram adapter changes (KISS approach — reactions stay in the adapter, no hub changes):**
- Add `TelegramMessageID int64` field to `hub.InMessage` so the agent can reference the original message
- Adapter sets ⏳ reaction immediately on receipt, before writing to `hub.In`
- Adapter sets ✅ or ❌ in `dispatchOut`/`dispatchStream` when the response arrives
- Add `SetReaction(chatID, messageID int64, emoji string) error` helper method on the adapter
- All reaction logic stays in the adapter — no new hub channels needed

**Fallback:** If bot lacks reaction permissions (older groups, etc.), silently skip. Reactions are cosmetic, never error on failure.

---

## v0.8.0 — Tool Use & AI

Milestone theme: make Herald capable of taking actions and accessing knowledge.

### 4. Tool Framework

**Goal:** Extensible tool system that lets Herald call external services and feed results back to the LLM.

**Architecture:**

```go
// internal/tool/tool.go
package tool

// Tool defines an action Herald can perform.
type Tool interface {
    Name() string
    Description() string        // Shown to LLM for tool selection
    Parameters() []Parameter    // JSON Schema-like parameter definitions
    Execute(ctx context.Context, args map[string]any) (string, error)
}

type Parameter struct {
    Name        string
    Type        string // "string", "number", "boolean"
    Description string
    Required    bool
}

// Registry holds available tools.
type Registry struct {
    tools map[string]Tool
}
```

**Agent loop integration:**
- Build tool descriptions into the system prompt (or function calling schema if provider supports it)
- LLM responds with tool call request (detect via structured output or convention)
- Agent executes tool, feeds result back as a new message, LLM generates final response
- Max tool calls per turn: 3 (prevent infinite loops)
- Timeout per tool call: 10 seconds

**Provider interface change:**

The current `LLMProvider` interface returns `(string, error)`. To support tools, introduce a `ChatOptions` struct rather than changing the method signature or abusing context values:

```go
// ChatOptions holds optional parameters for a Chat call.
type ChatOptions struct {
    Tools []ToolDefinition // nil = no tools
}

type ToolDefinition struct {
    Name        string
    Description string
    Parameters  []ToolParameter
}

type ToolParameter struct {
    Name        string
    Type        string // "string", "number", "boolean"
    Description string
    Required    bool
}

// ChatResponse wraps the LLM output, which may include tool calls.
type ChatResponse struct {
    Text      string
    ToolCalls []ToolCall // nil = plain text response
}

type ToolCall struct {
    Name string
    Args map[string]any
}
```

The `LLMProvider` interface gains a new method `ChatWithTools(ctx, []Message, ChatOptions) (ChatResponse, error)`. Providers that don't support tools return `ChatResponse{Text: ...}` with nil ToolCalls. The existing `Chat` method stays for backward compatibility; the agent loop calls `ChatWithTools` when tools are registered.

**Tool call detection per provider:**

**OpenAI-compatible provider:** Native function calling. Map `ToolDefinition` to the OpenAI `tools` array in the request body. Parse `tool_calls` from the response `choices[0].message`. Well-defined protocol, no ambiguity.

**Claude CLI provider:** `claude -p` does not support custom tool definitions natively. The `--allowedTools` flag only controls built-in tools (WebSearch, WebFetch, etc.), not user-defined ones. Therefore:
- Inject tool definitions into the system prompt as a structured block (JSON or XML format describing available tools)
- Instruct the LLM to emit tool calls in a parseable format: `<tool_call>{"name": "...", "args": {...}}</tool_call>`
- Parse the response text for `<tool_call>` blocks using regex
- If a tool call is found, execute it, then re-call the LLM with the tool result appended as a message
- This is a prompt-engineering approach but reliable — Claude models follow structured output instructions well

**Fallback provider:** Delegates to whichever underlying provider is active; no additional logic needed.

**Config validation for tools:**

Tool configs are validated at startup, consistent with the existing provider validation pattern in `validate.go`. Missing required env vars (e.g., `HA_TOKEN` set but `HA_BASE_URL` missing) produce clear error messages. Tools with invalid configs are disabled with a warning, not fatal — Herald should still work without tools.

### 5. Home Assistant Tool

**Goal:** Query and control Home Assistant from Telegram.

```go
// internal/tool/homeassistant/ha.go
type HomeAssistant struct {
    baseURL string  // e.g., "http://192.168.0.100:8123"
    token   string  // Long-lived access token
    client  *http.Client
}
```

**Capabilities:**
- `ha_get_state` — get state of an entity (e.g., `sensor.temperature_living_room`)
- `ha_call_service` — call a service (e.g., `light.turn_on` for `light.bedroom`)
- `ha_list_entities` — list entities matching a pattern (for discovery)

**Config:**
```json
{
  "tools": {
    "home_assistant": {
      "base_url_env": "HA_BASE_URL",
      "token_env": "HA_TOKEN"
    }
  }
}
```

**Safety:** All tool calls logged. In v0.8, start with read-only queries only (`ha_get_state`, `ha_list_entities`). Add `ha_call_service` with inline keyboard confirmation in v0.9 once the adapter supports callback queries (non-trivial: requires callback query handler, pending action store, timeouts).

### 6. Calendar & Reminders Tool

**Goal:** Simple reminder system with Telegram notifications.

**Approach:** Start with a local reminder store in bbolt rather than iCloud integration (simpler, no external deps).

```go
// internal/tool/reminder/reminder.go
type Reminder struct {
    ID      string
    ChatID  int64
    Text    string
    DueAt   time.Time
    Created time.Time
}
```

**Capabilities:**
- `reminder_add` — "remind me to X at Y" → parse time, store in bbolt, schedule notification
- `reminder_list` — show upcoming reminders
- `reminder_remove` — cancel a reminder

**Notification:** Background goroutine checks reminders every minute, sends Telegram message when due. Persisted in bbolt so they survive restarts.

**Startup behavior:** On startup, load all reminders from bbolt. Reminders with `DueAt` in the past fire immediately with a note: "Reminder (was scheduled for [time]): [text]". Future reminders are loaded into the in-memory scheduler. The ticker goroutine is owned by the reminder tool and receives a send function (or hub reference) via constructor injection to deliver messages.

**Time parsing:** Use simple regex patterns for common formats ("in 30 minutes", "at 3pm", "tomorrow at 9", "2026-03-20 14:00"). A single-user bot doesn't need a full NLP time parser — YAGNI. If patterns prove insufficient, add `github.com/olebedev/when` later.

### 7. Document RAG (basic)

**Goal:** Index documents sent to Herald and retrieve relevant context for future questions.

**Approach:** Keep it simple — no vector database, no embeddings service. Use keyword-based search over stored document text.

```
internal/store/
  document.go  — store/retrieve document text in bbolt (bucket: "documents")
```

**Chunking strategy:**
- On PDF receipt, split extracted text into chunks at paragraph boundaries (~500 tokens / ~2KB per chunk)
- Store each chunk as a separate bbolt key: `{chat_id}/{doc_name}/{timestamp}/{chunk_index}`
- Store document metadata (name, total chunks, total pages) in a separate key for listing

**How it works:**
- When a PDF is sent, chunk and store as above
- On each message, score stored chunks by simple term frequency (count of query terms appearing in chunk)
- Inject top-N matching chunks into context (reuse `max_document_tokens` budget, typically 2-8 chunks)
- No TF-IDF — simple term frequency is sufficient for a single-user system with a small document corpus (YAGNI)

**Future upgrade path:** Replace keyword search with embedding-based search when a suitable pure-Go or API-based solution is available. The storage and chunking layers stay the same.

**Commands:**
- `/docs` — list stored documents
- `/docs forget <name>` — remove a stored document

---

## v0.9.0 — Quality & Operations

Milestone theme: harden the codebase and improve observability.

### 8. Code Quality Fixes

**Memory extraction JSON parsing** (`agent/loop.go`):
- Replace backtick string stripping with `json.Decoder` that skips leading non-JSON characters
- Validate that decoded value is a string array

**EXIF orientation** (`provider/image.go`):
- Use `github.com/rwcarlsen/goexif/exif` (pure Go) to read orientation tag
- Apply rotation/flip before sending to LLM
- Only affects JPEG (PNG/WebP don't have EXIF orientation)

**Archive cleanup** (`store/conversation.go`):
- Add `max_archived_conversations` config (default: 50)
- On `/new`, if archives exceed limit, delete oldest
- Add `/conversations clear` to manually purge all archives

### 9. Test Improvements

| Area | What to add |
|------|-------------|
| `hub/` | Basic tests for channel routing and drain mode |
| `agent/` | Integration test with mock provider and mock store — full message lifecycle |
| `tool/` | Test framework: tool registration, execution, error handling |
| `document/` | PDF extraction tests with sample PDFs |
| End-to-end | Test with mock Telegram server — send message, verify response |

### 10. Lightweight Monitoring

**Goal:** Work with existing Uptime Kuma setup, no new infrastructure.

**Health endpoint improvements:**
- `/health` already exists — ensure it returns degraded status when provider is unhealthy
- Add provider latency percentiles to `/health` response (p50, p95 from in-memory histogram)
- Uptime Kuma can monitor `/health` endpoint directly

**Alert-to-Telegram:**
- New optional config: `alert_chat_id` — Herald sends operational alerts to this Telegram chat
- Triggers: provider failover, all providers down, auth failure, high error rate
- Debounce: max 1 alert per event type per 15 minutes
- Implementation: simple alert channel in hub, telegram adapter reads and sends

**Metrics improvements:**
- Add per-tool call counts and latency to metrics
- Add document processing counts
- `/metrics` endpoint already exists — extend the snapshot

### 11. Voice Messages (provider TBD)

**Architecture (provider-agnostic):**

```go
// internal/transcribe/transcribe.go — separate package (audio is not a "document")
package transcribe

type Transcriber interface {
    Transcribe(ctx context.Context, audio []byte, format string) (string, error)
}
```

**Telegram integration:**
- Handle `Message.Voice` and `Message.Audio`
- Download OGG file via Bot API
- Pass to transcriber, inject transcript as message text
- Treat the rest of the flow as a normal text message

**Provider decision deferred.** The interface is ready; implementation plugs in later (OpenAI Whisper API, local Whisper, or other).

### 12. Multi-modal Output

**Goal:** Herald can generate images via the tool framework.

**Implementation:** Add an image generation tool that calls an external API (DALL-E, Stable Diffusion API, etc.).

```go
// internal/tool/imagegen/imagegen.go
type ImageGenerator struct {
    apiKey  string
    baseURL string
    client  *http.Client
}
```

- Tool returns image URL or base64 data
- Agent loop detects image response, sends via Telegram `sendPhoto` instead of `sendMessage`
- Extend `hub.OutMessage` with optional `Photo []byte` and `PhotoCaption string` fields (nil = text-only, non-nil = send as photo). Avoids a separate message type.

---

## Dependency Summary

| Dependency | Purpose | Version |
|------------|---------|---------|
| `github.com/ledongthuc/pdf` (or similar) | PDF text extraction | TBD |
| `github.com/rwcarlsen/goexif/exif` | EXIF orientation reading | TBD |

All must be pure Go, no CGO.

## Config Changes

```json
{
  "max_document_tokens": 4000,
  "max_archived_conversations": 50,
  "alert_chat_id": 0,
  "tools": {
    "home_assistant": {
      "base_url_env": "HA_BASE_URL",
      "token_env": "HA_TOKEN"
    }
  }
}
```

## Migration

No breaking changes. All new config fields have sensible defaults. Existing `config.json` files work without modification.

## Out of Scope

- Multi-user support
- Webhook mode (long-polling is fine for single user)
- MCP server integration (future, after tool framework proves out)
- Self-hosted Whisper (needs GPU)
- OCR for scanned PDFs (future tool)
