# PDF Text Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Users send a PDF to Herald via Telegram, Herald extracts text and discusses it in conversation.

**Architecture:** New `internal/document/` package extracts PDF text via `ledongthuc/pdf`. Telegram adapter downloads the file, extracts text, and attaches a `Document` to `hub.InMessage`. The agent loop stores document text as a system-role history message so follow-up questions work naturally. Page-based truncation respects a configurable token budget.

**Tech Stack:** `github.com/ledongthuc/pdf` (pure Go PDF reader), existing `go-telegram/bot`, `bbolt`

**Spec:** `docs/superpowers/specs/2026-03-19-pdf-extraction-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `internal/document/document.go` | `Document` struct, `Extractor` interface, `FormatContext` helper |
| Create | `internal/document/pdf.go` | `PDFExtractor` — wraps `ledongthuc/pdf`, page-by-page extraction with token budget |
| Create | `internal/document/pdf_test.go` | Tests for PDF extraction, truncation, error detection, density check |
| Create | `internal/document/testdata/` | Test fixture PDFs |
| Modify | `internal/hub/hub.go:14-20` | Add `Document *document.Document` field to `InMessage` |
| Modify | `internal/config/config.go:12-28` | Add `MaxDocumentTokens` field to `Config` |
| Modify | `internal/config/config.go:58-123` | Default `MaxDocumentTokens` to 4000 in `LoadWithDefaults` |
| Modify | `internal/telegram/adapter.go:36-44` | Add `extractor` field to `Adapter` |
| Modify | `internal/telegram/adapter.go:48-75` | Accept `Extractor` in `New()` constructor |
| Modify | `internal/telegram/adapter.go:88-126` | Insert document check in `handleUpdate` |
| Create | (in adapter.go) | `handleDocument` method |
| Modify | `internal/agent/loop.go:420-485` | Save document system message in `saveAndProcess` |
| Modify | `internal/agent/loop.go:595-643` | Compact document text in `maybeSummarize` |
| Modify | `cmd/herald/main.go:73-166` | Create `PDFExtractor`, pass to `telegram.New` |
| Modify | `config.json.example` | Add `max_document_tokens` field |
| Modify | `docs/configuration.md` | Document `max_document_tokens` |

---

### Task 1: Add dependency and create document package types

**Files:**
- Create: `internal/document/document.go`

- [ ] **Step 1: Add `ledongthuc/pdf` dependency**

```bash
go get github.com/ledongthuc/pdf
```

- [ ] **Step 2: Create `internal/document/document.go`**

```go
// Package document handles text extraction from file attachments.
package document

import (
	"fmt"
	"io"
	"unicode"
)

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

// FormatContext formats a document for injection into conversation context.
func FormatContext(doc *Document) string {
	var header, footer string
	if doc.Truncated {
		header = fmt.Sprintf("--- Document: %s (%d/%d pages shown) ---", doc.Name, doc.ShownPages, doc.Pages)
		omitted := doc.Pages - doc.ShownPages
		footer = fmt.Sprintf("--- End of document (%d pages omitted due to length) ---", omitted)
	} else {
		header = fmt.Sprintf("--- Document: %s (%d pages) ---", doc.Name, doc.Pages)
		footer = "--- End of document ---"
	}
	return header + "\n" + doc.Text + "\n" + footer
}

