package format

import (
	"strings"
	"testing"
)

func TestSplit_ShortMessage(t *testing.T) {
	got := Split("hello", 4096)
	if len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected single chunk, got %v", got)
	}
}

func TestSplit_EmptyString(t *testing.T) {
	got := Split("", 4096)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("expected single empty chunk, got %v", got)
	}
}

func TestSplit_ExactlyAtLimit(t *testing.T) {
	s := strings.Repeat("a", 100)
	got := Split(s, 100)
	if len(got) != 1 || got[0] != s {
		t.Errorf("expected single chunk, got %d chunks", len(got))
	}
}

func TestSplit_ParagraphBoundary(t *testing.T) {
	first := strings.Repeat("a", 40)
	second := strings.Repeat("b", 40)
	s := first + "\n\n" + second
	got := Split(s, 50)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	if got[0] != first+"\n\n" {
		t.Errorf("first chunk = %q, want %q", got[0], first+"\n\n")
	}
	if got[1] != second {
		t.Errorf("second chunk = %q, want %q", got[1], second)
	}
}

func TestSplit_NewlineBoundary(t *testing.T) {
	first := strings.Repeat("a", 40)
	second := strings.Repeat("b", 40)
	s := first + "\n" + second
	got := Split(s, 50)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	if got[0] != first+"\n" {
		t.Errorf("first chunk = %q, want %q", got[0], first+"\n")
	}
}

func TestSplit_SpaceBoundary(t *testing.T) {
	s := strings.Repeat("a", 20) + " " + strings.Repeat("b", 40)
	got := Split(s, 30)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	if got[0] != strings.Repeat("a", 20)+" " {
		t.Errorf("first chunk = %q, want %q", got[0], strings.Repeat("a", 20)+" ")
	}
}

func TestSplit_HardCut(t *testing.T) {
	s := strings.Repeat("x", 200)
	got := Split(s, 100)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(got))
	}
	if len(got[0]) != 100 {
		t.Errorf("first chunk len = %d, want 100", len(got[0]))
	}
	if len(got[1]) != 100 {
		t.Errorf("second chunk len = %d, want 100", len(got[1]))
	}
}

func TestSplit_BoldTagRepair(t *testing.T) {
	s := "<b>" + strings.Repeat("x", 100) + "</b>"
	got := Split(s, 60)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	if !strings.HasSuffix(got[0], "</b>") {
		t.Errorf("first chunk should end with </b>, got: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "<b>") {
		t.Errorf("second chunk should start with <b>, got: %q", got[1])
	}
}

func TestSplit_NestedTagRepair(t *testing.T) {
	s := "<pre><code>" + strings.Repeat("x", 100) + "</code></pre>"
	got := Split(s, 60)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	if !strings.HasSuffix(got[0], "</code></pre>") {
		t.Errorf("first chunk should close nested tags, got: %q", got[0])
	}
	if !strings.HasPrefix(got[1], "<pre><code>") {
		t.Errorf("second chunk should reopen nested tags, got: %q", got[1])
	}
}

func TestSplit_LargePreBlock(t *testing.T) {
	content := strings.Repeat("line of code\n", 400)
	s := "<pre><code>" + content + "</code></pre>"
	got := Split(s, 4096)
	if len(got) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(got))
	}
	for i, chunk := range got {
		if len(chunk) > 4096 {
			t.Errorf("chunk %d exceeds 4096: len=%d", i, len(chunk))
		}
		if !strings.Contains(chunk, "<pre><code>") {
			t.Errorf("chunk %d missing opening tags: %q", i, chunk[:min(50, len(chunk))])
		}
	}
}

func TestSplit_AllChunksWithinLimit(t *testing.T) {
	s := strings.Repeat("word ", 1000)
	maxLen := 200
	got := Split(s, maxLen)
	for i, chunk := range got {
		if len(chunk) > maxLen {
			t.Errorf("chunk %d exceeds maxLen: len=%d", i, len(chunk))
		}
	}
}

func TestSplit_PreservesContent(t *testing.T) {
	s := strings.Repeat("x", 300)
	got := Split(s, 100)
	joined := strings.Join(got, "")
	if joined != s {
		t.Errorf("content not preserved: got len %d, want len %d", len(joined), len(s))
	}
}

func TestSplit_DoesNotBreakEntity(t *testing.T) {
	s := strings.Repeat("x", 45) + "&amp;" + strings.Repeat("y", 45)
	got := Split(s, 48)
	for _, chunk := range got {
		for i := 0; i < len(chunk); i++ {
			if chunk[i] == '&' {
				semi := strings.IndexByte(chunk[i:], ';')
				if semi < 0 {
					t.Errorf("entity broken in chunk: %q", chunk)
				}
			}
		}
	}
}

func TestSplit_DoesNotBreakTag(t *testing.T) {
	s := strings.Repeat("x", 40) + `<a href="https://example.com">link</a>` + strings.Repeat("y", 40)
	got := Split(s, 50)
	for _, chunk := range got {
		// Every < should have a matching >
		opens := strings.Count(chunk, "<")
		closes := strings.Count(chunk, ">")
		if opens != closes {
			t.Errorf("unbalanced angle brackets in chunk: %q", chunk)
		}
	}
}

func TestSplit_DefaultMaxLen(t *testing.T) {
	s := strings.Repeat("a", 5000)
	got := Split(s, 0)
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks with default maxLen, got %d", len(got))
	}
	if len(got[0]) != 4096 {
		t.Errorf("first chunk len = %d, want 4096", len(got[0]))
	}
}

func TestSplit_MultipleChunks(t *testing.T) {
	s := strings.Repeat("a", 500)
	got := Split(s, 100)
	if len(got) != 5 {
		t.Errorf("expected 5 chunks, got %d", len(got))
	}
	for i, chunk := range got {
		if len(chunk) != 100 {
			t.Errorf("chunk %d len = %d, want 100", i, len(chunk))
		}
	}
}
