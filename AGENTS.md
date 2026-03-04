# Herald

Lightweight, self-hosted AI assistant bot for Telegram. Single Go binary, bbolt storage, minimal dependencies. Deployed as an LXC container on Proxmox.

Tracking issue: [sgraczyk/homelab#30](https://github.com/sgraczyk/homelab/issues/30)

## Architecture

### Message flow

```
Telegram ──write──> Hub.In ──read──> Agent Loop ──call──> Provider (claude -p / Chutes.ai)
                                         │
                                         ├──read/write──> Store (bbolt)
                                         │
                                         └──write──> Hub.Out ──read──> Telegram
```

The **hub** is the central message router using Go channels. The Telegram adapter writes incoming messages to `Hub.In`, the agent loop reads them, calls the LLM provider, and writes responses to `Hub.Out`, which the Telegram adapter reads and sends back.

### Provider fallback chain

```
1. claude -p (free, uses existing Claude subscription)
   └─ fail? ──> 2. Chutes.ai (OpenAI-compatible, $3/mo)
                  └─ fail? ──> 3. Return error to user
```

The `claude -p` provider executes the Claude CLI in pipe mode with `--output-format json`. The OpenAI-compatible provider is a standard HTTP client that works with any OpenAI-compatible API (Chutes.ai, Groq, Gemini, etc.).

## Tech Stack

| Package | Purpose | Notes |
|---------|---------|-------|
| `github.com/go-telegram/bot` | Telegram Bot API | 0 transitive deps, Bot API 9.4 |
| `github.com/spf13/cobra` | CLI framework | ~3 transitive deps |
| `go.etcd.io/bbolt` | Embedded key/value store | 0 transitive deps, pure Go |

No CGO. Single static binary. Cross-compiles trivially with `GOOS=linux GOARCH=amd64`.

## Repo Structure

```
cmd/herald/main.go           # Entry point + CLI (cobra)
internal/
  hub/
    hub.go                   # Message hub: fan-in/fan-out via Go channels
  agent/
    loop.go                  # Agent loop: read hub, call LLM, write response
    context.go               # System prompt assembly (personality + memory + history)
  telegram/
    adapter.go               # go-telegram/bot long-polling, user whitelist
  provider/
    provider.go              # LLMProvider interface
    claude.go                # claude -p backend (exec, parse JSON output)
    openai.go                # OpenAI-compatible HTTP client (Chutes.ai, Groq, etc.)
    fallback.go              # Try providers in order, return first success
  store/
    db.go                    # bbolt init (go.etcd.io/bbolt, pure Go)
    history.go               # Conversation history per chat (bucket per chat_id)
config.json.example
.env.example
go.mod
```

## Key Interfaces

### LLMProvider

```go
type LLMProvider interface {
    Name() string
    Chat(ctx context.Context, messages []Message) (string, error)
}
```

### Message

```go
type Message struct {
    Role      string    // "user", "assistant", "system"
    Content   string
    Timestamp time.Time
}
```

### bbolt Storage Design

```
herald.db (single file)
├── messages/              # Top-level bucket
│   ├── <chat_id>/         # Nested bucket per chat
│   │   ├── 00000001 → {"role":"user","content":"...","timestamp":"..."}
│   │   ├── 00000002 → {"role":"assistant","content":"...","timestamp":"..."}
│   │   └── ...            # Sequential uint64 keys (big-endian, naturally sorted)
│   └── <chat_id>/         # ... more chats
└── metadata/              # Stretch: long-term memory, cron state
```

- Keys: big-endian uint64 (auto-increment per bucket) — bbolt's `NextSequence()`
- Values: JSON-encoded `{role, content, timestamp}`
- Prune: after insert, iterate from start, delete oldest if count > 50
- Clear: delete and recreate the chat bucket

## Deployment

| Parameter | Value |
|-----------|-------|
| Container | LXC on Proxmox (Debian minimal) |
| CT ID | 107 |
| IP | 192.168.0.107 |
| DNS | `ai.internal` (via Caddy) |
| Resources | 1 CPU, 512 MB RAM, 4 GB disk |
| Runtime deps | Claude Code CLI (Node.js), herald binary |
| Service | systemd unit, auto-restart |
| Credentials | `/etc/herald/.env` (TELEGRAM_TOKEN, CHUTES_API_KEY, ALLOWED_USER_IDS) |

RAM is 512 MB (not 256 MB) because Claude Code CLI requires Node.js runtime. The Go binary itself uses ~10 MB.

## Conventions

**Language:** English (all code, commits, docs, comments).

**Commits:** Descriptive, imperative mood. Prefix with scope when useful: `telegram: add long polling`, `provider: implement fallback chain`, `store: add history pruning`.

**Go conventions:**
- `internal/` packages for all non-main code
- File naming: `snake_case.go`
- Error wrapping: `fmt.Errorf("operation description: %w", err)`
- Context propagation: pass `context.Context` as first parameter
- No global state — inject dependencies via constructors

**Config:**
- Runtime config via `config.json` (see `config.json.example`)
- Secrets via environment variables from `.env` (see `.env.example`)
- Config references env vars by name (e.g., `"token_env": "TELEGRAM_TOKEN"`), code reads them at startup

## Development

**Build:**

```bash
CGO_ENABLED=0 go build -o herald ./cmd/herald
```

**Test:**

```bash
go test ./...
```

**Cross-compile for deployment target:**

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o herald ./cmd/herald
```

**Run locally:**

```bash
./herald          # Start Telegram bot (default)
./herald ask "question"   # CLI mode for testing
```

## MVP Scope (v0.1.0)

1. Telegram bot responding to messages via `claude -p`
2. Fallback to Chutes.ai when Claude CLI fails or is slow
3. Conversation history in bbolt (50 messages/chat, structured)
4. `/clear` command to reset context
5. `/model` command to switch between providers
6. `/status` command showing uptime, provider, message count
7. User ID whitelist (only respond to authorized Telegram users)
8. CLI mode for local testing (`./herald ask "question"`)
9. Systemd service with auto-restart

## Rules

- **No secrets in repo.** Use environment variables via `.env` files (gitignored).
- **No CGO.** All dependencies must be pure Go. Use `CGO_ENABLED=0` for builds.
- **Single binary.** No sidecar processes, no Docker, no orchestration.
- **Config via files + env.** `config.json` for structure, `.env` for secrets.
- **Don't over-engineer.** Single user, single node. Keep it simple.
