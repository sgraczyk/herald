# Image Understanding

Developer reference for Herald's image understanding feature (PR #98, closes #39).

Herald can analyze images sent via Telegram by routing them to an OpenAI-compatible vision API. Photos are downloaded, preprocessed in-memory, and sent as inline base64 data URIs. Images are ephemeral -- they are never persisted to bbolt.

## Architecture

### Data Flow

```
Telegram Photo
    |
    v
adapter.handlePhoto()
    |-- Select largest PhotoSize (last element in array)
    |-- bot.GetFile() to get file path
    |-- HTTP GET with 30s timeout + LimitReader(20MB)
    |-- http.DetectContentType() for MIME type
    |-- provider.PreprocessImage(): decode -> resize -> JPEG encode -> base64
    |
    v
hub.InMessage{Text: caption, Images: []ImageAttachment{...}}
    |
    v
agent.Loop.handleMessage()
    |-- Load history + memories from bbolt
    |-- buildMessages() for conversation context
    |-- Convert hub.ImageAttachment -> provider.ImageData on last message
    |
    v
provider.Fallback.Chat()
    |-- hasImages() detects image content
    |-- filterVisionProviders() keeps only *OpenAI instances
    |-- OpenAI.Chat() calls buildOpenAIContent()
    |     |-- Content array: [{type:"text",...}, {type:"image_url",...}]
    |
    v
API Response -> store "[image] <caption>" in bbolt -> hub.OutMessage -> Telegram
```

### Design Decisions

**Ephemeral images.** Only a `[image] <caption>` placeholder is stored in conversation history. The base64 data lives in memory for the duration of the API call only. This keeps bbolt small and avoids storing binary blobs in the embedded KV store.

**Auto-routing.** The fallback chain inspects messages for image content and automatically routes to vision-capable providers. Text-only messages use the normal fallback chain unchanged.

**Inline base64 data URIs.** Image data is sent as `data:<mime>;base64,<data>` in the API request body, not as external URLs. This avoids requiring the vision API to reach a public endpoint.

**Default prompt.** If no caption is provided with the photo, the text defaults to `"What's in this image?"`.

## Image Preprocessing Pipeline

Defined in `internal/provider/image.go`.

```
Raw bytes + MIME type
    |
    +-- WEBP? --> base64 encode directly (no resize) --> size check --> ImageData
    |
    +-- JPEG/PNG --> image.Decode() --> check dimensions
                          |
                          +-- > 2000px --> resizeImage() (CatmullRom)
                          |
                          +-- <= 2000px --> pass through
                                   |
                             jpeg.Encode(quality=85)
                                   |
                             base64.StdEncoding
                                   |
                             size check (4MB cap)
                                   |
                             ImageData{MimeType: "image/jpeg"}
```

### Constants

| Constant | Value | Purpose |
|----------|-------|---------|
| `maxImageDimension` | 2000 | Max width or height in pixels before resize |
| `maxBase64Size` | 4 MB (`4 << 20`) | Reject images exceeding this after encode |
| Download limit | 20 MB (`20 << 20`) | `io.LimitReader` cap on photo download |
| JPEG quality | 85 | Compression quality for re-encoded output |
| Download timeout | 30 seconds | `context.WithTimeout` on HTTP GET |

### Implementation Notes

- All JPEG/PNG input is re-encoded as JPEG at quality 85 regardless of input format.
- PNG decoding registered via blank import (`_ "image/png"`).
- Resize uses CatmullRom interpolation from `golang.org/x/image/draw`, preserving aspect ratio.
- WEBP cannot be decoded by the Go stdlib without CGO, so it passes through without resize (still subject to the 4 MB base64 cap).

## Provider Routing

The fallback chain in `internal/provider/fallback.go` handles image routing:

1. `hasImages(messages)` scans all messages for `len(m.Images) > 0`.
2. If images are present, `filterVisionProviders(providers)` keeps only `*OpenAI` instances via type assertion.
3. If no vision-capable provider exists, returns error: `"no vision-capable provider configured"`.
4. Otherwise, the filtered list proceeds through normal fallback logic.

Text-only messages are completely unaffected -- they use the standard chain (`claude -p` first, then OpenAI).

### Why claude -p is Skipped

The Claude CLI in pipe mode (`claude -p --output-format json`) accepts only text via stdin. There is no mechanism to pass image data through the CLI interface. Vision requests must go through an HTTP API that supports multimodal content arrays.

### OpenAI Content Array Construction

