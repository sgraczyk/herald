// Package format converts markdown to Telegram-compatible HTML and handles
// message splitting for Telegram length limits.
package format

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// md is a package-level goldmark instance shared across all TelegramHTML calls.
// This is safe because goldmark.Markdown is stateless and concurrent-safe,
// and telegramRenderer is an empty struct with no mutable fields.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRenderer(
		renderer.NewRenderer(
			renderer.WithNodeRenderers(
				util.Prioritized(&telegramRenderer{}, 100),
			),
		),
	),
)

// TelegramHTML converts standard markdown to Telegram-compatible HTML.
// Unsupported features (tables, headings) are converted to readable alternatives.
func TelegramHTML(markdown string) string {
	source := []byte(markdown)

	var buf bytes.Buffer
	if err := md.Convert(source, &buf); err != nil {
		return html.EscapeString(markdown)
	}

	result := strings.TrimSpace(buf.String())
	return fixUnparsedBold(result)
}

type telegramRenderer struct{}

func (r *telegramRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	// Block nodes
	reg.Register(ast.KindDocument, r.renderDocument)
	reg.Register(ast.KindParagraph, r.renderParagraph)
	reg.Register(ast.KindHeading, r.renderHeading)
	reg.Register(ast.KindBlockquote, r.renderBlockquote)
	reg.Register(ast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(ast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(ast.KindList, r.renderList)
	reg.Register(ast.KindListItem, r.renderListItem)
	reg.Register(ast.KindThematicBreak, r.renderThematicBreak)
	reg.Register(ast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(ast.KindTextBlock, r.renderTextBlock)

	// Inline nodes
	reg.Register(ast.KindText, r.renderText)
	reg.Register(ast.KindString, r.renderString)
	reg.Register(ast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(ast.KindEmphasis, r.renderEmphasis)
	reg.Register(ast.KindLink, r.renderLink)
	reg.Register(ast.KindAutoLink, r.renderAutoLink)
	reg.Register(ast.KindImage, r.renderImage)
	reg.Register(ast.KindRawHTML, r.renderRawHTML)

	// GFM extension nodes
	reg.Register(east.KindTable, r.renderTable)
	reg.Register(east.KindTableHeader, r.renderTableHeader)
	reg.Register(east.KindTableRow, r.renderTableRow)
	reg.Register(east.KindTableCell, r.renderTableCell)
	reg.Register(east.KindStrikethrough, r.renderStrikethrough)
}

func (r *telegramRenderer) renderDocument(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderParagraph(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		// Inside nested list items, use single newline to avoid awkward double-spacing.
		if node.Parent() != nil && node.Parent().Kind() == ast.KindListItem && listDepth(node.Parent()) > 0 {
			_, _ = w.WriteString("\n")
		} else {
			_, _ = w.WriteString("\n\n")
		}
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderHeading(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<b>")
	} else {
		_, _ = w.WriteString("</b>\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderBlockquote(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<blockquote>")
	} else {
		_, _ = w.WriteString("</blockquote>\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<pre>")
		r.writeLines(w, source, node)
		_, _ = w.WriteString("</pre>\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		n := node.(*ast.FencedCodeBlock)
		lang := n.Language(source)
		if lang != nil {
			_, _ = fmt.Fprintf(w, `<pre><code class="language-%s">`, html.EscapeString(string(lang)))
		} else {
			_, _ = w.WriteString("<pre><code>")
		}
		r.writeLines(w, source, node)
		_, _ = w.WriteString("</code></pre>\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderList(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

// listDepth returns the nesting depth of a list item by counting ancestor
// List nodes. Returns 0 for top-level, capped at 2 (three levels).
func listDepth(node ast.Node) int {
	depth := 0
	for p := node.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == ast.KindList {
			depth++
		}
	}
	// depth counts the List ancestors; the item's own parent List = 1,
	// so subtract 1 to get nesting level (0-based).
	depth--
	if depth < 0 {
		depth = 0
	}
	if depth > 2 {
		depth = 2
	}
	return depth
}

// bullets maps nesting depth to bullet character for unordered lists.
var bullets = [3]string{"• ", "◦ ", "▪ "}

func (r *telegramRenderer) renderListItem(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		depth := listDepth(node)
		indent := strings.Repeat("\u00a0\u00a0", depth)

		list := node.Parent().(*ast.List)
		if list.IsOrdered() {
			index := 1
			if list.Start > 0 {
				index = list.Start
			}
			for c := node.Parent().FirstChild(); c != nil && c != node; c = c.NextSibling() {
				index++
			}
			_, _ = fmt.Fprintf(w, "%s%d. ", indent, index)
		} else {
			_, _ = w.WriteString(indent + bullets[depth])
		}
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderThematicBreak(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("———\n\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		r.writeLines(w, source, node)
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderTextBlock(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

// Inline renderers

func (r *telegramRenderer) renderText(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Text)
	segment := n.Segment
	value := segment.Value(source)
	if n.IsRaw() {
		_, _ = w.Write(value)
	} else {
		_, _ = w.WriteString(html.EscapeString(string(value)))
	}
	if n.SoftLineBreak() {
		_, _ = w.WriteString("\n")
	}
	if n.HardLineBreak() {
		_, _ = w.WriteString("\n")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderString(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.String)
	if n.IsCode() {
		_, _ = w.Write(n.Value)
	} else {
		_, _ = w.WriteString(html.EscapeString(string(n.Value)))
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderCodeSpan(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<code>")
	} else {
		_, _ = w.WriteString("</code>")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderEmphasis(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Emphasis)
	tag := "i"
	if n.Level == 2 {
		tag = "b"
	}
	if entering {
		_, _ = fmt.Fprintf(w, "<%s>", tag)
	} else {
		_, _ = fmt.Fprintf(w, "</%s>", tag)
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderLink(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.Link)
	if entering {
		_, _ = fmt.Fprintf(w, `<a href="%s">`, html.EscapeString(string(n.Destination)))
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderAutoLink(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	n := node.(*ast.AutoLink)
	if !entering {
		return ast.WalkContinue, nil
	}
	url := html.EscapeString(string(n.URL(source)))
	label := html.EscapeString(string(n.Label(source)))
	_, _ = fmt.Fprintf(w, `<a href="%s">%s</a>`, url, label)
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderImage(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	// Telegram doesn't support inline images in text messages.
	// Render as a link instead.
	n := node.(*ast.Image)
	if entering {
		_, _ = fmt.Fprintf(w, `<a href="%s">`, html.EscapeString(string(n.Destination)))
	} else {
		_, _ = w.WriteString("</a>")
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderRawHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.RawHTML)
	for i := 0; i < n.Segments.Len(); i++ {
		segment := n.Segments.At(i)
		_, _ = w.WriteString(html.EscapeString(string(segment.Value(source))))
	}
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderStrikethrough(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<s>")
	} else {
		_, _ = w.WriteString("</s>")
	}
	return ast.WalkContinue, nil
}

// Table rendering: convert to readable text format.
// Tables are rendered as bullet lists for 2-column tables (key-value),
// or as pre-formatted blocks for wider tables.

func (r *telegramRenderer) renderTable(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("\n")
		return ast.WalkContinue, nil
	}

	// Collect table data by walking children manually.
	table := node.(*east.Table)
	headers, rows := r.collectTableData(source, table)

	r.writeTable(w, headers, rows)

	return ast.WalkSkipChildren, nil
}

// Stub renderers for table sub-nodes (handled by renderTable via WalkSkipChildren).
func (r *telegramRenderer) renderTableHeader(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderTableRow(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) renderTableCell(_ util.BufWriter, _ []byte, _ ast.Node, _ bool) (ast.WalkStatus, error) {
	return ast.WalkContinue, nil
}

func (r *telegramRenderer) collectTableData(source []byte, table *east.Table) (headers []string, rows [][]string) {
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		var cells []string
		for cell := child.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, r.collectInlineText(source, cell))
		}
		switch child.Kind() {
		case east.KindTableHeader:
			headers = cells
		case east.KindTableRow:
			rows = append(rows, cells)
		}
	}
	return headers, rows
}

func (r *telegramRenderer) collectInlineText(source []byte, node ast.Node) string {
	var buf bytes.Buffer
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		switch v := n.(type) {
		case *ast.Text:
			if entering {
				buf.WriteString(html.EscapeString(string(v.Segment.Value(source))))
			}
		case *ast.String:
			if entering {
				buf.WriteString(html.EscapeString(string(v.Value)))
			}
		case *ast.CodeSpan:
			if entering {
				buf.WriteString("<code>")
			} else {
				buf.WriteString("</code>")
			}
		case *ast.Emphasis:
			tag := "i"
			if v.Level == 2 {
				tag = "b"
			}
			if entering {
				fmt.Fprintf(&buf, "<%s>", tag)
			} else {
				fmt.Fprintf(&buf, "</%s>", tag)
			}
		case *ast.Link:
			if entering {
				fmt.Fprintf(&buf, `<a href="%s">`, html.EscapeString(string(v.Destination)))
			} else {
				buf.WriteString("</a>")
			}
		case *east.Strikethrough:
			if entering {
				buf.WriteString("<s>")
			} else {
				buf.WriteString("</s>")
			}
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

func (r *telegramRenderer) writeTable(w util.BufWriter, headers []string, rows [][]string) {
	if len(headers) == 2 && allRowsHaveNCols(rows, 2) {
		r.writeKeyValueTable(w, headers, rows)
	} else {
		r.writePreTable(w, headers, rows)
	}
}

func (r *telegramRenderer) writeKeyValueTable(w util.BufWriter, headers []string, rows [][]string) {
	// If headers have content, show them as a bold title line.
	if headers[0] != "" || headers[1] != "" {
		_, _ = fmt.Fprintf(w, "<b>%s</b> — <b>%s</b>\n", headers[0], headers[1])
	}
	for _, row := range rows {
		key := row[0]
		val := row[1]
		if key != "" {
			_, _ = fmt.Fprintf(w, "• <b>%s</b>: %s\n", key, val)
		} else {
			_, _ = fmt.Fprintf(w, "• %s\n", val)
		}
	}
}

func (r *telegramRenderer) writePreTable(w util.BufWriter, headers []string, rows [][]string) {
	// Compute column widths from plain text (strip HTML tags for width calc).
	allRows := append([][]string{headers}, rows...)
	ncols := 0
	for _, row := range allRows {
		if len(row) > ncols {
			ncols = len(row)
		}
	}

	widths := make([]int, ncols)
	for _, row := range allRows {
		for i, cell := range row {
			plain := stripHTMLTags(cell)
			if len(plain) > widths[i] {
				widths[i] = len(plain)
			}
		}
	}

	_, _ = w.WriteString("<pre>")
	for ri, row := range allRows {
		for i := 0; i < ncols; i++ {
			cell := ""
			if i < len(row) {
				cell = stripHTMLTags(row[i])
			}
			if i > 0 {
				_, _ = w.WriteString("  ")
			}
			_, _ = w.WriteString(html.EscapeString(cell))
			if i < ncols-1 {
				padding := widths[i] - len(cell)
				for j := 0; j < padding; j++ {
					_ = w.WriteByte(' ')
				}
			}
		}
		_, _ = w.WriteString("\n")
		// Separator after header
		if ri == 0 && len(rows) > 0 {
			for i := 0; i < ncols; i++ {
				if i > 0 {
					_, _ = w.WriteString("  ")
				}
				_, _ = w.WriteString(strings.Repeat("─", widths[i]))
			}
			_, _ = w.WriteString("\n")
		}
	}
	_, _ = w.WriteString("</pre>")
}

func (r *telegramRenderer) writeLines(w util.BufWriter, source []byte, node ast.Node) {
	l := node.Lines().Len()
	for i := 0; i < l; i++ {
		line := node.Lines().At(i)
		value := line.Value(source)
		_, _ = w.WriteString(html.EscapeString(string(value)))
	}
}

func allRowsHaveNCols(rows [][]string, n int) bool {
	for _, row := range rows {
		if len(row) != n {
			return false
		}
	}
	return true
}

func stripHTMLTags(s string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// unparsedBoldRe matches **...** that goldmark left unconverted.
// The first character after ** must not be whitespace or * to avoid
// matching lone ** markers that goldmark intentionally left as literal text.
var unparsedBoldRe = regexp.MustCompile(`\*\*([^\s*].*?)\*\*`)

// fixUnparsedBold converts remaining **...** patterns to <b>...</b>.
// CommonMark does not treat ** as a closing delimiter when it follows
// punctuation and is immediately followed by a Unicode letter (e.g.,
// **word:**nextword). This post-processing step catches those cases.
// Content inside <pre>, <code>, and <b> tags is left unchanged.
// Inside <b> blocks (e.g., headings), ** markers are stripped rather than
// converted, since the content is already bold.
func fixUnparsedBold(s string) string {
	if !strings.Contains(s, "**") {
		return s
	}

	var buf strings.Builder
	buf.Grow(len(s))

	for len(s) > 0 {
		preIdx := strings.Index(s, "<pre")
		codeIdx := strings.Index(s, "<code")
		boldIdx := strings.Index(s, "<b>")

		tagIdx := -1
		var closeTag string
		stripOnly := false
		switch {
		case preIdx >= 0 && (codeIdx < 0 || preIdx <= codeIdx) && (boldIdx < 0 || preIdx <= boldIdx):
			tagIdx = preIdx
			closeTag = "</pre>"
		case codeIdx >= 0 && (boldIdx < 0 || codeIdx <= boldIdx):
			tagIdx = codeIdx
			closeTag = "</code>"
		case boldIdx >= 0:
			tagIdx = boldIdx
			closeTag = "</b>"
			stripOnly = true
		}

		if tagIdx < 0 {
			buf.WriteString(unparsedBoldRe.ReplaceAllString(s, "<b>$1</b>"))
			break
		}

		// Process text before the tag.
		buf.WriteString(unparsedBoldRe.ReplaceAllString(s[:tagIdx], "<b>$1</b>"))
		s = s[tagIdx:]

		// Find the matching close tag.
		closeIdx := strings.Index(s, closeTag)
		if closeIdx < 0 {
			buf.WriteString(s)
			break
		}
		closeIdx += len(closeTag)

		if stripOnly {
			// Inside <b> blocks (e.g., headings), strip ** markers
			// instead of converting to nested <b> tags.
			buf.WriteString(strings.ReplaceAll(s[:closeIdx], "**", ""))
		} else {
			buf.WriteString(s[:closeIdx])
		}
		s = s[closeIdx:]
	}

	return buf.String()
}
