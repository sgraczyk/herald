package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
)

// mockProvider implements provider.LLMProvider for testing.
type mockProvider struct {
	name     string
	response string
	err      error
	called   bool
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Chat(_ context.Context, _ []provider.Message) (string, error) {
	m.called = true
	return m.response, m.err
}

func testLoop(t *testing.T, p provider.LLMProvider) (*Loop, *hub.Hub, *store.DB) {
	t.Helper()
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	l := NewLoop(h, p, db, 50)
	return l, h, db
}

func readOut(t *testing.T, h *hub.Hub) hub.OutMessage {
	t.Helper()
	select {
	case msg := <-h.Out:
		return msg
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for output message")
		return hub.OutMessage{}
	}
}

func TestClearCommand(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, db := testLoop(t, mock)

	// Add a message to history first.
	_ = db.Append(1, provider.Message{Role: "user", Content: "hello"}, 50)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/clear"})

	out := readOut(t, h)
	if out.Text != "History cleared." {
		t.Errorf("expected 'History cleared.', got %q", out.Text)
	}

	count, _ := db.Count(1)
	if count != 0 {
		t.Errorf("expected 0 messages after clear, got %d", count)
	}
}

func TestStatusCommand(t *testing.T) {
	mock := &mockProvider{name: "claude-cli"}
	l, h, db := testLoop(t, mock)

	// Add some messages.
	for i := 0; i < 3; i++ {
		_ = db.Append(1, provider.Message{Role: "user", Content: "msg"}, 50)
	}

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/status"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "Provider: claude-cli") {
		t.Errorf("expected provider name, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "Messages: 3/50") {
		t.Errorf("expected message count, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "Uptime:") {
		t.Errorf("expected uptime, got %q", out.Text)
	}
}

func TestModelCommandNoArg(t *testing.T) {
	fb := provider.NewFallback([]provider.LLMProvider{
		&mockProvider{name: "claude-cli"},
		&mockProvider{name: "chutes"},
	})
	l, h, _ := testLoop(t, fb)
	l.provider = fb

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/model"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "Active: claude-cli") {
		t.Errorf("expected active provider, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "- claude-cli") || !strings.Contains(out.Text, "- chutes") {
		t.Errorf("expected available providers, got %q", out.Text)
	}
}

func TestModelCommandSwitch(t *testing.T) {
	fb := provider.NewFallback([]provider.LLMProvider{
		&mockProvider{name: "claude-cli"},
		&mockProvider{name: "chutes"},
	})
	l, h, _ := testLoop(t, fb)
	l.provider = fb

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/model", Text: "chutes"})

	out := readOut(t, h)
	if out.Text != "Switched to chutes." {
		t.Errorf("expected switch confirmation, got %q", out.Text)
	}
	if fb.Name() != "chutes" {
		t.Errorf("expected active provider 'chutes', got %q", fb.Name())
	}
}

func TestModelCommandInvalid(t *testing.T) {
	fb := provider.NewFallback([]provider.LLMProvider{
		&mockProvider{name: "claude-cli"},
	})
	l, h, _ := testLoop(t, fb)
	l.provider = fb

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/model", Text: "nonexistent"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "Error:") {
		t.Errorf("expected error message, got %q", out.Text)
	}
}

func TestHandleMessageErrorTimeout(t *testing.T) {
	mock := &mockProvider{name: "test", err: fmt.Errorf("slow: %w", provider.ErrTimeout)}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "took too long") {
		t.Errorf("expected timeout message, got %q", out.Text)
	}
}

func TestHandleMessageErrorAuth(t *testing.T) {
	mock := &mockProvider{name: "test", err: fmt.Errorf("bad creds: %w", provider.ErrAuthFailure)}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "configuration issue") {
		t.Errorf("expected auth error message, got %q", out.Text)
	}
}

func TestHandleMessageErrorGeneric(t *testing.T) {
	mock := &mockProvider{name: "test", err: fmt.Errorf("something broke")}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "temporarily unavailable") {
		t.Errorf("expected generic error message, got %q", out.Text)
	}
}

func TestUnknownCommandPassesToLLM(t *testing.T) {
	mock := &mockProvider{name: "test", response: "I don't know that command."}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/foo", Text: "/foo bar"})

	out := readOut(t, h)
	if !mock.called {
		t.Error("expected provider to be called for unknown command")
	}
	if out.Text != "I don't know that command." {
		t.Errorf("expected LLM response, got %q", out.Text)
	}
}
