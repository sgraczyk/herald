# Herald

Lightweight, self-hosted AI assistant bot for Telegram. Single Go binary, bbolt storage, minimal dependencies. Deployed as an LXC container on Proxmox.

Part of the [sgraczyk/homelab](https://github.com/sgraczyk/homelab) project. Tracking issue: [homelab#30](https://github.com/sgraczyk/homelab/issues/30).

## Workflow

All work **must** follow this process:

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

## Architecture

### Message flow

```
Telegram ──write──> Hub.In ──read──> Agent Loop ──call──> Provider (claude -p / Chutes.ai)
                                         │
                                         ├──read/write──> Store (bbolt)
                                         │
                                         └──write──> Hub.Out ──read──> Format (md→HTML) ──send──> Telegram
```

### Provider fallback chain

```
1. claude -p (free, uses existing Claude subscription)
   └─ fail? ──> 2. Chutes.ai (OpenAI-compatible)
                  └─ fail? ──> 3. Return error to user
```

## Tech Stack

| Package | Purpose |
|---------|---------|
| `github.com/go-telegram/bot` | Telegram Bot API |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/yuin/goldmark` | Markdown parser |
| `go.etcd.io/bbolt` | Embedded key/value store |

No CGO. Single static binary.

## Repo Structure

```
cmd/herald/main.go           # Entry point + CLI (cobra)
internal/
  agent/
    loop.go                  # Agent loop: read hub, call LLM, write response
    context.go               # System prompt assembly (personality + memory + history)
  config/
    config.go                # Config file loading and validation
  format/
    telegram.go              # Markdown → Telegram HTML converter (goldmark-based)
    split.go                 # Message splitting for Telegram length limits
  health/
    handler.go               # HTTP health check endpoint
  hub/
    hub.go                   # Message hub: fan-in/fan-out via Go channels
  metrics/
    metrics.go               # In-memory counters and JSON endpoint
  provider/
    provider.go              # LLMProvider interface + Message type
    claude.go                # claude -p backend (exec, parse JSON output)
    openai.go                # OpenAI-compatible HTTP client (Chutes.ai, Groq, etc.)
    fallback.go              # Try providers in order, return first success
    image.go                 # Image/photo handling for LLM providers
    validate.go              # Provider configuration validation
  store/
    db.go                    # bbolt init (go.etcd.io/bbolt, pure Go)
    history.go               # Conversation history per chat (bucket per chat_id)
    memory.go                # Long-term memory per chat (facts, preferences)
  telegram/
    adapter.go               # go-telegram/bot long-polling, user whitelist
docs/
  ci.md                      # CI workflow details
  configuration.md           # Config file reference, providers, system prompt
  deployment.md              # User whitelist, systemd, credential management
  features.md                # Memory, images, responses, personality, commands
  formatting.md              # Markdown to Telegram HTML formatting
  godoc.md                   # Doc comment conventions and browsing godoc
  logging.md                 # Structured logging reference (slog levels, fields)
```

## Conventions

**Language:** English (all code, commits, docs, comments).

### Design Principles

- **KISS** — solve the problem at hand, nothing more
- **Minimalism** — export the minimum API surface
- **SRP** — one package = one concern, one function = one job
- **DRY** — extract shared logic, but prefer copying over cross-package abstractions
- **YAGNI** — don't build for hypothetical requirements; single user, single node

### Commits

[Conventional Commits](https://www.conventionalcommits.org/), imperative mood, with optional scope: `feat(telegram): add long polling`, `fix(provider): handle timeout`.

### Go Code

Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) strictly. Project-specific notes:

- `internal/` packages for all non-main code
- No global state — inject dependencies via constructors
- No panics for recoverable errors
- No CGO — all dependencies must be pure Go

### Go Comments

Follow [Go Doc Comments](https://go.dev/doc/comment). Every exported symbol must have a doc comment. Don't add comments to code you didn't change.

### Config

- Runtime config via `config.json` (see `config.json.example`)
- Secrets via environment variables from `.env` (see `.env.example`)

## Development

```bash
CGO_ENABLED=0 go build -o herald ./cmd/herald     # Build
go test ./...                                       # Test
./herald                                            # Run (Telegram bot)
./herald ask "question"                             # Run (CLI mode)
```

## CI/CD

Two GitHub Actions workflows in `.github/workflows/`:

- **CI** (`ci.yml`): lint (`go vet` + `staticcheck`), vulncheck (`govulncheck`), build, test (race detector + coverage)
- **Release Please** (`release-please.yml`): automated version bumps, changelogs, GitHub Releases with binary artifacts

Versioning: [semver](https://semver.org/) via release-please. Config in `release-please-config.json`.
