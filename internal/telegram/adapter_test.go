package telegram

import (
	"strings"
	"testing"

	"github.com/sgraczyk/herald/internal/hub"
)

func TestNewEmptyAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, nil)
	if err == nil {
		t.Fatal("expected error for nil allowedUserIDs, got nil")
	}
}

func TestNewEmptySliceAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, []int64{})
	if err == nil {
		t.Fatal("expected error for empty allowedUserIDs, got nil")
	}
}

func TestNewZeroOnlyAllowedIDs(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, []int64{0, 0})
	if err == nil {
		t.Fatal("expected error when all IDs are zero, got nil")
	}
}

func TestNewZeroFilteredFromAllowedIDs(t *testing.T) {
	h := hub.New()
	// This will fail at bot.New because "test-token" is not a real token,
	// but it should NOT fail with the "no valid allowed user IDs" error.
	_, err := New("test-token", h, []int64{0, 12345})
	if err == nil {
		return // bot.New succeeded (unlikely with fake token, but acceptable)
	}
	if strings.Contains(err.Error(), "no valid allowed user IDs") {
		t.Fatal("zero ID was not filtered; valid ID 12345 should have been kept")
	}
}

func TestNewNegativeIDsFiltered(t *testing.T) {
	h := hub.New()
	_, err := New("test-token", h, []int64{-1, -999})
	if err == nil {
		t.Fatal("expected error when all IDs are negative, got nil")
	}
}

func TestParseMessageUnknownCommandWithArgs(t *testing.T) {
	in := parseMessage(1, 2, "/foo bar")
	if in.Command != "/foo" {
		t.Errorf("expected command /foo, got %q", in.Command)
	}
	if in.Text != "/foo bar" {
		t.Errorf("expected text '/foo bar', got %q", in.Text)
	}
}

func TestParseMessageUnknownCommandNoArgs(t *testing.T) {
	in := parseMessage(1, 2, "/foo")
	if in.Command != "/foo" {
		t.Errorf("expected command /foo, got %q", in.Command)
	}
	if in.Text != "/foo" {
		t.Errorf("expected text '/foo', got %q", in.Text)
	}
}

func TestParseMessageKnownCommandWithArgs(t *testing.T) {
	for _, tc := range []struct {
		text    string
		cmd     string
		wantArg string
	}{
		{"/model claude", "/model", "claude"},
		{"/remember I prefer Go", "/remember", "I prefer Go"},
		{"/forget python", "/forget", "python"},
	} {
		in := parseMessage(1, 2, tc.text)
		if in.Command != tc.cmd {
			t.Errorf("%s: expected command %q, got %q", tc.text, tc.cmd, in.Command)
		}
		if in.Text != tc.wantArg {
			t.Errorf("%s: expected text %q, got %q", tc.text, tc.wantArg, in.Text)
		}
	}
}

func TestParseMessageKnownCommandNoArgs(t *testing.T) {
	in := parseMessage(1, 2, "/clear")
	if in.Command != "/clear" {
		t.Errorf("expected command /clear, got %q", in.Command)
	}
	if in.Text != "/clear" {
		t.Errorf("expected text '/clear', got %q", in.Text)
	}
}

func TestParseMessageRegularText(t *testing.T) {
	in := parseMessage(1, 2, "hello world")
	if in.Command != "" {
		t.Errorf("expected empty command, got %q", in.Command)
	}
	if in.Text != "hello world" {
		t.Errorf("expected text 'hello world', got %q", in.Text)
	}
}

func TestParseMessageBotMentionSuffix(t *testing.T) {
	in := parseMessage(1, 2, "/clear@herald_bot")
	if in.Command != "/clear" {
		t.Errorf("expected command /clear, got %q", in.Command)
	}
}

func TestAllowedIDsMap(t *testing.T) {
	a := &Adapter{
		allowedIDs: map[int64]bool{111: true, 222: true},
	}

	if !a.allowedIDs[111] {
		t.Error("expected user 111 to be allowed")
	}
	if !a.allowedIDs[222] {
		t.Error("expected user 222 to be allowed")
	}
	if a.allowedIDs[999] {
		t.Error("expected user 999 to be rejected")
	}
}
