package provider

import (
	"context"
	"encoding/json"
	"errors"
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

// NewClaude creates a new Claude CLI provider with the given timeout.
func NewClaude(timeout time.Duration) *Claude {
	return &Claude{
		timeout: timeout,
	}
}

func (c *Claude) Name() string { return "claude" }

func (c *Claude) Chat(ctx context.Context, messages []Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	systemPrompt, userInput := buildClaudeInput(messages)

	args := []string{"-p", "--output-format", "json"}
	if systemPrompt != "" {
		args = append(args, "--system-prompt", systemPrompt)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = strings.NewReader(userInput)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("execute claude: %w, stderr: %s", err, exitErr.Stderr)
		}
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

// buildClaudeInput separates messages into a system prompt and user input.
// System messages are joined into a single system prompt string.
// Non-system messages become the user input: history as context, last message as the question.
func buildClaudeInput(messages []Message) (systemPrompt, userInput string) {
	var system strings.Builder
	var history []Message

	for _, m := range messages {
		if m.Role == "system" {
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
		} else {
			history = append(history, m)
		}
	}

	var b strings.Builder

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

	return system.String(), b.String()
}
