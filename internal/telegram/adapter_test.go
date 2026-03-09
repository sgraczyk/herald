package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
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

func TestParseMessageStatusCommand(t *testing.T) {
	in := parseMessage(1, 2, "/status")
	if in.Command != "/status" {
		t.Errorf("expected command /status, got %q", in.Command)
	}
	if in.Text != "/status" {
		t.Errorf("expected text '/status', got %q", in.Text)
	}
}

// testAdapter creates an Adapter with a real Hub, bypassing bot.New.
func testAdapter(t *testing.T, allowedIDs map[int64]bool) (*Adapter, *hub.Hub) {
	t.Helper()
	h := hub.New()
	return &Adapter{
		hub:        h,
		allowedIDs: allowedIDs,
		typing:     make(map[int64]context.CancelFunc),
	}, h
}

// readIn reads from Hub.In with a short timeout to avoid hangs.
func readIn(t *testing.T, h *hub.Hub) (hub.InMessage, bool) {
	t.Helper()
	select {
	case msg := <-h.In:
		return msg, true
	case <-time.After(100 * time.Millisecond):
		return hub.InMessage{}, false
	}
}

func TestHandleUpdateAuthorizedUser(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 111},
			Chat: models.Chat{ID: 42},
			Text: "hello",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	msg, ok := readIn(t, h)
	if !ok {
		t.Fatal("expected message on Hub.In, got timeout")
	}
	if msg.ChatID != 42 {
		t.Errorf("expected ChatID 42, got %d", msg.ChatID)
	}
	if msg.UserID != 111 {
		t.Errorf("expected UserID 111, got %d", msg.UserID)
	}
	if msg.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", msg.Text)
	}
}

func TestHandleUpdateUnauthorizedUser(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 999},
			Chat: models.Chat{ID: 42},
			Text: "hello",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	_, ok := readIn(t, h)
	if ok {
		t.Fatal("expected no message on Hub.In for unauthorized user")
	}
}

func TestHandleUpdateEmptyWhitelist(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 111},
			Chat: models.Chat{ID: 42},
			Text: "hello",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	_, ok := readIn(t, h)
	if ok {
		t.Fatal("expected no message on Hub.In for empty whitelist")
	}
}

func TestHandleUpdateNilMessage(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{Message: nil}
	a.handleUpdate(context.Background(), nil, update)

	_, ok := readIn(t, h)
	if ok {
		t.Fatal("expected no message on Hub.In for nil message")
	}
}

func TestHandleUpdateEmptyText(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 111},
			Chat: models.Chat{ID: 42},
			Text: "",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	_, ok := readIn(t, h)
	if ok {
		t.Fatal("expected no message on Hub.In for empty text")
	}
}

func TestHandleUpdateWhitespaceText(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 111},
			Chat: models.Chat{ID: 42},
			Text: "   ",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	_, ok := readIn(t, h)
	if ok {
		t.Fatal("expected no message on Hub.In for whitespace-only text")
	}
}

func TestHandleUpdateCommandParsing(t *testing.T) {
	a, h := testAdapter(t, map[int64]bool{111: true})

	update := &models.Update{
		Message: &models.Message{
			From: &models.User{ID: 111},
			Chat: models.Chat{ID: 42},
			Text: "/model chutes",
		},
	}
	a.handleUpdate(context.Background(), nil, update)

	msg, ok := readIn(t, h)
	if !ok {
		t.Fatal("expected message on Hub.In, got timeout")
	}
	if msg.Command != "/model" {
		t.Errorf("expected command /model, got %q", msg.Command)
	}
	if msg.Text != "chutes" {
		t.Errorf("expected text 'chutes', got %q", msg.Text)
	}
}
