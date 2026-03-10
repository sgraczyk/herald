# Godoc

How Herald's source code is documented using Go doc comments, and how to browse them.

## Conventions

Every exported symbol has a doc comment following the [Go Doc Comments](https://go.dev/doc/comment) specification. This is enforced by the project convention in AGENTS.md and checked during code review.

### Comment patterns

| Symbol kind | Pattern | Example |
|-------------|---------|---------|
| Package | `// Package <name> <verb phrase>.` | `// Package hub provides the message routing channels...` |
| Type | `// <TypeName> <noun or verb phrase>.` | `// Hub routes messages between...` |
| Function | `// <FuncName> <verb phrase>.` | `// New creates a new Hub with buffered channels.` |
| Method | `// <MethodName> <verb phrase>.` | `// Chat sends a conversation to the LLM...` |
| Variable | `// <VarName> <verb phrase>.` | `// DefaultConfig holds the embedded default configuration...` |
| Sentinel error | `// <ErrName> indicates that <condition>.` | `// ErrTimeout indicates that a provider call exceeded its deadline.` |

All comments are complete sentences ending with a period. Comments start with the symbol name and avoid first-person pronouns.

### Placement rules

- **Package comments** go on the file that best represents the package's purpose (e.g., `provider.go` for the `provider` package). Only one file per package has the package comment.
- **Interface methods** are documented at the interface definition. Concrete implementations provide implementation-specific details in their own doc comments.

## Browsing documentation

### Command line

Run from the repository root:

```bash
# Package summary
go doc ./internal/provider

# Specific type or interface
go doc ./internal/provider LLMProvider

# Specific method
go doc ./internal/provider Fallback.Chat

# All exported symbols in a package
go doc -all ./internal/hub
```

Pipe through a pager for long output:

```bash
go doc -all ./internal/provider | less
```

### All packages

```bash
go doc ./internal/agent       # Agent loop and LLM interaction
go doc ./internal/config      # Configuration loading and validation
go doc ./internal/format      # Markdown to Telegram HTML conversion
go doc ./internal/health      # HTTP health check endpoint
go doc ./internal/hub         # Message routing channels
go doc ./internal/provider    # LLM backends and fallback chain
go doc ./internal/store       # Persistent storage (bbolt)
go doc ./internal/telegram    # Telegram Bot API adapter
```

### Web browser

Start the local documentation server with [pkgsite](https://pkg.go.dev/golang.org/x/pkgsite):

```bash
go install golang.org/x/pkgsite/cmd/pkgsite@latest
pkgsite -open .
```

This serves the full documentation at `http://localhost:8080` with search, cross-references, and source links.

## Writing doc comments

When adding new exported symbols, follow the patterns in the table above and these guidelines:

1. Start the comment with the symbol name, followed by a verb phrase.
2. End every sentence with a period.
3. For sentinel errors, use the phrase "indicates that" to describe the condition.
4. For embedded variables, describe the variable's purpose, not the embed directive.
5. Update the doc comment in the same commit as any behavior change to the associated code.
6. Do not add or modify comments on code you did not change.

Verify coverage before submitting:

```bash
# Check all symbols render correctly
go doc -all ./internal/<package>

# Run staticcheck to catch missing package comments
staticcheck ./...
```

### Adding a new package

1. Place the `// Package <name> ...` comment on the primary file.
2. Use a verb phrase that describes the package's single responsibility.
3. Document every exported symbol before submitting.

### Adding a new LLM provider

1. Add a doc comment on the struct describing the backend.
2. Document `Name()` with the actual return value.
3. Document `Chat()` with the transport mechanism.
4. Document any additional exported methods (e.g., `AuthStatus`).

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `go doc` shows "no symbol found" | Run from the project root. The `./internal/...` paths are relative to the module root. |
| `pkgsite` does not start | Requires Go 1.22 or later. If the port is in use, try `pkgsite -http=:8081 -open .` |
| Doc comments appear outdated | Run `git pull`. Doc comments are part of the source -- `go doc` reads them directly. |

## Related docs

- [configuration.md](configuration.md) -- config file reference and provider settings
- [features.md](features.md) -- user-facing features (memory, images, commands)
- [formatting.md](formatting.md) -- Markdown to Telegram HTML conversion
- [deployment.md](deployment.md) -- setup, whitelist, credentials
