# Herald

Lightweight, self-hosted AI assistant bot for Telegram. Single Go binary, bbolt storage, minimal dependencies. Deployed as an LXC container on Proxmox.

Part of the [sgraczyk/homelab](https://github.com/sgraczyk/homelab) project. Tracking issue: [homelab#30](https://github.com/sgraczyk/homelab/issues/30).

## Workflow

All work **must** follow this process. No exceptions. AI agents must enforce this on every task.

```
1. Problem Statement     — Understand the problem, gather context
2. Create GitHub Issue   — `gh issue create` with clear acceptance criteria
3. Plan Work             — Research codebase, design approach, get user approval
4. Implement Plan        — Write code on a feature branch
5. Test Implementation   — `go test ./...`, `go vet ./...`, manual verification
6. Create PR             — `gh pr create`, link the issue
7. Review PR             — Check diff, verify acceptance criteria are met
8. Apply Changes         — Address review feedback if any
9. Retest                — Run tests again after changes
10. Merge with Squash   — `gh pr merge --squash`, confirm issue closes
11. Back to Main        — `git checkout main && git pull`, pick up next task
```

**Between tasks:** always return to `main`, pull latest, and check `gh issue list` for the next item.

## Task Tracking

All tasks and open work items are tracked as GitHub Issues in this repo. When asked about tasks, remaining work, or what to do next, check `gh issue list`.

## AGENTS.md Maintenance

This file is the source of truth for project conventions. AI agents should periodically validate that it matches reality:
- Verify repo structure section matches actual files (`find internal/ -name '*.go'`)
- Verify CI/CD section matches workflow YAML files
- Verify deployment section matches current infrastructure
- Flag any drift as a new GitHub Issue

When conventions change (new patterns, new tools, new workflows), update this file as part of the same PR.

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

## CI/CD

Two GitHub Actions workflows in `.github/workflows/`:

**CI** (`ci.yml`) — runs on push/PR to `main`:
- **lint:** `go vet ./...` + `staticcheck` (via `dominikh/staticcheck-action`)
- **build:** `go build ./cmd/herald` (verifies compilation with `CGO_ENABLED=0`)
- **test:** `go test -race ./...` (race detector enabled via `CGO_ENABLED=1`)

**Release** (`release.yml`) — runs on tag push (`v*`):
- Builds `linux/amd64` static binary with `-trimpath -ldflags="-s -w"`
- Injects version from git tag via `-X main.version`
- Creates GitHub Release with the binary attached (auto-generated release notes)

## Release Process

1. Ensure `main` is green (CI passing)
2. Tag: `git tag v0.x.x && git push origin v0.x.x`
3. Release workflow builds the binary and creates a GitHub Release automatically
4. Deploy manually: download binary from the release, copy to LXC, restart service

```bash
# Example deploy
scp herald-linux-amd64 root@192.168.0.107:/usr/local/bin/herald
ssh root@192.168.0.107 systemctl restart herald
```

Versioning: [semver](https://semver.org/). Tags must match `v*` pattern.

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
