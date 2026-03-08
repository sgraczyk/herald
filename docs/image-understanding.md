# Image Understanding

Herald analyzes images sent via Telegram by routing them to an OpenAI-compatible vision API. Photos are downloaded, preprocessed in-memory, and sent as inline base64 data URIs. Images are ephemeral -- never persisted to bbolt.

## User Guide

Send a photo to Herald in Telegram. No commands or setup needed -- Herald automatically detects images and responds.

**With a caption:** Herald uses your caption as the prompt (e.g., "What does this sign say?", "Translate the text in this image.").

**Without a caption:** Herald defaults to "What's in this image?" and gives a general description.

### Requirements

A vision-capable OpenAI-compatible provider must be configured (e.g., Chutes.ai with a `VL` model). The `claude -p` provider cannot handle images -- Herald automatically routes photos to a compatible provider.

### Limitations

- **Images are not saved.** Only a `[image] <caption>` placeholder is stored in history. Send the photo again for follow-up questions.
- **Large images are resized.** Images over 2000px in either dimension are scaled down automatically.
- **WEBP passthrough.** WEBP images are not resized (no Go stdlib decoder without CGO) but are still subject to the 4 MB size cap.
- **Single image per message.** Telegram sends one photo per message.

## Architecture

### Data Flow

```
Telegram Photo
    |
    v
adapter.handlePhoto()
    |-- Select largest PhotoSize (last in array)
    |-- bot.GetFile() -> HTTP GET (30s timeout, 20MB LimitReader)
    |-- http.DetectContentType() for MIME
    |-- provider.PreprocessImage(): decode -> resize -> JPEG encode -> base64
    |
    v
hub.InMessage{Text: caption, Images: []ImageAttachment{...}}
    |
    v
agent.Loop.handleMessage()
    |-- buildMessages() for context
    |-- Convert hub.ImageAttachment -> provider.ImageData
    |
    v
provider.Fallback.Chat()
    |-- hasImages() -> filterVisionProviders() (keeps only *OpenAI)
    |-- OpenAI.Chat() -> buildOpenAIContent()
    |     |-- [{type:"text",...}, {type:"image_url", image_url:{url:"data:..."}}]
    |
    v
API Response -> store placeholder -> hub.OutMessage -> Telegram
```

### Preprocessing Pipeline (`internal/provider/image.go`)

```
Raw bytes + MIME type
    |
    +-- WEBP? --> base64 encode directly (no resize) --> size check --> ImageData
    |
    +-- JPEG/PNG --> image.Decode() --> check dimensions
                          |
                          +-- > 2000px --> resizeImage() (CatmullRom)
                          +-- <= 2000px --> pass through
                                   |
                             jpeg.Encode(quality=85) -> base64 -> size check (4MB) -> ImageData
```

| Constant | Value | Purpose |
|----------|-------|---------|
| `maxImageDimension` | 2000px | Resize threshold |
| `maxBase64Size` | 4 MB | Reject after encode |
| Download limit | 20 MB | `io.LimitReader` on download |
| JPEG quality | 85 | Re-encoding compression |
| Download timeout | 30s | HTTP GET timeout |

### Provider Routing

The fallback chain in `internal/provider/fallback.go`:

1. `hasImages(messages)` scans for `len(m.Images) > 0`
2. `filterVisionProviders(providers)` keeps only `*OpenAI` via type assertion
3. No vision provider → error: `"no vision-capable provider configured"`
4. Text-only messages use the normal chain unchanged

Claude CLI is skipped for images because pipe mode (`claude -p --output-format json`) only accepts text via stdin.

### Security Controls

| Control | Mechanism | Location |
|---------|-----------|----------|
| Download size cap | `io.LimitReader(resp.Body, 20<<20)` | `telegram/adapter.go` |
| Base64 size cap | Reject if > 4 MB | `provider/image.go` |
| Dimension cap | Resize if > 2000px | `provider/image.go` |
| Download timeout | 30s `context.WithTimeout` | `telegram/adapter.go` |
| Authorization | Photo handling after whitelist check | `telegram/adapter.go` |
