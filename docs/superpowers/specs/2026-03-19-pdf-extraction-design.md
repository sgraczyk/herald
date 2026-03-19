# PDF Text Extraction and Conversation Integration

Design spec for Herald issue #146 — user sends a PDF to Herald via Telegram, Herald extracts text and discusses it.

## Context

Herald v0.6.1 handles text and image messages. This feature adds PDF document support following the same adapter → hub → agent pattern used for photos. Target hardware is Intel N150 with 16GB RAM in an LXC container.

### Constraints

- Pure Go, no CGO
- Single-column documents: invoices, receipts, contracts, letters
- ~10 MB peak memory per request (safe for N150/16GB)
- No OCR for scanned PDFs — text-based only
- Single active document per conversation

### User preferences

- Document persists in conversation until `/new` or `/clear`
- Page-based truncation when over token budget
- No new commands — automatic on PDF attachment

---

## 1. New Package: `internal/document/`

### Types (`types.go`)

```go
// Document holds extracted text from a file attachment.
type Document struct {
    Name       string // original filename
    MimeType   string // e.g. "application/pdf"
    Pages      int    // total page count
    Text       string // extracted text (may be truncated)
    Truncated  bool   // true if text was cut to fit token budget
    ShownPages int    // how many pages fit within budget
}

// Extractor defines how to extract text from a document.
type Extractor interface {
    Extract(r io.ReaderAt, size int64, name string) (*Document, error)
}
```

### PDF implementation (`pdf.go`)

Uses `github.com/ledongthuc/pdf` (~580 stars, pure Go, `rsc/pdf` lineage).

```go
// PDFExtractor extracts text from PDF files.
type PDFExtractor struct {
    maxTokens int // token budget for extracted text
}
```

**Extraction flow:**

1. `pdf.NewReader(r, size)` — random access via `io.ReaderAt`, no full load into memory
2. Iterate pages via `Reader.Page(n)` and extract text with `GetTextByRow()` for row-level structure
3. Accumulate text page-by-page, estimating tokens at 4 chars/token
4. Stop when token budget exhausted — set `Truncated: true`, record `ShownPages`
5. Run text density check on each page's output

**Panic safety:** Wrap `pdf.NewReader` and `GetPlainText` calls in a `recover()` block. `ledongthuc/pdf` may panic on malformed PDFs — convert to an error return. This is consistent with Herald's existing `recover()` usage in background goroutines (`loop.go`).

**Why `ledongthuc/pdf`:**

- Most battle-tested pure-Go option for simple Latin-text documents
- Page-by-page API enables page-based truncation
- `io.ReaderAt` interface keeps memory low
- Unmaintained since 2021 but PDF is a stable format; `dslipak/pdf` (identical API) is a drop-in replacement if needed

**Why an `Extractor` interface:**

