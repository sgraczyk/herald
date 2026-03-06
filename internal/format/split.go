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
// Prefers: paragraph break (\n\n) > newline > space > hard cut.
// Delimiters inside HTML tags are ignored.
func bestBoundary(html string, limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit >= len(html) {
		return len(html)
	}

	s := html[:limit]

	if i := lastSafeIndex(s, "\n\n"); i > 0 {
		return i + 2
	}
	if i := lastSafeIndex(s, "\n"); i > 0 {
		return i + 1
	}
	if i := lastSafeIndex(s, " "); i > 0 {
		return i + 1
	}

	// Hard cut: avoid landing inside a tag or entity.
	limit = avoidTagSplit(html, limit)
	limit = avoidEntitySplit(html, limit)
	return limit
}

// lastSafeIndex returns the last index of delim in s that is not inside an HTML tag.
// Returns -1 if not found.
func lastSafeIndex(s, delim string) int {
	inTag := false
	last := -1
	dlen := len(delim)
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			inTag = true
		} else if s[i] == '>' {
			inTag = false
		} else if !inTag && i+dlen <= len(s) && s[i:i+dlen] == delim {
			last = i
		}
	}
	return last
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
