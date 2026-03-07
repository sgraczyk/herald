package provider

import (
	"context"
	"errors"
	"time"
)

// ErrAuthFailure indicates that a provider failed due to expired or invalid
// credentials. Callers can check for this with errors.Is to provide targeted
// user-facing messages and health status reporting.
var ErrAuthFailure = errors.New("provider auth failure")

// ErrTimeout indicates that a provider call exceeded its deadline.
var ErrTimeout = errors.New("provider timeout")

// ImageData holds a base64-encoded image for vision API requests.
type ImageData struct {
	Base64   string `json:"base64"`
	MimeType string `json:"mime_type"`
}

// Message represents a single message in a conversation.
type Message struct {
	Role      string      `json:"role"`    // "user", "assistant", "system"
	Content   string      `json:"content"`
	Images    []ImageData `json:"images,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

// LLMProvider is the interface that all LLM backends must implement.
type LLMProvider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (string, error)
}
