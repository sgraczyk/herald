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

// textDensity returns the ratio of readable characters to total runes.
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
