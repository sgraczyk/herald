package tool

import (
	"fmt"
	"regexp"
	"strings"
)

// PromptCaller handles tool calling for providers that don't support native
// function calling. It injects tool definitions as XML into the system prompt
// and parses XML tool call blocks from LLM responses.
type PromptCaller struct {
	registry *Registry
}

// NewPromptCaller creates a PromptCaller backed by the given registry.
func NewPromptCaller(r *Registry) *PromptCaller {
	return &PromptCaller{registry: r}
}

// PromptFragment returns the XML tool definitions block to append to the
// system prompt. Returns "" if the registry has no tools.
func (pc *PromptCaller) PromptFragment() string {
	tools := pc.registry.All()
	if len(tools) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\nYou have access to the following tools:\n")

	for _, t := range tools {
		b.WriteString("\n<tool>\n")
		fmt.Fprintf(&b, "<name>%s</name>\n", t.Name())
		fmt.Fprintf(&b, "<description>%s</description>\n", t.Description())
		params := t.Parameters()
		if len(params) > 0 {
			b.WriteString("<parameters>\n")
			for _, p := range params {
				req := "false"
				if p.Required {
					req = "true"
				}
				fmt.Fprintf(&b, "<parameter name=%q type=%q required=%q>%s</parameter>\n", p.Name, p.Type, req, p.Description)
			}
			b.WriteString("</parameters>\n")
		}
		b.WriteString("</tool>\n")
	}

	b.WriteString("\nWhen you want to use a tool, respond with this XML block:\n")
	b.WriteString("<tool_use>\n<name>tool_name</name>\n<parameters>\n<param_name>value</param_name>\n</parameters>\n</tool_use>")

	return b.String()
}

// toolUseRe matches a <tool_use> block and captures the name and parameters section.
var toolUseRe = regexp.MustCompile(`(?s)<tool_use>\s*<name>(.*?)</name>\s*<parameters>(.*?)</parameters>\s*</tool_use>`)

// paramRe matches individual parameter elements within a <parameters> block.
// It captures the open tag name, content, and close tag name separately because
// Go's RE2 engine does not support backreferences.
var paramRe = regexp.MustCompile(`(?s)<(\w+)>(.*?)</(\w+)>`)

// Parse extracts a tool call from an LLM text response. Returns nil if the
// response contains no valid tool call for a registered tool.
func (pc *PromptCaller) Parse(response string) *ToolCall {
	matches := toolUseRe.FindStringSubmatch(response)
	if len(matches) < 3 {
		return nil
	}

	name := strings.TrimSpace(matches[1])
	if _, ok := pc.registry.Get(name); !ok {
		return nil
	}

	args := make(map[string]string)
	paramMatches := paramRe.FindAllStringSubmatch(matches[2], -1)
	for _, pm := range paramMatches {
		// pm[1] = open tag name, pm[2] = content, pm[3] = close tag name
		if len(pm) >= 4 && pm[1] == pm[3] {
			args[pm[1]] = strings.TrimSpace(pm[2])
		}
	}

	return &ToolCall{Name: name, Args: args}
}