// textDensity returns the ratio of letters and whitespace to total runes.
// Used to detect garbled output from PDFs with non-standard font encodings.
func textDensity(text string) float64 {
	if len(text) == 0 {
		return 0
	}
	var total, readable int
	for _, r := range text {
		total++
		if unicode.IsLetter(r) || unicode.IsSpace(r) || unicode.IsDigit(r) || unicode.IsPunct(r) {
			readable++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(readable) / float64(total)
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/document/`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/document/document.go go.mod go.sum
git commit -m "feat(document): add document package with types and extractor interface"
```

---

### Task 2: Implement PDF extractor with tests

**Files:**
- Create: `internal/document/pdf.go`
- Create: `internal/document/pdf_test.go`
- Create: `internal/document/testdata/` (test fixtures)

- [ ] **Step 1: Create test fixtures**

Create minimal PDF test fixtures programmatically. We need a helper that creates PDFs for testing:

```bash
mkdir -p internal/document/testdata
```

- [ ] **Step 2: Write failing tests in `internal/document/pdf_test.go`**

```go
package document

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestPDFExtractorBasic(t *testing.T) {
	// Use a minimal valid PDF with known text.
	// We'll create this with a helper that generates a tiny PDF.
	data := minimalPDF("Hello, World!")
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(4000)
	doc, err := ext.Extract(r, int64(len(data)), "test.pdf")
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if doc.Name != "test.pdf" {
		t.Errorf("Name = %q, want %q", doc.Name, "test.pdf")
	}
	if doc.MimeType != "application/pdf" {
		t.Errorf("MimeType = %q, want %q", doc.MimeType, "application/pdf")
	}
	if doc.Pages != 1 {
		t.Errorf("Pages = %d, want 1", doc.Pages)
	}
	if !strings.Contains(doc.Text, "Hello") {
		t.Errorf("Text = %q, expected to contain 'Hello'", doc.Text)
	}
	if doc.Truncated {
		t.Error("Truncated = true, want false")
	}
	if doc.ShownPages != 1 {
		t.Errorf("ShownPages = %d, want 1", doc.ShownPages)
	}
}

func TestPDFExtractorTruncation(t *testing.T) {
	// Create a PDF where text exceeds a very small token budget.
	longText := strings.Repeat("This is a test sentence. ", 100) // ~2500 chars
	data := minimalPDF(longText)
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(10) // Very small budget: ~40 chars
	doc, err := ext.Extract(r, int64(len(data)), "long.pdf")
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if !doc.Truncated {
		t.Error("Truncated = false, want true for small token budget")
	}
}

func TestPDFExtractorEmptyPDF(t *testing.T) {
	// A PDF with no text content should return ErrNoText.
	data := minimalPDF("")
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(4000)
	_, err := ext.Extract(r, int64(len(data)), "empty.pdf")
	if err != ErrNoText {
		t.Errorf("Extract() error = %v, want ErrNoText", err)
	}
}

func TestPDFExtractorMalformedPDF(t *testing.T) {
	// Random bytes should not panic, should return an error.
	data := []byte("this is not a PDF at all")
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(4000)
	_, err := ext.Extract(r, int64(len(data)), "bad.pdf")
	if err == nil {
		t.Error("Extract() expected error for malformed PDF, got nil")
	}
}

func TestPDFExtractorEncrypted(t *testing.T) {
	// ledongthuc/pdf returns an error containing "encrypted" for password-protected PDFs.
	// We can't easily generate an encrypted PDF without a library, so test the
	// isEncryptedError detection directly and verify the sentinel propagates.
	ext := NewPDFExtractor(4000)

	// A PDF header followed by an Encrypt dictionary entry will trigger
	// an error from the library. If it doesn't, at least verify that
	// isEncryptedError correctly detects the pattern.
	if !isEncryptedError(fmt.Errorf("file is encrypted")) {
		t.Error("isEncryptedError should detect 'encrypted' in error message")
	}
	if !isEncryptedError(fmt.Errorf("password required")) {
		t.Error("isEncryptedError should detect 'password' in error message")
	}
	if isEncryptedError(fmt.Errorf("invalid format")) {
		t.Error("isEncryptedError should not match unrelated errors")
	}
	_ = ext // suppress unused
}

func TestTextDensity(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantLow bool // true if density should be below 0.8
	}{
		{"normal text", "Hello, this is a normal PDF document.", false},
		{"accented text", "Zażółć gęślą jaźń — polskie znaki.", false},
		{"garbled", "\x00\x01\x02\x03\x04\x05normal\x06\x07\x08\x09", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := textDensity(tt.text)
			if tt.wantLow && d >= 0.8 {
				t.Errorf("textDensity(%q) = %f, expected < 0.8", tt.text, d)
			}
			if !tt.wantLow && d < 0.8 {
				t.Errorf("textDensity(%q) = %f, expected >= 0.8", tt.text, d)
			}
		})
	}
}

func TestFormatContext(t *testing.T) {
	doc := &Document{
		Name:       "invoice.pdf",
		Pages:      3,
		Text:       "Some invoice text",
		Truncated:  false,
		ShownPages: 3,
	}
	got := FormatContext(doc)
	if !strings.Contains(got, "--- Document: invoice.pdf (3 pages) ---") {
		t.Errorf("expected non-truncated header, got %q", got)
	}
	if !strings.Contains(got, "--- End of document ---") {
		t.Errorf("expected end marker, got %q", got)
	}

	doc.Truncated = true
	doc.Pages = 5
	doc.ShownPages = 3
	got = FormatContext(doc)
	if !strings.Contains(got, "(3/5 pages shown)") {
		t.Errorf("expected truncated header, got %q", got)
	}
	if !strings.Contains(got, "2 pages omitted") {
		t.Errorf("expected omitted note, got %q", got)
	}
}

// minimalPDF creates a minimal valid PDF with the given text on one page.
// This is a hand-crafted PDF structure — the smallest valid PDF that
// ledongthuc/pdf can parse.
func minimalPDF(text string) []byte {
	// Build a minimal PDF with a single page containing the text.
	// This uses raw PDF syntax to avoid needing a PDF generation library.
	var b bytes.Buffer

	b.WriteString("%PDF-1.0\n")

	// Object 1: Catalog
	obj1Offset := b.Len()
	b.WriteString("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n")

	// Object 2: Pages
	obj2Offset := b.Len()
	b.WriteString("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n")

	// Object 3: Page
	obj3Offset := b.Len()
	b.WriteString("3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n")

	// Object 4: Content stream
	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET"
	obj4Offset := b.Len()
	b.WriteString("4 0 obj<</Length " + strconv.Itoa(len(stream)) + ">>stream\n")
	b.WriteString(stream)
	b.WriteString("\nendstream endobj\n")

	// Object 5: Font
	obj5Offset := b.Len()
	b.WriteString("5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n")

	// Cross-reference table
	xrefOffset := b.Len()
	b.WriteString("xref\n0 6\n")
	b.WriteString("0000000000 65535 f \n")
	b.WriteString(padOffset(obj1Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj2Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj3Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj4Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj5Offset) + " 00000 n \n")

	// Trailer
	b.WriteString("trailer<</Size 6/Root 1 0 R>>\n")
	b.WriteString("startxref\n")
	b.WriteString(strconv.Itoa(xrefOffset) + "\n")
	b.WriteString("%%EOF\n")

	return b.Bytes()
}

func padOffset(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}
```

Note: the `minimalPDF` helper and `padOffset` go in the test file since they're only used by tests.

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/document/ -v -run TestPDFExtractor`
Expected: compilation error — `NewPDFExtractor` not defined

- [ ] **Step 4: Create `internal/document/pdf.go`**

```go
package document

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// Sentinel errors for PDF extraction failures.
var (
	ErrEncrypted = errors.New("encrypted PDF")
	ErrNoText    = errors.New("no text content")
	ErrGarbled   = errors.New("garbled text output")
	ErrMalformed = errors.New("malformed PDF")
)

const (
	// charsPerToken is the estimated characters per token for budget calculations.
	charsPerToken = 4
	// minTextDensity is the minimum ratio of readable characters to total runes.
	minTextDensity = 0.8
)

// PDFExtractor extracts text from PDF files using ledongthuc/pdf.
type PDFExtractor struct {
	maxTokens int
}

// NewPDFExtractor creates a PDFExtractor with the given token budget.
func NewPDFExtractor(maxTokens int) *PDFExtractor {
	return &PDFExtractor{maxTokens: maxTokens}
}

// Extract reads a PDF from r and returns a Document with extracted text.
// Extraction stops when the token budget is exhausted.
func (e *PDFExtractor) Extract(r io.ReaderAt, size int64, name string) (doc *Document, err error) {
	// Recover from panics in the PDF library.
	defer func() {
		if r := recover(); r != nil {
			doc = nil
			err = fmt.Errorf("%w: %v", ErrMalformed, r)
		}
	}()

	reader, err := pdf.NewReader(r, size)
	if err != nil {
		if isEncryptedError(err) {
			return nil, ErrEncrypted
		}
		return nil, fmt.Errorf("open PDF: %w", err)
	}

	totalPages := reader.NumPage()
	if totalPages == 0 {
		return nil, ErrNoText
	}

	maxChars := e.maxTokens * charsPerToken
	var text strings.Builder
	var shownPages int
	truncated := false

	for i := 1; i <= totalPages; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}

		pageText, extractErr := extractPageText(page)
		if extractErr != nil {
			continue // Skip unreadable pages.
		}

		pageText = strings.TrimSpace(pageText)
		if pageText == "" {
			continue
		}

		// Check text density on the first non-empty page.
		if shownPages == 0 && textDensity(pageText) < minTextDensity {
			return nil, ErrGarbled
		}

		// Check if adding this page would exceed the budget.
		if text.Len()+len(pageText)+1 > maxChars {
			truncated = true
			break
		}

		if text.Len() > 0 {
			text.WriteString("\n")
		}
		text.WriteString(pageText)
		shownPages++
	}

	if shownPages == 0 {
		return nil, ErrNoText
	}

	return &Document{
		Name:       name,
		MimeType:   "application/pdf",
		Pages:      totalPages,
		Text:       text.String(),
		Truncated:  truncated,
		ShownPages: shownPages,
	}, nil
}

// extractPageText gets plain text from a PDF page, recovering from panics.
func extractPageText(page pdf.Page) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text = ""
			err = fmt.Errorf("page extraction panic: %v", r)
		}
	}()

	rows, err := page.GetTextByRow()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, row := range rows {
		for _, word := range row.Content {
			if b.Len() > 0 {
				b.WriteString(" ")
			}
			b.WriteString(word.S)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// isEncryptedError checks if a PDF library error indicates encryption.
func isEncryptedError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "encrypted") || strings.Contains(msg, "password")
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/document/ -v`
Expected: all tests pass. If `minimalPDF` fixture doesn't parse correctly with `ledongthuc/pdf`, adjust the PDF structure until tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/document/
git commit -m "feat(document): implement PDF text extraction with ledongthuc/pdf"
```

---

### Task 3: Add config field and hub type change

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/hub/hub.go`
- Modify: `config.json.example`

- [ ] **Step 1: Add `MaxDocumentTokens` to config**

In `internal/config/config.go`, add the field to `Config` struct (after `HistoryTokenBudget`, line 18):

```go
MaxDocumentTokens  int              `json:"max_document_tokens,omitempty"`
```

In `LoadWithDefaults`, after the `HistoryTokenBudget` default (after line 79):

```go
if cfg.MaxDocumentTokens == 0 {
    cfg.MaxDocumentTokens = 4000
}
```

- [ ] **Step 2: Add `Document` field to `hub.InMessage`**

In `internal/hub/hub.go`, add import and field. Add to imports:

```go
"github.com/sgraczyk/herald/internal/document"
```

Add field to `InMessage` (after `Images`, line 19):

```go
Document *document.Document // optional document attachment
```

- [ ] **Step 3: Add `max_document_tokens` to `config.json.example`**

Add after `"history_token_budget": 8000,` (line 24):

```json
"max_document_tokens": 4000,
```

- [ ] **Step 4: Verify everything compiles**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Run existing tests**

Run: `go test ./...`
Expected: all pass (no behavior change)

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/hub/hub.go config.json.example
git commit -m "feat(config): add max_document_tokens field and Document to hub.InMessage"
```

---

### Task 4: Add document handling to Telegram adapter

**Files:**
- Modify: `internal/telegram/adapter.go`

- [ ] **Step 1: Add `extractor` field to Adapter**

In `adapter.go`, add import:

```go
"github.com/sgraczyk/herald/internal/document"
```

Add field to `Adapter` struct (after `allowedIDs`, line 40):

```go
extractor  document.Extractor
```

- [ ] **Step 2: Update `New()` constructor**

Change signature to accept extractor (line 48):

```go
func New(token string, h *hub.Hub, allowedUserIDs []int64, ext document.Extractor) (*Adapter, error) {
```

Set the field in the constructor:

```go
a := &Adapter{
    hub:        h,
    allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
    typing:     make(map[int64]context.CancelFunc),
    streamMsgs: make(map[int64]int),
    extractor:  ext,
}
```

- [ ] **Step 3: Add document check to `handleUpdate`**

Insert after the photo check (after line 112) and before the text extraction (before line 114):

```go
// Handle document messages (PDF).
if msg.Document != nil && msg.Document.MimeType == "application/pdf" {
    a.handleDocument(ctx, b, msg, chatID, userID)
    return
}
```

- [ ] **Step 4: Add `handleDocument` method**

Add the method, following the `handlePhoto` pattern:

```go
const maxDocumentSize = 10 << 20 // 10 MB

func (a *Adapter) handleDocument(ctx context.Context, b *bot.Bot, msg *models.Message, chatID, userID int64) {
	if msg.Document.FileSize > maxDocumentSize {
		a.sendError(ctx, chatID, "PDF too large (max 10 MB).")
		return
	}

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: msg.Document.FileID})
	if err != nil {
		slog.Error("get document file failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	fileURL := a.bot.FileDownloadLink(file)
	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, fileURL, nil)
	if err != nil {
		slog.Error("create document download request failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("download document failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentSize))
	if err != nil {
		slog.Error("read document data failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	r := bytes.NewReader(data)
	doc, err := a.extractor.Extract(r, int64(len(data)), msg.Document.FileName)
	if err != nil {
		slog.Warn("document extraction failed",
			slog.Int64("chat_id", chatID),
			slog.String("file", msg.Document.FileName),
			slog.String("error", err.Error()),
		)
		a.sendError(ctx, chatID, documentErrorMessage(err))
		return
	}

	text := strings.TrimSpace(msg.Caption)
	if text == "" {
		text = "What's in this document?"
	}

	if a.hub.Draining() {
		slog.Debug("dropping document message, hub is draining", slog.Int64("chat_id", chatID))
		return
	}

	a.hub.In <- hub.InMessage{
		ChatID:   chatID,
		UserID:   userID,
		Text:     text,
		Document: doc,
	}
}

// documentErrorMessage maps extraction errors to user-friendly messages.
func documentErrorMessage(err error) string {
	switch {
	case errors.Is(err, document.ErrEncrypted):
		return "Sorry, I can't read encrypted PDFs."
	case errors.Is(err, document.ErrNoText), errors.Is(err, document.ErrGarbled):
		return "This PDF appears to be scanned/image-based. Text extraction isn't supported yet."
	case errors.Is(err, document.ErrMalformed):
		return "Couldn't process this PDF. The file may be corrupted."
	default:
		return "Couldn't process this PDF. Try a different file."
	}
}
```

Add `"bytes"` and `"errors"` to imports, and `"github.com/sgraczyk/herald/internal/document"`.

- [ ] **Step 5: Fix `telegram.New` call in `cmd/herald/main.go`**

Update line 121 in `cmd/herald/main.go`:

```go
tg, err := telegram.New(cfg.Telegram.Token, h, cfg.AllowedUserIDs, document.NewPDFExtractor(cfg.MaxDocumentTokens))
```

Add import:

```go
"github.com/sgraczyk/herald/internal/document"
```

- [ ] **Step 6: Fix `telegram.New` call in adapter tests**

In `internal/telegram/adapter_test.go`, update any calls to `telegram.New` to pass a nil extractor (or a mock). Search for existing calls:

If tests call `New(token, hub, ids)`, change to `New(token, hub, ids, nil)`.

- [ ] **Step 7: Verify compilation and tests**

Run: `go build ./... && go test ./...`
Expected: all pass

- [ ] **Step 8: Commit**

```bash
git add internal/telegram/adapter.go cmd/herald/main.go internal/telegram/adapter_test.go
git commit -m "feat(telegram): add PDF document handling in adapter"
```

---

### Task 5: Integrate documents into agent loop

**Files:**
- Modify: `internal/agent/loop.go`

- [ ] **Step 1: Modify `saveAndProcess` to save document system message**

In `saveAndProcess` (line 420), add document handling before saving the user message. After the `userContent` preparation and before `userMsg`:

```go
// Save document as a system-role history message before the user message.
if msg.Document != nil {
    docMsg := provider.Message{
        Role:      "system",
        Content:   document.FormatContext(msg.Document),
        Timestamp: time.Now(),
    }
    if err := l.store.Append(msg.ChatID, docMsg, l.historyLimit); err != nil {
        slog.Error("save document message failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
    }
}
```

Also update the user content placeholder for documents (in the same function, after image handling):

```go
if msg.Document != nil {
    userContent = fmt.Sprintf("[document: %s] %s", msg.Document.Name, userContent)
}
```

Add import for `"github.com/sgraczyk/herald/internal/document"`.

- [ ] **Step 2: Modify `maybeSummarize` to compact document text**

In `maybeSummarize` (line 595), replace the loop that builds the pending messages string (lines 615-618). Change:

```go
var b strings.Builder
for _, m := range pending {
    fmt.Fprintf(&b, "%s: %s\n", m.Role, m.Content)
}
```

To:

```go
var b strings.Builder
for _, m := range pending {
    content := m.Content
    // Replace large document text with a short placeholder for the summarizer.
    if m.Role == "system" && strings.HasPrefix(content, "--- Document:") {
        // Extract just the header line as a placeholder.
        if idx := strings.Index(content, "\n"); idx > 0 {
            content = content[:idx]
        }
    }
    fmt.Fprintf(&b, "%s: %s\n", m.Role, content)
}
```

- [ ] **Step 3: Run existing tests**

Run: `go test ./internal/agent/ -v`
Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add internal/agent/loop.go
git commit -m "feat(agent): save document text in history and compact for summarization"
```

---

### Task 6: Add agent loop tests for document handling

**Files:**
- Modify: `internal/agent/loop_test.go`

- [ ] **Step 1: Write test for document context in provider messages**

Add to `loop_test.go`:

```go
func TestHandleMessageWithDocument(t *testing.T) {
	cap := &capturingProvider{responses: []string{"I see an invoice.", "[]"}}
	l, h, db := testLoop(t, cap)
	l.extProvider = cap

	doc := &document.Document{
		Name:       "invoice.pdf",
		MimeType:   "application/pdf",
		Pages:      2,
		Text:       "Invoice #123\nTotal: $500",
		Truncated:  false,
		ShownPages: 2,
	}

	l.handle(context.Background(), hub.InMessage{
		ChatID:   1,
		Text:     "What's the total?",
		Document: doc,
	})
	out := readOut(t, h)
	l.Wait()

	if out.Text != "I see an invoice." {
		t.Errorf("expected 'I see an invoice.', got %q", out.Text)
	}

	// Verify the provider received a system message with document context.
	if len(cap.captured) == 0 {
		t.Fatal("provider was never called")
	}
	msgs := cap.captured[0]

	// Find the document system message in the provider call.
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "--- Document: invoice.pdf") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected document system message in provider context")
	}

	// Verify document is stored in history for follow-up questions.
	stored, _ := db.List(1)
	docFound := false
	for _, m := range stored {
		if m.Role == "system" && strings.Contains(m.Content, "Invoice #123") {
			docFound = true
		}
	}
	if !docFound {
		t.Error("expected document system message in stored history")
	}

	// Verify user message has placeholder.
	userFound := false
	for _, m := range stored {
		if m.Role == "user" && strings.Contains(m.Content, "[document: invoice.pdf]") {
			userFound = true
		}
	}
	if !userFound {
		t.Error("expected user message with document placeholder in stored history")
	}
}
```

Add import for `"github.com/sgraczyk/herald/internal/document"`.

- [ ] **Step 2: Write test for document follow-up (persistence in history)**

```go
func TestDocumentPersistsForFollowUp(t *testing.T) {
	cap := &capturingProvider{responses: []string{"The total is $500.", "[]", "The date is Jan 1.", "[]"}}
	l, h, _ := testLoop(t, cap)
	l.extProvider = cap

	doc := &document.Document{
		Name:       "invoice.pdf",
		MimeType:   "application/pdf",
		Pages:      1,
		Text:       "Invoice #123\nTotal: $500\nDate: Jan 1",
		ShownPages: 1,
	}

	// First message with document.
	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "What's the total?", Document: doc})
	readOut(t, h)
	l.Wait()

	// Follow-up without document — should still have document in history.
	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "What's the date?"})
	readOut(t, h)
	l.Wait()

	// The second provider call should include the document in history.
	if len(cap.captured) < 3 {
		t.Fatalf("expected at least 3 provider calls (2 chat + extraction), got %d", len(cap.captured))
	}
	// Find the second chat call (index 2, since extraction is index 1).
	// Actually the order depends on timing. Check all captured calls for one that
	// has both the document and "What's the date?" question.
	found := false
	for _, msgs := range cap.captured {
		hasDoc := false
		hasFollowUp := false
		for _, m := range msgs {
			if m.Role == "system" && strings.Contains(m.Content, "Invoice #123") {
				hasDoc = true
			}
			if m.Role == "user" && m.Content == "What's the date?" {
				hasFollowUp = true
			}
		}
		if hasDoc && hasFollowUp {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected follow-up call to include document from history")
	}
}
```

- [ ] **Step 3: Write test for summarization compaction of document text**

```go
func TestSummarizationCompactsDocumentText(t *testing.T) {
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create loop with summarize=true and limit=4.
	l := NewLoop(h, &mockProvider{name: "test"}, db, 4, 8000, true, false, "", nil)

	// Fill history: a document system message + 3 regular messages = 4 total.
	longDocText := "--- Document: invoice.pdf (2 pages) ---\n" + strings.Repeat("Invoice line item. ", 500) + "\n--- End of document ---"
	_ = db.Append(1, provider.Message{Role: "system", Content: longDocText}, 50)
	_ = db.Append(1, provider.Message{Role: "user", Content: "What's the total?"}, 50)
	_ = db.Append(1, provider.Message{Role: "assistant", Content: "The total is $500."}, 50)
	_ = db.Append(1, provider.Message{Role: "user", Content: "msg-3"}, 50)

	// Provider: 1st = chat response, 2nd = summarization call.
	// Capture what the summarizer receives.
	cap := &capturingProvider{responses: []string{"chat reply", "Summary: user discussed an invoice."}}
	l.provider = cap
	l.extProvider = cap

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hi"})
	readOut(t, h)
	l.Wait()

	// Find the summarization call — it should contain the compacted document header,
	// NOT the full document text.
	for _, msgs := range cap.captured {
		for _, m := range msgs {
			if strings.Contains(m.Content, "Invoice line item.") {
				t.Error("summarization call should not contain full document text, expected compacted header only")
			}
			if strings.Contains(m.Content, "--- Document: invoice.pdf") {
				// Good — the header was included as a placeholder.
			}
		}
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/agent/ -v -run TestHandleMessageWithDocument`
Run: `go test ./internal/agent/ -v -run TestDocumentPersistsForFollowUp`
Run: `go test ./internal/agent/ -v -run TestSummarizationCompactsDocumentText`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `go test ./...`
Expected: all pass

- [ ] **Step 6: Commit**

```bash
git add internal/agent/loop_test.go
git commit -m "test(agent): add document handling and follow-up persistence tests"
```

---

### Task 7: Update documentation

**Files:**
- Modify: `docs/configuration.md`

- [ ] **Step 1: Add `max_document_tokens` to field reference table**

In `docs/configuration.md`, add to the field reference JSON (after `history_token_budget` line):

```json
"max_document_tokens": 4000,
```

Add to the table (after the `history_token_budget` row):

```
| `max_document_tokens` | integer | No | `4000` | Max estimated tokens for extracted document text. PDFs exceeding this budget are truncated by page. ~4 characters per token. |
```

- [ ] **Step 2: Add document support note to Vision Support section**

In the Vision Support section, add a new subsection or row:

```
### Document Support

| Format | Supported | Notes |
|--------|:---------:|-------|
| PDF (text-based) | Yes | Pure-Go extraction, max 10 MB, text truncated to `max_document_tokens` |
| PDF (scanned/image) | No | Requires OCR — not supported yet |
```

- [ ] **Step 3: Commit**

```bash
git add docs/configuration.md
git commit -m "docs: add max_document_tokens and document support to configuration reference"
```

---

### Task 8: Final verification

- [ ] **Step 1: Run full test suite with race detector**

Run: `go test -race ./...`
Expected: all pass, no races

- [ ] **Step 2: Run linters**

Run: `go vet ./...`
Expected: no issues

- [ ] **Step 3: Build binary**

Run: `CGO_ENABLED=0 go build -o herald ./cmd/herald`
Expected: builds successfully

- [ ] **Step 4: Verify `go mod tidy`**

Run: `go mod tidy && git diff go.mod go.sum`
Expected: no unexpected changes

- [ ] **Step 5: Commit any remaining changes and verify clean state**

```bash
git status
```
Expected: clean working tree (all changes committed across tasks 1-7)
