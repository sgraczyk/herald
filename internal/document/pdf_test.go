package document

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func TestPDFExtractorBasic(t *testing.T) {
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
	longText := strings.Repeat("This is a test sentence. ", 100)
	data := minimalPDF(longText)
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(10) // Very small budget: ~40 chars
	doc, err := ext.Extract(r, int64(len(data)), "long.pdf")
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// First page is always included, but Truncated should be true
	// since the text exceeds the budget.
	if !doc.Truncated {
		t.Error("Truncated = false, want true for small token budget")
	}
}

func TestPDFExtractorEmptyPDF(t *testing.T) {
	data := minimalPDF("")
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(4000)
	_, err := ext.Extract(r, int64(len(data)), "empty.pdf")
	if err != ErrNoText {
		t.Errorf("Extract() error = %v, want ErrNoText", err)
	}
}

func TestPDFExtractorMalformedPDF(t *testing.T) {
	data := []byte("this is not a PDF at all")
	r := bytes.NewReader(data)

	ext := NewPDFExtractor(4000)
	_, err := ext.Extract(r, int64(len(data)), "bad.pdf")
	if err == nil {
		t.Error("Extract() expected error for malformed PDF, got nil")
	}
}

func TestPDFExtractorEncrypted(t *testing.T) {
	if !isEncryptedError(fmt.Errorf("file is encrypted")) {
		t.Error("isEncryptedError should detect 'encrypted' in error message")
	}
	if !isEncryptedError(fmt.Errorf("password required")) {
		t.Error("isEncryptedError should detect 'password' in error message")
	}
	if isEncryptedError(fmt.Errorf("invalid format")) {
		t.Error("isEncryptedError should not match unrelated errors")
	}
}

func TestTextDensity(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantLow bool
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
func minimalPDF(text string) []byte {
	var b bytes.Buffer

	b.WriteString("%PDF-1.0\n")

	obj1Offset := b.Len()
	b.WriteString("1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n")

	obj2Offset := b.Len()
	b.WriteString("2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n")

	obj3Offset := b.Len()
	b.WriteString("3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n")

	stream := "BT /F1 12 Tf 72 720 Td (" + text + ") Tj ET"
	obj4Offset := b.Len()
	b.WriteString("4 0 obj<</Length " + strconv.Itoa(len(stream)) + ">>stream\n")
	b.WriteString(stream)
	b.WriteString("\nendstream endobj\n")

	obj5Offset := b.Len()
	b.WriteString("5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n")

	xrefOffset := b.Len()
	b.WriteString("xref\n0 6\n")
	b.WriteString("0000000000 65535 f \n")
	b.WriteString(padOffset(obj1Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj2Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj3Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj4Offset) + " 00000 n \n")
	b.WriteString(padOffset(obj5Offset) + " 00000 n \n")

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