- Keeps Telegram adapter decoupled from PDF library
- Leaves room for voice transcription (#156) to use the same pattern without restructuring

### Error types (`errors.go`)

| Sentinel | Trigger | User-facing message |
|----------|---------|---------------------|
| `ErrEncrypted` | `pdf.NewReader` fails on encrypted file | "Sorry, I can't read encrypted PDFs." |
| `ErrNoText` | All pages produce empty text | "This PDF appears to be scanned/image-based. Text extraction isn't supported yet." |
| `ErrGarbled` | Text density below threshold | Same as `ErrNoText` |
| `ErrMalformed` | Panic recovered during extraction | "Couldn't process this PDF. The file may be corrupted." |

**Text density check:** Ratio of readable characters (`unicode.IsLetter`, `unicode.IsSpace`, `unicode.IsDigit`, `unicode.IsPunct`) to total runes. Threshold: 0.8. This is Unicode-aware and handles accented characters (Polish, German, French) correctly. Density check runs on the first non-empty page; if it fails, bail early.

---

## 2. Hub Changes

`hub.InMessage` gains one field:

```go
Document *document.Document // optional document attachment
```

Pointer type — nil when no document is attached. Single document only, matching the current constraint.

No new hub channels. Documents flow through existing `In` → agent → `Out` path.

---

## 3. Telegram Adapter Changes

New `handleDocument` method in `adapter.go`, mirroring `handlePhoto`.

**Detection:** In `handleUpdate`, after the photo check and before the text check:

```go
if msg.Document != nil && msg.Document.MimeType == "application/pdf" {
    a.handleDocument(ctx, b, msg, chatID, userID)
    return
}
```

Other MIME types fall through to the existing text handler. This placement ensures document messages (which use `Caption`, not `Text`) are not silently dropped.

**Flow:**

1. **Size guard:** `msg.Document.FileSize > 10 MB` → `sendError("PDF too large (max 10 MB).")`. Note: `FileSize` is best-effort from Telegram; the `io.LimitReader` at step 2 is the true enforcement.
2. **Download:** `GetFile` → `FileDownloadLink` → HTTP GET with 30s timeout, `io.LimitReader` at 10 MB
3. **Extract:** `bytes.Reader` wraps `[]byte` (implements `io.ReaderAt`), pass to `Extractor.Extract()`
4. **Caption:** `msg.Caption` as user text, default "What's in this document?" if empty
5. **Error mapping:** Typed errors → user-friendly messages via `sendError`
6. **Send:** Set `Document` on `hub.InMessage`, send to `hub.In`

The adapter holds a `document.Extractor` field, injected via constructor.

---

## 4. Agent Context Integration

### Document injection flow

When a message arrives with a document, `handleMessage` in `loop.go`:

1. Appends a system-role message containing the formatted document text to the store (via `store.Append`)
2. Appends the user message placeholder to the store
3. Builds the provider message list via `buildMessages` — the document system message is now part of `history`

On follow-up messages (no new document), `buildMessages` picks up the document system message naturally from history. **No special parameter or re-injection needed** — the document persists as a regular history entry.

### Document system message format

```
--- Document: invoice.pdf (3 pages) ---
{extracted text}
--- End of document ---
```

When truncated:

```
--- Document: invoice.pdf (3/5 pages shown) ---
{extracted text}
--- End of document (2 pages omitted due to length) ---
```

### History persistence

- **Document text:** Stored as a system-role message via `store.Append` before the user message
- **User message placeholder:** `"[document: invoice.pdf] What's the total?"`
- Document text counts against `history_token_budget` — gets pruned naturally with old messages

### Summarization handling

When `maybeSummarize` encounters a system-role document message in the pending prune list, it replaces the full text with a short placeholder before passing to the summarizer:

```
system: [document: invoice.pdf, 3 pages]
```

This prevents a 16KB document blob from overwhelming the extraction provider's context window and produces better summaries (e.g., "User shared an invoice from Acme Corp totaling $1,234").

### Document lifecycle

- Document persists in history for the entire conversation
- New PDF does not remove old one — old document lives in history, gets summarized/pruned normally

---

## 5. Configuration

New field in `config.Config`:

```go
MaxDocumentTokens int `json:"max_document_tokens,omitempty"`
```

Default: `4000` (~16KB text). Set in `Load()` alongside other defaults.

Passed to `document.PDFExtractor` at construction in `cmd/herald/main.go`.

Update `config.json.example` and `docs/configuration.md` to document the new field.

---

## 6. Memory & Performance

| Component | Peak RAM |
|-----------|----------|
| Download buffer | 10 MB (limit) |
| `bytes.Reader` wrapper | ~0 (references same slice) |
| `ledongthuc/pdf` Reader | Small — random access, no full parse |
| Extracted text (~4000 tokens) | ~16 KB |
| **Total peak** | **~10-11 MB per request** |

- No temp files — everything in memory, 10 MB limit makes this safe
- No concurrency concern — single-user, one message at a time through agent loop
- Download buffer GC'd after `InMessage` sent to hub
- `ledongthuc/pdf` is a small pure-Go dependency — no binary size bloat

---

## 7. Testing

- **Unit tests (`internal/document/`):** Extract from test PDF fixtures, verify page count, text content, truncation behavior, error detection (encrypted, empty, garbled, malformed)
- **Unit tests (`internal/agent/`):** Verify document system message appears in history and context correctly
- **Unit tests (`internal/telegram/`):** Verify `handleDocument` routes correctly, size limit enforcement
- **Integration:** Manual — send real PDFs via Telegram, verify extraction and follow-up questions work

Test fixtures stored in `internal/document/testdata/` per Go conventions: small valid PDF, encrypted PDF, empty/image-only PDF, multi-page PDF (for truncation).
