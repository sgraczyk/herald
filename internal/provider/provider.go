// Package provider implements LLM backends and the fallback chain.
package provider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAuthFailure indicates that a provider failed due to expired or invalid
// credentials. Callers can check for this with errors.Is to provide targeted
// user-facing messages and health status reporting.
var ErrAuthFailure = errors.New("provider auth failure")

// ErrTimeout indicates that a provider call exceeded its deadline.
var ErrTimeout = errors.New("provider timeout")

// HTTPStatusError represents a non-OK HTTP status code from an API provider.
type HTTPStatusError struct {
	Code int    // HTTP status code
	Body string // response body
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("API error (status %d): %s", e.Code, e.Body)
}

// ImageData holds a base64-encoded image for vision API requests.
type ImageData struct {
	Base64   string `json:"base64"`
	MimeType string `json:"mime_type"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role        string       `json:"role"`    // "user", "assistant", "system"
	Content     string       `json:"content"`
	Images      []ImageData  `json:"images,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
	Timestamp   time.Time    `json:"timestamp"`
}

// LLMProvider is the interface that all LLM backends must implement.
type LLMProvider interface {
	// Name returns the display name of the provider.
	Name() string
	// Chat sends a conversation to the LLM and returns the assistant's reply.
	Chat(ctx context.Context, messages []Message) (string, error)
}

// StreamingProvider is an optional interface for providers that support
// incremental response delivery.
type StreamingProvider interface {
	LLMProvider
	// ChatStream sends a conversation and calls fn with text deltas as they arrive.
	// Providers must check ctx.Done() between processing each event/line to allow
	// prompt cancellation. Returns the complete response string on success.
	ChatStream(ctx context.Context, messages []Message, fn func(delta string)) (string, error)
}

// ChatOptions configures a tool-aware chat call.
type ChatOptions struct {
	Tools []ToolDefinition
}

// ToolDefinition describes a tool for providers with native function calling.
type ToolDefinition struct {
	Name        string
	Description string
	Parameters  []ToolParameter
}

// ToolParameter describes a single tool parameter.
type ToolParameter struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

// ChatResponse is the result of a tool-aware chat call.
type ChatResponse struct {
	Text      string     // final text response (empty if tool calls present)
	ToolCalls []ToolCall // tool invocations requested by the LLM
}

// ToolCall represents a tool invocation from the LLM.
type ToolCall struct {
	ID   string            // provider-assigned call ID (OpenAI requires for result pairing)
	Name string
	Args map[string]string
}

// ToolResult feeds a tool execution result back to the provider.
type ToolResult struct {
	CallID string // matches ToolCall.ID
	Result string
}

// ToolProvider is an optional interface for providers that support native
// function calling. Providers that don't implement this get tool support
// via XML prompt injection (PromptCaller).
type ToolProvider interface {
	LLMProvider
	ChatWithTools(ctx context.Context, messages []Message, opts ChatOptions) (*ChatResponse, error)
}
