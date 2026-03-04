package provider

import (
	"testing"
	"time"
)

func TestBuildClaudeInput(t *testing.T) {
	tests := []struct {
		name             string
		messages         []Message
		wantSystemPrompt string
		wantUserInput    string
	}{
		{
			name: "single user message",
			messages: []Message{
				{Role: "user", Content: "Hello"},
			},
			wantSystemPrompt: "",
			wantUserInput:    "Hello",
		},
		{
			name: "system prompt and user message",
			messages: []Message{
				{Role: "system", Content: "You are a helpful assistant."},
				{Role: "user", Content: "Hello"},
			},
			wantSystemPrompt: "You are a helpful assistant.",
			wantUserInput:    "Hello",
		},
		{
			name: "multiple system messages joined",
			messages: []Message{
				{Role: "system", Content: "Be helpful."},
				{Role: "system", Content: "Be concise."},
				{Role: "user", Content: "Hello"},
			},
			wantSystemPrompt: "Be helpful.\n\nBe concise.",
			wantUserInput:    "Hello",
		},
		{
			name: "conversation history",
			messages: []Message{
				{Role: "system", Content: "You are helpful."},
				{Role: "user", Content: "Hi"},
				{Role: "assistant", Content: "Hello!"},
				{Role: "user", Content: "How are you?"},
			},
			wantSystemPrompt: "You are helpful.",
			wantUserInput:    "Conversation so far:\n[user]: Hi\n[assistant]: Hello!\n\nHow are you?",
		},
		{
			name:             "empty messages",
			messages:         []Message{},
			wantSystemPrompt: "",
			wantUserInput:    "",
		},
		{
			name: "system only",
			messages: []Message{
				{Role: "system", Content: "You are helpful."},
			},
			wantSystemPrompt: "You are helpful.",
			wantUserInput:    "",
		},
		{
			name: "timestamps ignored in output",
			messages: []Message{
				{Role: "user", Content: "Hello", Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			},
			wantSystemPrompt: "",
			wantUserInput:    "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSystem, gotInput := buildClaudeInput(tt.messages)
			if gotSystem != tt.wantSystemPrompt {
				t.Errorf("systemPrompt:\n got: %q\nwant: %q", gotSystem, tt.wantSystemPrompt)
			}
			if gotInput != tt.wantUserInput {
				t.Errorf("userInput:\n got: %q\nwant: %q", gotInput, tt.wantUserInput)
			}
		})
	}
}
