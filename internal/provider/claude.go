package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// claudeResponse is the JSON output from `claude -p --output-format json`.
type claudeResponse struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// Claude executes the Claude CLI in pipe mode.
type Claude struct {
	timeout time.Duration

	mu         sync.RWMutex
	authStatus string // "ok", "auth_error", or "" (unknown)
}

// NewClaude creates a new Claude CLI provider.
func NewClaude() *Claude {
	return &Claude{
		timeout: 60 * time.Second,
	}
}

// Name returns the provider name "claude".
func (c *Claude) Name() string { return "claude" }

// AuthStatus returns the last known auth status: "ok", "auth_error", or "" (unknown).
func (c *Claude) AuthStatus() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authStatus
}

func (c *Claude) setAuthStatus(status string) {
	c.mu.Lock()
	c.authStatus = status
	c.mu.Unlock()
}

// Chat sends a conversation to the Claude CLI in pipe mode and returns the response.
func (c *Claude) Chat(ctx context.Context, messages []Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	input := buildClaudeInput(messages)

	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "json", "--allowedTools", "WebSearch,WebFetch")
	cmd.Stdin = strings.NewReader(input)

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("execute claude: %w: %w", ErrTimeout, err)
		}
		return "", fmt.Errorf("execute claude: %w", err)
	}

	var resp claudeResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return "", fmt.Errorf("parse claude response: %w", err)
	}

	if resp.IsError {
		if isAuthError(resp.Result) {
			c.setAuthStatus("auth_error")
			return "", fmt.Errorf("claude: %s: %w", resp.Result, ErrAuthFailure)
		}
		// CLI executed and authenticated — clear any stale auth_error.
		c.setAuthStatus("ok")
		return "", fmt.Errorf("claude error: %s", resp.Result)
	}

	if resp.Result == "" {
		return "", fmt.Errorf("empty response from claude")
	}

	c.setAuthStatus("ok")
	return resp.Result, nil
}

// claudeStreamEvent is a single NDJSON line from `claude -p --output-format stream-json`.
type claudeStreamEvent struct {
	Type    string `json:"type"`
	Result  string `json:"result"`
	Message struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// ChatStream sends a conversation to the Claude CLI in streaming mode and
// calls fn with text deltas as they arrive.
func (c *Claude) ChatStream(ctx context.Context, messages []Message, fn func(delta string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	input := buildClaudeInput(messages)

	cmd := exec.CommandContext(ctx, "claude", "-p", "--output-format", "stream-json", "--verbose", "--allowedTools", "WebSearch,WebFetch")
	cmd.Stdin = strings.NewReader(input)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start claude: %w", err)
	}

	var prevText string
	var result string

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var evt claudeStreamEvent
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "assistant":
			if len(evt.Message.Content) > 0 {
				text := evt.Message.Content[0].Text
				if len(text) > len(prevText) {
					delta := text[len(prevText):]
					fn(delta)
					prevText = text
				}
			}
		case "result":
			result = evt.Result
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read claude stdout: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("execute claude: %w: %w", ErrTimeout, err)
		}
		return "", fmt.Errorf("execute claude: %w", err)
	}

	if result == "" {
		return "", fmt.Errorf("empty response from claude stream")
	}

	c.setAuthStatus("ok")
	return result, nil
}

// isAuthError checks if the error message indicates an authentication problem.
func isAuthError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "not logged in") ||
		strings.Contains(lower, "please run /login") ||
		strings.Contains(lower, "token expired") ||
		strings.Contains(lower, "unauthorized")
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
