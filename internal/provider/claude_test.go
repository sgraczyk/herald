package provider

import "testing"

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
