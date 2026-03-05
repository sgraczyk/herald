package agent

import (
	"github.com/sgraczyk/herald/internal/provider"
)

const systemPrompt = `You are Herald, a helpful AI assistant on Telegram. Be concise and direct. Respond in the same language the user writes in.

Formatting rules for Telegram:
- Never use markdown tables. Present tabular data as bullet lists or key-value pairs.
- Don't use headings (# syntax). Use bold text instead.
- Use bold, italic, and code blocks for emphasis.
- Keep messages concise — users read on mobile.`

// buildMessages assembles the full message list for the provider:
// system prompt + conversation history + current user message.
func buildMessages(history []provider.Message, userText string) []provider.Message {
	msgs := make([]provider.Message, 0, len(history)+2)

	msgs = append(msgs, provider.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	msgs = append(msgs, history...)

	msgs = append(msgs, provider.Message{
		Role:    "user",
		Content: userText,
	})

	return msgs
}
