package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// claudeResponse is the JSON output from `claude -p --output-format json`.
type claudeResponse struct {
	Result string `json:"result"`
}

// Claude executes the Claude CLI in pipe mode.
type Claude struct {
	timeout time.Duration
}

// NewClaude creates a new Claude CLI provider.
func NewClaude() *Claude {
	return &Claude{
		timeout: 30 * time.Second,
	}
}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) Chat(ctx context.Context, messages []Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	input := buildClaudeInput(messages)

	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "json")
	cmd.Stdin = strings.NewReader(input)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("execute claude: %w", err)
	}

	var resp claudeResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return "", fmt.Errorf("parse claude response: %w", err)
	}

	if resp.Result == "" {
		return "", fmt.Errorf("empty response from claude")
	}

	return resp.Result, nil
}

// buildClaudeInput assembles the conversation into a single prompt string.
// System messages become context, history provides conversation continuity,
// and the last user message is the actual question.
func buildClaudeInput(messages []Message) string {
	var b strings.Builder

	// Collect system messages as context.
	var history []Message
	for _, m := range messages {
		if m.Role == "system" {
			b.WriteString(m.Content)
			b.WriteString("\n\n")
		} else {
			history = append(history, m)
		}
	}

	// Add conversation history.
	if len(history) > 1 {
		b.WriteString("Conversation so far:\n")
		for _, m := range history[:len(history)-1] {
			b.WriteString(fmt.Sprintf("[%s]: %s\n", m.Role, m.Content))
		}
		b.WriteString("\n")
	}

	// Add the latest user message.
	if len(history) > 0 {
		last := history[len(history)-1]
		b.WriteString(last.Content)
	}

	return b.String()
}
