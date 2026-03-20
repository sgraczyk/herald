package format

import "strings"

// htmlTag tracks an open HTML tag for repair across split boundaries.
type htmlTag struct {
	name string // tag name, e.g. "pre", "a"
	open string // full opening tag, e.g. `<a href="...">`
}

// Split divides an HTML string into chunks of at most maxLen bytes.
// Tags are closed at split boundaries and reopened in the next chunk.
// Empty chunks are filtered out. Always returns at least one element.
func Split(html string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = 4096
	}
	if len(html) <= maxLen {
		return []string{html}
	}

	var (
		chunks []string
		carry  []htmlTag
	)

	for len(html) > 0 {
		prefix := renderOpenTags(carry)

		if len(prefix)+len(html) <= maxLen {
			chunks = appendNonBlank(chunks, prefix+html)
			break
		}

		budget := maxLen - len(prefix)
		if budget < 1 {
			budget = 1
		}
		if budget > len(html) {
			budget = len(html)
		}

		splitAt := bestBoundary(html, budget)
		if splitAt < 1 {
			splitAt = 1
		}

		tags := tagStack(carry, html[:splitAt])
		suffix := renderCloseTags(tags)

		// Shrink until the assembled chunk fits within maxLen.
		for total := len(prefix) + splitAt + len(suffix); total > maxLen && splitAt > 1; {
			over := total - maxLen
			if over < 1 {
				over = 1
			}
			nb := splitAt - over
			if nb < 1 {
				nb = 1
			}
			splitAt = bestBoundary(html, nb)
			if splitAt < 1 {
				splitAt = 1
			}
			tags = tagStack(carry, html[:splitAt])
			suffix = renderCloseTags(tags)
			total = len(prefix) + splitAt + len(suffix)
		}

		chunks = appendNonBlank(chunks, prefix+html[:splitAt]+suffix)
		html = html[splitAt:]
		carry = tags
	}

	if len(chunks) == 0 {
		return []string{html}
	}
	return chunks
}

// bestBoundary finds the best split position at or before limit.
// Prefers boundaries outside <pre> blocks over those inside.
// Priority: outside-pre \n\n > outside-pre \n > inside-pre \n >
// outside-pre space > inside-pre space > hard cut.
// Delimiters inside HTML tags (between < and >) are always ignored.
func bestBoundary(html string, limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit >= len(html) {
		return len(html)
	}

	s := html[:limit]

	// Single pass: track HTML tag boundaries and <pre> depth.
	var (
		inTag   bool
		preDep  int
		outsideParaBreak = -1 // priority 1
		outsideNewline   = -1 // priority 2
		insideNewline    = -1 // priority 3
		outsideSpace     = -1 // priority 4
		insideSpace      = -1 // priority 5
	)

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '<' {
			inTag = true
			// Detect <pre> and </pre> for depth tracking.
			if i+4 < len(s) && s[i+1:i+5] == "pre>" {
				preDep++
			} else if i+5 < len(s) && s[i+1:i+6] == "/pre>" {
				preDep--
				if preDep < 0 {
					preDep = 0
				}
			}
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}

		if inTag {
			continue
		}

		inPre := preDep > 0

		if ch == '\n' && i+1 < len(s) && s[i+1] == '\n' {
			if !inPre {
				outsideParaBreak = i
			}
			// \n\n inside pre counts as insideNewline (priority 3)
			if inPre {
				insideNewline = i
			}
			continue
		}

		if ch == '\n' {
			if inPre {
				insideNewline = i
			} else {
				outsideNewline = i
			}
			continue
		}

		if ch == ' ' {
			if inPre {
				insideSpace = i
			} else {
				outsideSpace = i
			}
		}
	}

	// Return best match by priority.
	if outsideParaBreak > 0 {
		return outsideParaBreak + 2
	}
	if outsideNewline > 0 {
		return outsideNewline + 1
	}
	if insideNewline > 0 {
		return insideNewline + 1
	}
	if outsideSpace > 0 {
		return outsideSpace + 1
	}
	if insideSpace > 0 {
		return insideSpace + 1
	}

	// Hard cut: avoid landing inside a tag or entity.
	limit = avoidTagSplit(html, limit)
	limit = avoidEntitySplit(html, limit)
	return limit
}

// avoidTagSplit adjusts limit to avoid splitting inside an HTML tag.
func avoidTagSplit(html string, limit int) int {
	for i := limit - 1; i >= 0 && i > limit-500; i-- {
		if html[i] == '>' {
			return limit
		}
		if html[i] == '<' {
			return i
		}
	}
	return limit
}

// avoidEntitySplit adjusts limit to avoid splitting inside an HTML entity.
func avoidEntitySplit(html string, limit int) int {
	for i := limit - 1; i >= 0 && i > limit-12; i-- {
		switch html[i] {
		case ';', ' ', '\n', '<':
			return limit
		case '&':
			return i
		}
	}
	return limit
}

// tagStack computes the open-tag stack after applying opens/closes in content,
// starting from an initial set of carried-over open tags.
func tagStack(initial []htmlTag, content string) []htmlTag {
	stack := make([]htmlTag, len(initial))
	copy(stack, initial)

	for i := 0; i < len(content); {
		if content[i] != '<' {
			i++
			continue
		}
		end := strings.IndexByte(content[i:], '>')
		if end < 0 {
			break
		}
		raw := content[i : i+end+1]
		i += end + 1

		if len(raw) < 3 {
			continue
		}

		if raw[1] == '/' {
			name := extractName(raw[2:])
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k].name == name {
					stack = append(stack[:k], stack[k+1:]...)
					break
				}
			}
		} else {
			name := extractName(raw[1:])
			if tgTags[name] {
				stack = append(stack, htmlTag{name: name, open: raw})
			}
		}
	}
	return stack
}

// extractName extracts the tag name from text following '<' or '</'.
func extractName(s string) string {
	s = strings.TrimRight(s, ">")
	if i := strings.IndexAny(s, " \t\n/>"); i > 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

// tgTags lists HTML tags supported by Telegram that need tracking.
var tgTags = map[string]bool{
	"b": true, "i": true, "u": true, "s": true,
	"code": true, "pre": true, "a": true, "blockquote": true,
}

func renderOpenTags(tags []htmlTag) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range tags {
		b.WriteString(t.open)
	}
	return b.String()
}

func renderCloseTags(tags []htmlTag) string {
	if len(tags) == 0 {
		return ""
	}
	var b strings.Builder
	for i := len(tags) - 1; i >= 0; i-- {
		b.WriteString("</")
		b.WriteString(tags[i].name)
		b.WriteByte('>')
	}
	return b.String()
}

func appendNonBlank(chunks []string, chunk string) []string {
	if strings.TrimSpace(chunk) == "" {
		return chunks
	}
	return append(chunks, chunk)
}
