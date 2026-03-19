# Chutes.ai Image Generation — Design Spec

**Date:** 2026-03-20
**Status:** Approved
**Scope:** Replace DALL-E 3 image provider with Chutes.ai FLUX.1-schnell

## Problem

PR #161 added image generation via OpenAI DALL-E 3, but the user only has Claude CLI and Chutes.ai — no OpenAI API key. The DALL-E implementation is dead code. Chutes.ai hosts FLUX.1-schnell for image generation with a different API format.

## Approach

Minimal swap: replace the `DallE` struct with a `Chutes` struct implementing the same `ImageProvider` interface. The agent loop, hub, and telegram adapter are untouched — the interface boundary isolates the change.

## Changes

### `internal/provider/imagegen.go`

Delete all DALL-E types (`DallE`, `dalleRequest`, `dalleResponse`, `dalleImage`, `NewDallE`).

Add `Chutes` struct implementing `ImageProvider`:

```go
type Chutes struct {
    baseURL string
    apiKey  string
    client  *http.Client
}

func NewChutes(baseURL, apiKey string) *Chutes
```

- `baseURL`: full chute endpoint (e.g. `https://chutes.ai/app/chute/<id>`), `/generate` appended internally
- Auth: `Authorization: Bearer <apiKey>`
- Request format:
  ```json
  {
    "input_args": {
      "prompt": "...",
      "width": 1024,
      "height": 1024
    }
  }
  ```
- Response: raw binary image data (JPEG), read directly into `[]byte`
- 60-second HTTP timeout (matches existing pattern)
- Error handling: same pattern as DallE — `ErrAuthFailure` for 401/403, `ErrTimeout` for timeouts, `HTTPStatusError` for other failures

Update `ImageProvider` doc comment from "raw PNG bytes" to "raw image bytes" since the format is now JPEG.

### `internal/config/config.go`

Update `ImageProviderConfig`:

```go
type ImageProviderConfig struct {
    Type      string `json:"type"`          // "chutes" or "none"
    BaseURL   string `json:"base_url"`      // chute endpoint URL
    APIKeyEnv string `json:"api_key_env"`   // env var name for API key
    APIKey    string `json:"-"`             // resolved secret
}
```

Add validation in `config.Load()`: when `Type == "chutes"` and `BaseURL` is empty, return an error.

### `cmd/herald/main.go`

Replace:
```go
if cfg.ImageProvider.Type == "openai" ...
    imgProvider = provider.NewDallE(cfg.ImageProvider.APIKey)
```
With:
```go
if cfg.ImageProvider.Type == "chutes" ...
    imgProvider = provider.NewChutes(cfg.ImageProvider.BaseURL, cfg.ImageProvider.APIKey)
```

### `internal/provider/imagegen_test.go`

Rewrite tests for Chutes.ai format:
- httptest server returning raw JPEG binary
- Verify request JSON structure (`input_args` wrapper)
- Verify auth header (`Bearer` token)
- Error cases: API errors, empty response, timeout

### Config and docs

**`config.json.example`:**
```json
"image_provider": {
  "type": "none",
  "base_url": "https://chutes.ai/app/chute/a292d47b-8f0f-5662-b2b0-6f0ebba48031",
  "api_key_env": "CHUTES_API_KEY"
}
```

**`.env.example`:** Remove `OPENAI_API_KEY` line (already has `CHUTES_API_KEY`).

**`docs/features.md`:** Update "Image Generation" section — FLUX.1-schnell via Chutes.ai instead of DALL-E 3.

**`docs/configuration.md`:** Update `image_provider` fields — type is `"chutes"`, add `base_url` field, env var is `CHUTES_API_KEY`.

## Unchanged

- `ImageProvider` interface — `Generate(ctx, prompt) ([]byte, error)`
- `internal/agent/loop.go` — tool call parsing, `handleImageGeneration`, streaming path
- `internal/agent/context.go` — `imageToolPrompt` system prompt
- `internal/hub/hub.go` — `ImageMessage` type, `Image` channel
- `internal/telegram/adapter.go` — `dispatchImage` goroutine
- Agent loop tests for tool call parsing and image generation flow (may need constructor call updates if `NewLoop` signature stays the same — it should, since it takes `provider.ImageProvider`)
