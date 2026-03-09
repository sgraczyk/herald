# Herald

Lightweight, self-hosted AI assistant bot for Telegram.

[![CI](https://github.com/sgraczyk/herald/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/sgraczyk/herald/actions/workflows/ci.yml)
![coverage](https://img.shields.io/endpoint?url=https://gist.githubusercontent.com/sgraczyk/4935f95f21a0c7a553bb08194b36b2d3/raw/coverage.json)
[![Release](https://img.shields.io/github/v/release/sgraczyk/herald)](https://github.com/sgraczyk/herald/releases/latest)

## Features

- **Provider fallback** — Claude CLI as primary, any OpenAI-compatible API as backup
- **Conversation history** — per-chat message history stored in bbolt (embedded, pure Go)
- **Long-term memory** — remembers facts and preferences across conversations
- **User whitelist** — only responds to authorized Telegram user IDs
- **CLI mode** — test locally with `./herald ask "question"` without Telegram
- **Telegram commands** — `/clear` resets context, `/model` switches providers, `/status` shows bot info
- **Image support** — understands photos sent in Telegram chats
- **Single binary** — no CGO, no Docker, no external dependencies at runtime

## Quick Start

Build:

```bash
CGO_ENABLED=0 go build -o herald ./cmd/herald
```

Configure:

- Copy [`config.json.example`](config.json.example) to `config.json` and edit to taste
- Copy [`.env.example`](.env.example) to `.env` and fill in your secrets

Run:

```bash
./herald
```

## Configuration

Herald uses `config.json` for structure and environment variables (via `.env`) for secrets. The config file references env var names; the binary reads them at startup.

See [docs/configuration.md](docs/configuration.md) for the full reference.

## Deployment

Designed for single-node deployment as a systemd service. See [docs/deployment.md](docs/deployment.md) for setup instructions.

## Documentation

| Document | Contents |
|----------|----------|
| [docs/configuration.md](docs/configuration.md) | Config file reference, providers, system prompt |
| [docs/deployment.md](docs/deployment.md) | User whitelist, systemd, credential management |
| [docs/features.md](docs/features.md) | Memory, images, responses, personality, commands |
| [docs/logging.md](docs/logging.md) | Structured logging reference (slog levels, fields) |
| [AGENTS.md](AGENTS.md) | Architecture, conventions, development guide |