`buildOpenAIContent()` in `internal/provider/openai.go` returns:

- **Text-only message:** plain `string`
- **Message with images:** `[]map[string]any` content array

```json
[
  {"type": "text", "text": "describe this"},
  {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,..."}}
]
```

The text part is only added when `m.Content != ""`. Multiple images produce multiple `image_url` entries. The `openaiMessage.Content` field is typed as `any` to accommodate both forms.

## Key Types

### provider.ImageData

```go
// provider/provider.go
type ImageData struct {
    Base64   string `json:"base64"`
    MimeType string `json:"mime_type"`
}
```

Added as `Images []ImageData` field on `provider.Message` (tagged `json:"images,omitempty"`).

### hub.ImageAttachment

```go
// hub/hub.go
type ImageAttachment struct {
    Base64   string
    MimeType string
}
```

Added as `Images []ImageAttachment` field on `hub.InMessage`.

### Type Flow Between Packages

```
telegram/adapter.go           hub/hub.go               agent/loop.go            provider/provider.go
-----------------           ----------               -------------            -------------------
provider.PreprocessImage()
    -> ImageData            ImageAttachment           hub.ImageAttachment       provider.ImageData
                               (on InMessage)         -> provider.ImageData        (on Message)
                                                        (field copy)
```

The agent loop converts `hub.ImageAttachment` to `provider.ImageData` when attaching images to the last provider message. The types are structurally identical but belong to different packages to maintain separation.

### Key Functions

| Function | Package | Exported | Purpose |
|----------|---------|----------|---------|
| `PreprocessImage(data, mimeType)` | provider | Yes | Full pipeline: decode, resize, compress, base64 |
| `resizeImage(img, w, h)` | provider | No | Scale to fit within max dimension |
| `buildOpenAIContent(m)` | provider | No | Build string or content array for API |
| `hasImages(messages)` | provider | No | Check if any message contains images |
| `filterVisionProviders(providers)` | provider | No | Keep only `*OpenAI` instances |
| `handlePhoto(ctx, b, msg, chatID, userID)` | telegram | No | Download, preprocess, route to hub |
| `sendError(ctx, chatID, text)` | telegram | No | Send plain-text error to user |

## Security

| Control | Mechanism | Location |
|---------|-----------|----------|
| Download size cap | `io.LimitReader(resp.Body, 20<<20)` | `telegram/adapter.go` |
| Base64 size cap | Reject if encoded size > 4 MB | `provider/image.go` |
| Dimension cap | Resize if width or height > 2000px | `provider/image.go` |
| Download timeout | 30s `context.WithTimeout` on HTTP GET | `telegram/adapter.go` |
| User authorization | Photo handling runs after whitelist check in `handleUpdate()` | `telegram/adapter.go` |

Unauthorized users cannot trigger photo downloads or API calls. The download cap prevents memory exhaustion on the 512 MB container.

## Limitations

- **WEBP passthrough.** WEBP images are not resized because Go's stdlib has no WEBP decoder without CGO. They are base64-encoded as-is and rejected if they exceed 4 MB.
- **No image persistence.** Images are not stored in bbolt. Only a `[image] <caption>` text placeholder is saved. The LLM has no memory of previous images in follow-up messages.
- **No multi-image history.** Images are attached only to the current message. Prior conversation turns never contain image data, even if images were sent previously.
- **claude -p cannot handle images.** The CLI pipe mode only accepts text. All image messages bypass `claude -p` entirely.
- **Peak memory usage.** A single 4000x3000 image requires ~48 MB decoded in memory, plus the resize destination buffer. Serial processing keeps this safe for the 512 MB target, but concurrent image processing is not supported.
- **Single image per message.** Telegram sends one photo per message (though the type system supports multiple images per message for future extensibility).

## Test Coverage

Tests use hand-written test doubles and `httptest.NewServer` -- no mocking frameworks.

| File | Tests | Coverage |
|------|-------|----------|
| `provider/image_test.go` | 6 | Small/large JPEG, PNG conversion, WEBP passthrough, invalid data, portrait resize |
| `provider/openai_test.go` | 3 (new) | Text-only content, image content array, HTTP roundtrip with image |
| `provider/fallback_test.go` | 3 (new) | Image routes to OpenAI, no-provider error, text unaffected |

Not tested in isolation: `telegram/adapter.handlePhoto()` (requires Telegram Bot API mock), `agent/loop.handleMessage()` image attachment logic.
