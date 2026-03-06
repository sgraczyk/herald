package agent

import (
	"fmt"
	"strings"

	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
)

const systemPrompt = `You are Herald, a helpful AI assistant on Telegram. Be concise and direct. Respond in the same language the user writes in.

Formatting rules for Telegram:
- Never use markdown tables. Present tabular data as bullet lists or key-value pairs.
- Don't use headings (# syntax). Use bold text instead.
- Use bold, italic, and code blocks for emphasis.
- Keep messages concise — users read on mobile.`

// buildMessages assembles the full message list for the provider:
// system prompt (with memories) + conversation history + current user message.
func buildMessages(history []provider.Message, memories []store.Memory, userText string) []provider.Message {
	msgs := make([]provider.Message, 0, len(history)+2)

	prompt := systemPrompt
	if len(memories) > 0 {
		var b strings.Builder
		b.WriteString(prompt)
		b.WriteString("\n\nYou know the following about the user:")
		for _, m := range memories {
			fmt.Fprintf(&b, "\n- %s (%s)", m.Fact, m.Source)
		}
		prompt = b.String()
	}

	msgs = append(msgs, provider.Message{
		Role:    "system",
		Content: prompt,
	})

	msgs = append(msgs, history...)

	msgs = append(msgs, provider.Message{
		Role:    "user",
		Content: userText,
	})

	return msgs
}
