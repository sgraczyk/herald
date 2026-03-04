package provider

import (
	"context"
	"time"
)

// Message represents a single message in a conversation.
type Message struct {
	Role      string    `json:"role"`    // "user", "assistant", "system"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// LLMProvider is the interface that all LLM backends must implement.
type LLMProvider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (string, error)
}
