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

const charsPerToken = 4

const minTextDensity = 0.8

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
	defer func() {
		if rv := recover(); rv != nil {
			doc = nil
			err = fmt.Errorf("%w: %v", ErrMalformed, rv)
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
			continue
		}

		pageText = strings.TrimSpace(pageText)
		if pageText == "" {
			continue
		}

		if shownPages == 0 && textDensity(pageText) < minTextDensity {
			return nil, ErrGarbled
		}

		exceeds := text.Len()+len(pageText)+1 > maxChars
		if shownPages > 0 && exceeds {
			truncated = true
			break
		}

		if text.Len() > 0 {
			text.WriteString("\n")
		}
		text.WriteString(pageText)
		shownPages++

		// Mark truncated if even the first page exceeds the budget.
		if shownPages == 1 && exceeds {
			truncated = true
			break
		}
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

func extractPageText(page pdf.Page) (text string, err error) {
	defer func() {
		if rv := recover(); rv != nil {
			text = ""
			err = fmt.Errorf("page extraction panic: %v", rv)
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

func isEncryptedError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "encrypted") || strings.Contains(msg, "password")
}
