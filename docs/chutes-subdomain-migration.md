# Chutes.ai Per-Chute Subdomain URL Migration

PR #107 | Branch: `fix/chutes-subdomain-url` | 2026-03-08

## Overview

Chutes.ai migrated from a shared API endpoint to per-chute subdomain URLs. This was a config-and-docs-only change -- no Go code was modified.

| | Before | After |
|---|--------|-------|
| **Base URL** | `https://api.chutes.ai/v1` | `https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1` |
| **Model** | `Qwen/Qwen2.5-VL-72B-Instruct` | `Qwen/Qwen2.5-VL-32B-Instruct` |

The subdomain format is `chutes-` followed by the chute name with slashes replaced by hyphens and dots removed. Both the 72B and 32B models are vision-language models (the `VL` suffix). The 72B references were stale.

## Why No Code Changes Were Needed

The `OpenAI` provider in `internal/provider/openai.go` treats `baseURL` as an opaque string. Endpoint construction uses simple concatenation:

- Chat completions: `baseURL + "/chat/completions"`
- Model validation: `baseURL + "/models"`

The provider does not parse, validate, or manipulate the host or path. Any valid HTTPS base URL works. The `Authorization` header and API key mechanism are also unchanged. This URL-agnostic design made the migration a config-only change.

The startup validation added in PR #103 (`ValidateProviders` calling `GET {baseURL}/models`) continues to work without modification -- the per-chute subdomain URLs serve the `/models` endpoint identically.

## Updated Config

```json
{
  "name": "chutes",
  "type": "openai",
  "base_url": "https://chutes-qwen-qwen2-5-vl-32b-instruct.chutes.ai/v1",
  "model": "Qwen/Qwen2.5-VL-32B-Instruct",
  "api_key_env": "CHUTES_API_KEY"
}
```

## Files Modified

| File | Change |
|------|--------|
| `config.json.example` | Updated `base_url` to per-chute subdomain; updated `model` from 72B to 32B |
| `AGENTS.md` | Updated stale 72B model references to match the new config |

## Future Provider URL Changes

If Chutes.ai or another OpenAI-compatible provider changes their URL scheme again:

1. Update `base_url` in the deployed `config.json` and in `config.json.example`
2. Verify the new URL serves `/models` and `/chat/completions` endpoints
3. No code changes needed unless the authentication scheme changes
