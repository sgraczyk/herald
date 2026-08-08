package format

import (
	"strings"
	"testing"
)

func TestTelegramHTML_PlainText(t *testing.T) {
	got := TelegramHTML("Hello, world!")
	want := "Hello, world!"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_HTMLEscaping(t *testing.T) {
	got := TelegramHTML("Use <b> tags & \"quotes\"")
	want := "Use &lt;b&gt; tags &amp; &#34;quotes&#34;"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Bold(t *testing.T) {
	got := TelegramHTML("This is **bold** text")
	want := "This is <b>bold</b> text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_BoldNoSpaceAfterClosing(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bold colon no space",
			in:   "**Bianka:**Nasza koteczka",
			want: "<b>Bianka:</b>Nasza koteczka",
		},
		{
			name: "multiple bold colon no space",
			in:   "**Bianka:**kot\n**Homelab:**serwer\n**Gaming:**gra",
			want: "<b>Bianka:</b>kot\n<b>Homelab:</b>serwer\n<b>Gaming:</b>gra",
		},
		{
			name: "bold no space preserved in code block",
			in:   "```\n**not bold:**here\n```",
			want: "<pre><code>**not bold:**here\n</code></pre>",
		},
		{
			name: "bold no space preserved in inline code",
			in:   "use `**not bold:**here` ok",
			want: "use <code>**not bold:**here</code> ok",
		},
		{
			name: "normal bold still works",
			in:   "**Bianka:** kot",
			want: "<b>Bianka:</b> kot",
		},
		{
			name: "heading with unparsed bold no nesting",
			in:   "# **Title:**content",
			want: "<b>Title:content</b>",
		},
		{
			name: "lone double asterisks not converted",
			in:   "Use ** for emphasis **word:**next",
			want: "Use ** for emphasis <b>word:</b>next",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TelegramHTML(tt.in)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTelegramHTML_Italic(t *testing.T) {
	got := TelegramHTML("This is *italic* text")
	want := "This is <i>italic</i> text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Strikethrough(t *testing.T) {
	got := TelegramHTML("This is ~~deleted~~ text")
	want := "This is <s>deleted</s> text"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_InlineCode(t *testing.T) {
	got := TelegramHTML("Use `fmt.Println()` here")
	want := "Use <code>fmt.Println()</code> here"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_CodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	got := TelegramHTML(input)
	want := "<pre><code class=\"language-go\">fmt.Println(&#34;hello&#34;)\n</code></pre>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_CodeBlockNoLang(t *testing.T) {
	input := "```\nhello\n```"
	got := TelegramHTML(input)
	want := "<pre><code>hello\n</code></pre>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Link(t *testing.T) {
	got := TelegramHTML("[click here](https://example.com)")
	want := `<a href="https://example.com">click here</a>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Heading(t *testing.T) {
	got := TelegramHTML("# Hello World")
	want := "<b>Hello World</b>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Blockquote(t *testing.T) {
	got := TelegramHTML("> This is a quote")
	want := "<blockquote>This is a quote\n\n</blockquote>"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_UnorderedList(t *testing.T) {
	input := "- item one\n- item two\n- item three"
	got := TelegramHTML(input)
	want := "• item one\n• item two\n• item three"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_OrderedList(t *testing.T) {
	input := "1. first\n2. second\n3. third"
	got := TelegramHTML(input)
	want := "1. first\n2. second\n3. third"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_ThematicBreak(t *testing.T) {
	input := "before\n\n---\n\nafter"
	got := TelegramHTML(input)
	want := "before\n\n———\n\nafter"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_TwoColumnTable(t *testing.T) {
	input := `| Key | Value |
|-----|-------|
| **Temperature** | 15°C |
| **Sky** | Sunny |`
	got := TelegramHTML(input)
	// Two-column table should render as key-value bullet list.
	// Bold markdown in cells is preserved inside the key's <b> wrapper.
	if !strings.Contains(got, "Temperature") || !strings.Contains(got, "15°C") || !strings.Contains(got, "•") {
		t.Errorf("expected key-value format with temperature, got:\n%s", got)
	}
	if !strings.Contains(got, "Sky") || !strings.Contains(got, "Sunny") {
		t.Errorf("expected key-value format with sky, got:\n%s", got)
	}
}

func TestTelegramHTML_WideTable(t *testing.T) {
	input := `| Name | Age | City |
|------|-----|------|
| Alice | 30 | NYC |
| Bob | 25 | LA |`
	got := TelegramHTML(input)
	// Wide table should render as pre-formatted block
	if !strings.Contains(got, "<pre>") {
		t.Errorf("expected pre block for wide table, got:\n%s", got)
	}
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "Bob") {
		t.Errorf("expected table data, got:\n%s", got)
	}
}

func TestTelegramHTML_Image(t *testing.T) {
	got := TelegramHTML("![alt text](https://example.com/img.png)")
	want := `<a href="https://example.com/img.png">alt text</a>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_Empty(t *testing.T) {
	got := TelegramHTML("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestTelegramHTML_MixedContent(t *testing.T) {
	input := `# Weather Report

**Today's forecast:**

- Temperature: *15°C*
- Wind: 10 km/h

> Stay warm!`
	got := TelegramHTML(input)
	if !strings.Contains(got, "<b>Weather Report</b>") {
		t.Errorf("expected heading as bold, got:\n%s", got)
	}
	if !strings.Contains(got, "<b>Today&#39;s forecast:</b>") {
		t.Errorf("expected bold text, got:\n%s", got)
	}
	if !strings.Contains(got, "• Temperature: <i>15°C</i>") {
		t.Errorf("expected list with italic, got:\n%s", got)
	}
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("expected blockquote, got:\n%s", got)
	}
}

func TestTelegramHTML_NestedUnorderedList(t *testing.T) {
	input := "- top\n  - nested"
	got := TelegramHTML(input)
	want := "• top\n\u00a0\u00a0◦ nested"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_DeeplyNestedUnorderedList(t *testing.T) {
	input := "- top\n  - mid\n    - deep"
	got := TelegramHTML(input)
	want := "• top\n\u00a0\u00a0◦ mid\n\u00a0\u00a0\u00a0\u00a0▪ deep"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_NestedOrderedList(t *testing.T) {
	input := "1. top\n   1. nested\n   2. also nested"
	got := TelegramHTML(input)
	want := "1. top\n\u00a0\u00a01. nested\n\u00a0\u00a02. also nested"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_MixedNestedList(t *testing.T) {
	input := "1. top\n   - nested bullet"
	got := TelegramHTML(input)
	want := "1. top\n\u00a0\u00a0◦ nested bullet"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTelegramHTML_NestedListDepthCap(t *testing.T) {
	input := "- a\n  - b\n    - c\n      - d"
	got := TelegramHTML(input)
	// 4th level should render same as 3rd level (depth capped at 2)
	if !strings.Contains(got, "\u00a0\u00a0\u00a0\u00a0▪ c") {
		t.Errorf("expected depth-2 bullet for 'c', got:\n%s", got)
	}
	if !strings.Contains(got, "\u00a0\u00a0\u00a0\u00a0▪ d") {
		t.Errorf("expected depth-2 bullet for 'd' (capped), got:\n%s", got)
	}
}

func TestTelegramHTML_TopLevelLooseList(t *testing.T) {
	// Top-level loose list should still get \n\n paragraph spacing
	input := "- item one\n\n- item two"
	got := TelegramHTML(input)
	if !strings.Contains(got, "• item one") {
		t.Errorf("expected bullet for item one, got:\n%s", got)
	}
	if !strings.Contains(got, "• item two") {
		t.Errorf("expected bullet for item two, got:\n%s", got)
	}
}

func TestTelegramHTML_LooseNestedList(t *testing.T) {
	// Loose list: blank line between items makes goldmark wrap content in Paragraph nodes
	input := "- top\n\n  - nested\n\n  - also nested"
	got := TelegramHTML(input)
	// Nested items should NOT get double-spaced \n\n between them
	if strings.Contains(got, "nested\n\n") {
		t.Errorf("nested items should not be double-spaced, got:\n%s", got)
	}
	if !strings.Contains(got, "◦ nested") {
		t.Errorf("expected nested bullet, got:\n%s", got)
	}
}

func BenchmarkTelegramHTML(b *testing.B) {
	input := `# Weather Report

**Today's forecast:**

- Temperature: *15°C*
- Wind: 10 km/h

> Stay warm!

| Key | Value |
|-----|-------|
| **Humidity** | 60% |

Visit [example](https://example.com) for details.`

	b.ReportAllocs()
	for b.Loop() {
		TelegramHTML(input)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<b>bold</b>", "bold"},
		{"plain text", "plain text"},
		{"<a href=\"x\">link</a>", "link"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripHTMLTags(tt.input)
		if got != tt.want {
			t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

