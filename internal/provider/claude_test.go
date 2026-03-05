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
