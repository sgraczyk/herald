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

// maxContextMemories limits how many memories are injected into the system
// prompt. All explicit memories are always included; remaining slots are
// filled with the most recent auto-extracted memories.
const maxContextMemories = 50

// buildMessages assembles the full message list for the provider:
// system prompt (with memories) + conversation history + current user message.
func buildMessages(history []provider.Message, memories []store.Memory, userText string) []provider.Message {
	msgs := make([]provider.Message, 0, len(history)+2)

	selected := selectMemories(memories)
	prompt := systemPrompt
	if len(selected) > 0 {
		var b strings.Builder
		b.WriteString(prompt)
		b.WriteString("\n\nYou know the following about the user:")
		for _, m := range selected {
			fmt.Fprintf(&b, "\n- %s", m.Fact)
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

// selectMemories picks which memories to include in context.
// All explicit memories are always included. Remaining slots are filled
// with the most recent auto-extracted memories.
func selectMemories(memories []store.Memory) []store.Memory {
	if len(memories) <= maxContextMemories {
		return memories
	}

	var explicit []store.Memory
	var auto []store.Memory
	for _, m := range memories {
		if m.Source == "explicit" {
			explicit = append(explicit, m)
		} else {
			auto = append(auto, m)
		}
	}

	// Always include all explicit memories.
	selected := explicit
	remaining := maxContextMemories - len(explicit)
	if remaining > 0 && len(auto) > 0 {
		// Take the most recent auto memories (they're in chronological order).
		if len(auto) > remaining {
			auto = auto[len(auto)-remaining:]
		}
		selected = append(selected, auto...)
	}

	return selected
}
