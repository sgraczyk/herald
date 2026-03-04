package telegram

import "testing"

func TestParseCommand(t *testing.T) {
	tests := []struct {
		text    string
		wantCmd string
		wantArg string
	}{
		{"hello world", "", "hello world"},
		{"/clear", "/clear", ""},
		{"/clear@herald_bot", "/clear", ""},
		{"/model gpt-4", "/model", "gpt-4"},
		{"/model@herald_bot gpt-4", "/model", "gpt-4"},
		{"/ask what is 2+2?", "/ask", "what is 2+2?"},
		{"/status", "/status", ""},
	}

	for _, tt := range tests {
		cmd, arg := parseCommand(tt.text)
		if cmd != tt.wantCmd || arg != tt.wantArg {
			t.Errorf("parseCommand(%q) = (%q, %q), want (%q, %q)",
				tt.text, cmd, arg, tt.wantCmd, tt.wantArg)
		}
	}
}
