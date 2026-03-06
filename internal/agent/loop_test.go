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
	l := NewLoop(h, p, db, 50, "")
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

func TestRememberCommand(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, db := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/remember", Text: "I prefer Go"})

	out := readOut(t, h)
	if out.Text != "Remembered: I prefer Go" {
		t.Errorf("expected confirmation, got %q", out.Text)
	}

	mems, _ := db.ListMemories(1)
	if len(mems) != 1 || mems[0].Fact != "I prefer Go" || mems[0].Source != "explicit" {
		t.Errorf("unexpected memories: %+v", mems)
	}
}

func TestRememberCommandEmpty(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/remember", Text: ""})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "Usage") {
		t.Errorf("expected usage message, got %q", out.Text)
	}
}

func TestForgetCommand(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, db := testLoop(t, mock)

	db.AddMemory(1, store.Memory{Fact: "prefers Go over Python", Source: "explicit"})

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/forget", Text: "python"})

	out := readOut(t, h)
	if out.Text != "Memory removed." {
		t.Errorf("expected removal confirmation, got %q", out.Text)
	}

	mems, _ := db.ListMemories(1)
	if len(mems) != 0 {
		t.Errorf("expected 0 memories after forget, got %d", len(mems))
	}
}

func TestForgetCommandNoMatch(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/forget", Text: "nonexistent"})

	out := readOut(t, h)
	if out.Text != "No matching memory found." {
		t.Errorf("expected no match message, got %q", out.Text)
	}
}

func TestMemoriesCommand(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, db := testLoop(t, mock)

	db.AddMemory(1, store.Memory{Fact: "prefers Go", Source: "explicit"})
	db.AddMemory(1, store.Memory{Fact: "lives in Warsaw", Source: "auto"})

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/memories"})

	out := readOut(t, h)
	if !strings.Contains(out.Text, "Memories (2)") {
		t.Errorf("expected memory count, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "prefers Go [explicit]") {
		t.Errorf("expected explicit memory, got %q", out.Text)
	}
	if !strings.Contains(out.Text, "lives in Warsaw [auto]") {
		t.Errorf("expected auto memory, got %q", out.Text)
	}
}

func TestMemoriesCommandEmpty(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, _ := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/memories"})

	out := readOut(t, h)
	if out.Text != "No memories stored." {
		t.Errorf("expected empty message, got %q", out.Text)
	}
}

func TestAutoExtraction(t *testing.T) {
	callCount := 0
	l, h, db := testLoop(t, &mockProvider{name: "test"})

	l.provider = provider.LLMProvider(&sequentialProvider{
		responses: []string{"Sure, I can help!", `["prefers Go", "works at Acme"]`},
		callCount: &callCount,
	})

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "I prefer Go and I work at Acme"})

	out := readOut(t, h)
	if out.Text != "Sure, I can help!" {
		t.Errorf("expected chat response, got %q", out.Text)
	}

	mems, _ := db.ListMemories(1)
	if len(mems) != 2 {
		t.Fatalf("expected 2 auto-extracted memories, got %d", len(mems))
	}
	for _, m := range mems {
		if m.Source != "auto" {
			t.Errorf("expected auto source, got %q", m.Source)
		}
	}
}

func TestAutoExtractionDedup(t *testing.T) {
	callCount := 0
	l, h, db := testLoop(t, &mockProvider{name: "test"})

	// Pre-store a memory.
	db.AddMemory(1, store.Memory{Fact: "prefers Go", Source: "explicit"})

	l.provider = provider.LLMProvider(&sequentialProvider{
		responses: []string{"OK!", `["prefers Go"]`},
		callCount: &callCount,
	})

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "I prefer Go"})
	readOut(t, h)

	mems, _ := db.ListMemories(1)
	if len(mems) != 1 {
		t.Errorf("expected 1 memory (deduped), got %d", len(mems))
	}
}

func TestBuildMessagesWithMemories(t *testing.T) {
	history := []provider.Message{
		{Role: "user", Content: "hi"},
	}
	memories := []store.Memory{
		{Fact: "prefers Go", Source: "explicit"},
	}

	msgs := buildMessages(history, memories, "hello", "")

	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "prefers Go") {
		t.Errorf("expected memories in system prompt, got %q", msgs[0].Content)
	}
}

func TestSelectMemoriesExplicitPrioritized(t *testing.T) {
	// Build more memories than maxContextMemories.
	var memories []store.Memory
	// Add 10 explicit memories.
	for i := 0; i < 10; i++ {
		memories = append(memories, store.Memory{Fact: fmt.Sprintf("explicit %d", i), Source: "explicit"})
	}
	// Add 60 auto memories (total 70 > maxContextMemories of 50).
	for i := 0; i < 60; i++ {
		memories = append(memories, store.Memory{Fact: fmt.Sprintf("auto %d", i), Source: "auto"})
	}

	selected := selectMemories(memories)

	if len(selected) != maxContextMemories {
		t.Fatalf("expected %d selected, got %d", maxContextMemories, len(selected))
	}

	// Count explicit vs auto in selection.
	explicitCount := 0
	autoCount := 0
	for _, m := range selected {
		if m.Source == "explicit" {
			explicitCount++
		} else {
			autoCount++
		}
	}
	if explicitCount != 10 {
		t.Errorf("expected all 10 explicit memories, got %d", explicitCount)
	}
	if autoCount != 40 {
		t.Errorf("expected 40 auto memories (50-10), got %d", autoCount)
	}

	// Auto memories should be the most recent (last 40 of 60).
	lastAuto := selected[len(selected)-1]
	if lastAuto.Fact != "auto 59" {
		t.Errorf("expected most recent auto memory last, got %q", lastAuto.Fact)
	}
}

func TestSelectMemoriesUnderLimit(t *testing.T) {
	memories := []store.Memory{
		{Fact: "fact 1", Source: "explicit"},
		{Fact: "fact 2", Source: "auto"},
	}

	selected := selectMemories(memories)
	if len(selected) != 2 {
		t.Errorf("expected all memories when under limit, got %d", len(selected))
	}
}

func TestBuildMessagesWithCustomPrompt(t *testing.T) {
	custom := "You are a pirate assistant."
	msgs := buildMessages(nil, nil, "hello", custom)

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != custom {
		t.Errorf("expected custom prompt %q, got %q", custom, msgs[0].Content)
	}
}

func TestBuildMessagesWithoutMemories(t *testing.T) {
	msgs := buildMessages(nil, nil, "hello", "")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if strings.Contains(msgs[0].Content, "You know the following") {
		t.Error("expected no memory section in system prompt")
	}
}

// sequentialProvider returns different responses for each call.
type sequentialProvider struct {
	responses []string
	callCount *int
}

func (s *sequentialProvider) Name() string { return "test" }
func (s *sequentialProvider) Chat(_ context.Context, _ []provider.Message) (string, error) {
	idx := *s.callCount
	*s.callCount++
	if idx < len(s.responses) {
		return s.responses[idx], nil
	}
	return "[]", nil
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
