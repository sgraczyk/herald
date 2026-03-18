package provider

import (
	"context"
	"strings"
	"testing"
)

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"Not logged in · Please run /login", true},
		{"not logged in", true},
		{"Please run /login to authenticate", true},
		{"token expired", true},
		{"Unauthorized", true},
		{"rate limit exceeded", false},
		{"internal server error", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isAuthError(tt.msg)
		if got != tt.want {
			t.Errorf("isAuthError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}

func TestClaudeAuthStatusInitiallyEmpty(t *testing.T) {
	c := NewClaude()
	if got := c.AuthStatus(); got != "" {
		t.Errorf("expected empty initial auth status, got %q", got)
	}
}

func TestScanClaudeStreamSingleAssistant(t *testing.T) {
	input := `{"type":"init"}
{"type":"assistant","message":{"content":[{"text":"Hello, world!"}]}}
{"type":"result","result":"Hello, world!"}
`
	var deltas []string
	c := NewClaude()
	result, err := c.scanClaudeStream(context.Background(), strings.NewReader(input), func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello, world!" {
		t.Errorf("result = %q, want %q", result, "Hello, world!")
	}
	if len(deltas) != 1 {
		t.Fatalf("fn called %d times, want 1", len(deltas))
	}
	if deltas[0] != "Hello, world!" {
		t.Errorf("delta[0] = %q, want %q", deltas[0], "Hello, world!")
	}
}

func TestScanClaudeStreamMultipleAssistants(t *testing.T) {
	input := `{"type":"init"}
{"type":"assistant","message":{"content":[{"text":"Hello"}]}}
{"type":"assistant","message":{"content":[{"text":"Hello, world!"}]}}
{"type":"result","result":"Hello, world!"}
`
	var deltas []string
	c := NewClaude()
	result, err := c.scanClaudeStream(context.Background(), strings.NewReader(input), func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello, world!" {
		t.Errorf("result = %q, want %q", result, "Hello, world!")
	}
	if len(deltas) != 2 {
		t.Fatalf("fn called %d times, want 2", len(deltas))
	}
	if deltas[0] != "Hello" {
		t.Errorf("delta[0] = %q, want %q", deltas[0], "Hello")
	}
	if deltas[1] != ", world!" {
		t.Errorf("delta[1] = %q, want %q", deltas[1], ", world!")
	}
}

func TestScanClaudeStreamEmptyContent(t *testing.T) {
	input := `{"type":"init"}
{"type":"assistant","message":{"content":[{"text":""}]}}
{"type":"result","result":"done"}
`
	var fnCalled int
	c := NewClaude()
	result, err := c.scanClaudeStream(context.Background(), strings.NewReader(input), func(delta string) {
		fnCalled++
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("result = %q, want %q", result, "done")
	}
	if fnCalled != 0 {
		t.Errorf("fn called %d times, want 0", fnCalled)
	}
}

func TestScanClaudeStreamNoResult(t *testing.T) {
	input := `{"type":"init"}
{"type":"assistant","message":{"content":[{"text":"Hello"}]}}
`
	c := NewClaude()
	result, err := c.scanClaudeStream(context.Background(), strings.NewReader(input), func(delta string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("result = %q, want empty string", result)
	}
}

func TestScanClaudeStreamContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Use a reader that blocks, but context is already cancelled so the
	// select in the loop should catch it after scanning the first line.
	input := `{"type":"init"}
{"type":"assistant","message":{"content":[{"text":"Hello"}]}}
`
	c := NewClaude()
	_, err := c.scanClaudeStream(ctx, strings.NewReader(input), func(delta string) {})
	if err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestBuildClaudeInput(t *testing.T) {
	tests := []struct {
		name string
		msgs []Message
		want string
	}{
		{
			name: "empty slice",
			msgs: nil,
			want: "",
		},
		{
			name: "system only",
			msgs: []Message{{Role: "system", Content: "You are helpful"}},
			want: "You are helpful\n\n",
		},
		{
			name: "single user",
			msgs: []Message{{Role: "user", Content: "hi"}},
			want: "hi",
		},
		{
			name: "system + user",
			msgs: []Message{
				{Role: "system", Content: "Be brief"},
				{Role: "user", Content: "hi"},
			},
			want: "Be brief\n\nhi",
		},
		{
			name: "multi-turn 2 msgs",
			msgs: []Message{
				{Role: "user", Content: "hi"},
				{Role: "assistant", Content: "hello"},
			},
			want: "Conversation so far:\n[user]: hi\n\nhello",
		},
		{
			name: "multi-turn 3 msgs",
			msgs: []Message{
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
				{Role: "user", Content: "c"},
			},
			want: "Conversation so far:\n[user]: a\n[assistant]: b\n\nc",
		},
		{
			name: "system + multi-turn",
			msgs: []Message{
				{Role: "system", Content: "ctx"},
				{Role: "user", Content: "a"},
				{Role: "assistant", Content: "b"},
				{Role: "user", Content: "c"},
			},
			want: "ctx\n\nConversation so far:\n[user]: a\n[assistant]: b\n\nc",
		},
		{
			name: "multiple systems",
			msgs: []Message{
				{Role: "system", Content: "s1"},
				{Role: "system", Content: "s2"},
				{Role: "user", Content: "hi"},
			},
			want: "s1\n\ns2\n\nhi",
		},
		{
			name: "system mid-conversation",
			msgs: []Message{
				{Role: "user", Content: "a"},
				{Role: "system", Content: "ctx"},
				{Role: "user", Content: "b"},
			},
			want: "ctx\n\nConversation so far:\n[user]: a\n\nb",
		},
		{
			name: "empty content",
			msgs: []Message{{Role: "user", Content: ""}},
			want: "",
		},
		{
			name: "content with newlines",
			msgs: []Message{{Role: "user", Content: "line1\nline2"}},
			want: "line1\nline2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildClaudeInput(tt.msgs)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
