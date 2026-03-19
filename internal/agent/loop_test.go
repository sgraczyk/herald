package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	l := NewLoop(h, p, db, 50, 8000, 0, false, false, "", nil)
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

	// Add a message and a summary.
	_ = db.Append(1, provider.Message{Role: "user", Content: "hello"}, 50)
	_ = db.SaveSummary(1, "some old summary")

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/clear"})

	out := readOut(t, h)
	if out.Text != "History cleared." {
		t.Errorf("expected 'History cleared.', got %q", out.Text)
	}

	count, _ := db.Count(1)
	if count != 0 {
		t.Errorf("expected 0 messages after clear, got %d", count)
	}

	summary, _ := db.GetSummary(1)
	if summary != "" {
		t.Errorf("expected empty summary after clear, got %q", summary)
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
	}, 0, nil)
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
	}, 0, nil)
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
	}, 0, nil)
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
	l, h, db := testLoop(t, &mockProvider{name: "test"})

	sp := provider.LLMProvider(&sequentialProvider{
		responses: []string{"Sure, I can help!", `["prefers Go", "works at Acme"]`},
	})
	l.provider = sp
	l.extProvider = sp

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "I prefer Go and I work at Acme"})

	out := readOut(t, h)
	if out.Text != "Sure, I can help!" {
		t.Errorf("expected chat response, got %q", out.Text)
	}

	l.Wait()
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
	l, h, db := testLoop(t, &mockProvider{name: "test"})

	// Pre-store a memory.
	db.AddMemory(1, store.Memory{Fact: "prefers Go", Source: "explicit"})

	sp := provider.LLMProvider(&sequentialProvider{
		responses: []string{"OK!", `["prefers Go"]`},
	})
	l.provider = sp
	l.extProvider = sp

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "I really prefer Go"})
	readOut(t, h)

	l.Wait()
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

	msgs := buildMessages(history, memories, "hello", "", "")

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
	msgs := buildMessages(nil, nil, "hello", custom, "")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != custom {
		t.Errorf("expected custom prompt %q, got %q", custom, msgs[0].Content)
	}
}

func TestBuildMessagesWithoutMemories(t *testing.T) {
	msgs := buildMessages(nil, nil, "hello", "", "")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if strings.Contains(msgs[0].Content, "You know the following") {
		t.Error("expected no memory section in system prompt")
	}
}

func TestPickExtractionProvider(t *testing.T) {
	mock := &mockProvider{name: "mock"}
	oai := provider.NewOpenAI("openai", "http://localhost", "gpt-4", "key")

	tests := []struct {
		name     string
		provider provider.LLMProvider
		wantName string
	}{
		{"non-fallback returns same provider", mock, "mock"},
		{"fallback with openai returns openai", provider.NewFallback([]provider.LLMProvider{mock, oai}, 0, nil), "openai"},
		{"fallback without openai returns fallback", provider.NewFallback([]provider.LLMProvider{mock}, 0, nil), "mock"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickExtractionProvider(tt.provider)
			if got.Name() != tt.wantName {
				t.Errorf("pickExtractionProvider() = %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}

func TestAsyncExtractionDoesNotBlockResponse(t *testing.T) {
	l, h, _ := testLoop(t, &mockProvider{name: "test"})

	// Use a slow provider for extraction: chat responds instantly, extraction takes 200ms.
	sp := provider.LLMProvider(&sequentialProvider{
		responses: []string{"fast reply", `[]`},
		delay:     []time.Duration{0, 200 * time.Millisecond},
	})
	l.provider = sp
	l.extProvider = sp

	start := time.Now()
	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "some longer test message"})

	out := readOut(t, h)
	elapsed := time.Since(start)

	if out.Text != "fast reply" {
		t.Errorf("expected 'fast reply', got %q", out.Text)
	}
	if elapsed >= 200*time.Millisecond {
		t.Errorf("response took %v, expected < 200ms (extraction should be async)", elapsed)
	}

	l.Wait()
}

func TestTrivialMessageSkipsExtraction(t *testing.T) {
	l, h, _ := testLoop(t, &mockProvider{name: "test"})

	sp := &sequentialProvider{
		responses: []string{"ok"},
	}
	l.provider = sp
	l.extProvider = sp

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hi"})
	readOut(t, h)
	l.Wait()

	if got := int(sp.callCount.Load()); got != 1 {
		t.Errorf("expected 1 provider call (no extraction for trivial message), got %d", got)
	}
}

// sequentialProvider returns different responses for each call.
// The counter is atomic to support concurrent goroutines.
type sequentialProvider struct {
	responses []string
	callCount atomic.Int64
	delay     []time.Duration // optional per-call delay
}

func (s *sequentialProvider) Name() string { return "test" }
func (s *sequentialProvider) Chat(_ context.Context, _ []provider.Message) (string, error) {
	idx := int(s.callCount.Add(1) - 1)
	if idx < len(s.delay) && s.delay[idx] > 0 {
		time.Sleep(s.delay[idx])
	}
	if idx < len(s.responses) {
		return s.responses[idx], nil
	}
	return "[]", nil
}

func TestDrainMessagesOnShutdown(t *testing.T) {
	mock := &mockProvider{name: "test", response: "reply"}
	l, h, _ := testLoop(t, mock)

	// Buffer messages before starting the loop.
	h.In <- hub.InMessage{ChatID: 1, Text: "hi"}
	h.In <- hub.InMessage{ChatID: 2, Text: "hi"}

	// Start loop with an already-cancelled context so it drains immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		l.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after draining")
	}

	// Both messages should have produced responses.
	for i := 0; i < 2; i++ {
		select {
		case out := <-h.Out:
			if out.Text != "reply" {
				t.Errorf("expected 'reply', got %q", out.Text)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing output message %d", i+1)
		}
	}
}

func TestDrainSetsHubDraining(t *testing.T) {
	mock := &mockProvider{name: "test", response: "ok"}
	l, h, _ := testLoop(t, mock)

	if h.Draining() {
		t.Fatal("hub should not be draining before shutdown")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	l.Run(ctx)

	if !h.Draining() {
		t.Fatal("hub should be draining after shutdown")
	}
}

// capturingProvider records the messages it receives.
type capturingProvider struct {
	responses []string
	callCount int
	captured  [][]provider.Message
}

func (c *capturingProvider) Name() string { return "test" }
func (c *capturingProvider) Chat(_ context.Context, msgs []provider.Message) (string, error) {
	cp := make([]provider.Message, len(msgs))
	copy(cp, msgs)
	c.captured = append(c.captured, cp)
	idx := c.callCount
	c.callCount++
	if idx < len(c.responses) {
		return c.responses[idx], nil
	}
	return "[]", nil
}

func TestHandleMessageNoDuplicateUserMessage(t *testing.T) {
	cap := &capturingProvider{responses: []string{"response", "[]"}}
	l, h, db := testLoop(t, cap)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello world message"})
	readOut(t, h)
	l.Wait()

	// The first call to Chat is the main conversation call.
	if len(cap.captured) == 0 {
		t.Fatal("provider was never called")
	}
	msgs := cap.captured[0]

	// Count how many times the user message appears.
	count := 0
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "hello world message" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected user message exactly once, found %d times", count)
	}

	// Verify it's stored exactly once in the DB.
	stored, _ := db.List(1)
	userCount := 0
	for _, m := range stored {
		if m.Role == "user" && m.Content == "hello world message" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Errorf("expected 1 user message in store, got %d", userCount)
	}
}

func TestHandleMessageNoDuplicateWithHistory(t *testing.T) {
	cap := &capturingProvider{responses: []string{"response", "[]"}}
	l, h, db := testLoop(t, cap)

	// Pre-populate history.
	_ = db.Append(1, provider.Message{Role: "user", Content: "previous"}, 50)
	_ = db.Append(1, provider.Message{Role: "assistant", Content: "previous reply"}, 50)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "new message here"})
	readOut(t, h)
	l.Wait()

	msgs := cap.captured[0]
	count := 0
	for _, m := range msgs {
		if m.Role == "user" && m.Content == "new message here" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected user message exactly once, found %d times", count)
	}
}

func TestHandleMessageErrorDoesNotSaveUserMessage(t *testing.T) {
	mock := &mockProvider{name: "test", err: fmt.Errorf("provider down")}
	l, h, db := testLoop(t, mock)

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello"})
	readOut(t, h)

	stored, _ := db.List(1)
	if len(stored) != 0 {
		t.Errorf("expected no messages in store on provider error, got %d", len(stored))
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

func TestSummarizationBeforePrune(t *testing.T) {
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create loop with summarize=true and limit=4.
	l := NewLoop(h, &mockProvider{name: "test"}, db, 4, 8000, 0, true, false, "", nil)

	// Fill history to limit: 4 messages.
	for i := 0; i < 4; i++ {
		_ = db.Append(1, provider.Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)}, 50)
	}

	// Provider: 1st call = chat response, 2nd call = summary.
	// Use a trivial message to skip extraction and avoid racing goroutines.
	sp := &sequentialProvider{
		responses: []string{"chat reply", "Summary of old messages."},
	}
	l.provider = sp
	l.extProvider = sp

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hi"})
	readOut(t, h)
	l.Wait()

	summary, err := db.GetSummary(1)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "Summary of old messages." {
		t.Errorf("expected summary to be saved, got %q", summary)
	}
}

func TestSummarizationFailureDoesNotBreakFlow(t *testing.T) {
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	l := NewLoop(h, &mockProvider{name: "test"}, db, 4, 8000, 0, true, false, "", nil)

	// Fill history to limit.
	for i := 0; i < 4; i++ {
		_ = db.Append(1, provider.Message{Role: "user", Content: fmt.Sprintf("msg-%d", i)}, 50)
	}

	// Provider: 1st = chat response, 2nd = summarization error.
	// Use a trivial message to skip extraction and avoid racing goroutines.
	sp := &errorOnNthProvider{
		responses: []string{"chat reply", ""},
		errorOn:   1,
		err:       fmt.Errorf("summarization failed"),
	}
	l.provider = sp
	l.extProvider = sp

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hi"})
	out := readOut(t, h)
	l.Wait()

	if out.Text != "chat reply" {
		t.Errorf("expected chat reply despite summarization failure, got %q", out.Text)
	}
}

func TestSummaryInContext(t *testing.T) {
	cap := &capturingProvider{responses: []string{"response", "[]"}}
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	l := NewLoop(h, cap, db, 50, 8000, 0, true, false, "", nil)
	l.extProvider = cap

	// Store a summary.
	_ = db.SaveSummary(1, "User prefers concise answers.")

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hello world message"})
	readOut(t, h)
	l.Wait()

	if len(cap.captured) == 0 {
		t.Fatal("provider was never called")
	}
	systemPrompt := cap.captured[0][0].Content
	if !strings.Contains(systemPrompt, "Summary of earlier conversation:") {
		t.Errorf("expected summary in system prompt, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "User prefers concise answers.") {
		t.Errorf("expected summary text in system prompt, got %q", systemPrompt)
	}
}

func TestBuildMessagesWithSummary(t *testing.T) {
	msgs := buildMessages(nil, nil, "hello", "", "User likes Go.")

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Summary of earlier conversation:") {
		t.Errorf("expected summary section in system prompt, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "User likes Go.") {
		t.Errorf("expected summary text in system prompt, got %q", msgs[0].Content)
	}
}

func TestFormatDocumentContext(t *testing.T) {
	doc := &hub.DocumentAttachment{
		Name:       "invoice.pdf",
		Pages:      3,
		Text:       "Some invoice text",
		Truncated:  false,
		ShownPages: 3,
	}
	got := formatDocumentContext(doc)
	if !strings.Contains(got, "--- Document: invoice.pdf (3 pages) ---") {
		t.Errorf("expected non-truncated header, got %q", got)
	}
	if !strings.Contains(got, "--- End of document ---") {
		t.Errorf("expected end marker, got %q", got)
	}

	doc.Truncated = true
	doc.Pages = 5
	doc.ShownPages = 3
	got = formatDocumentContext(doc)
	if !strings.Contains(got, "(3/5 pages shown)") {
		t.Errorf("expected truncated header, got %q", got)
	}
	if !strings.Contains(got, "2 pages omitted") {
		t.Errorf("expected omitted note, got %q", got)
	}
}

// errorOnNthProvider returns an error on a specific call index.
// The counter is atomic to support concurrent goroutines.
type errorOnNthProvider struct {
	responses []string
	errorOn   int
	err       error
	callCount atomic.Int64
}

func (e *errorOnNthProvider) Name() string { return "test" }
func (e *errorOnNthProvider) Chat(_ context.Context, _ []provider.Message) (string, error) {
	idx := int(e.callCount.Add(1) - 1)
	if idx == e.errorOn {
		return "", e.err
	}
	if idx < len(e.responses) {
		return e.responses[idx], nil
	}
	return "[]", nil
}

func TestHandleMessageWithDocument(t *testing.T) {
	cap := &capturingProvider{responses: []string{"I see an invoice.", "[]"}}
	l, h, db := testLoop(t, cap)
	l.extProvider = cap

	doc := &hub.DocumentAttachment{
		Name:       "invoice.pdf",
		MimeType:   "application/pdf",
		Pages:      2,
		Text:       "Invoice #123\nTotal: $500",
		Truncated:  false,
		ShownPages: 2,
	}

	l.handle(context.Background(), hub.InMessage{
		ChatID:   1,
		Text:     "What's the total?",
		Document: doc,
	})
	out := readOut(t, h)
	l.Wait()

	if out.Text != "I see an invoice." {
		t.Errorf("expected 'I see an invoice.', got %q", out.Text)
	}

	// Verify the provider received a system message with document context.
	if len(cap.captured) == 0 {
		t.Fatal("provider was never called")
	}
	msgs := cap.captured[0]
	found := false
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "--- Document: invoice.pdf") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected document system message in provider context")
	}

	// Verify document is stored in history.
	stored, _ := db.List(1)
	docFound := false
	for _, m := range stored {
		if m.Role == "system" && strings.Contains(m.Content, "Invoice #123") {
			docFound = true
		}
	}
	if !docFound {
		t.Error("expected document system message in stored history")
	}

	// Verify user message has placeholder.
	userFound := false
	for _, m := range stored {
		if m.Role == "user" && strings.Contains(m.Content, "[document: invoice.pdf]") {
			userFound = true
		}
	}
	if !userFound {
		t.Error("expected user message with document placeholder in stored history")
	}
}

func TestDocumentPersistsForFollowUp(t *testing.T) {
	cap := &capturingProvider{responses: []string{"The total is $500.", "[]", "The date is Jan 1.", "[]"}}
	l, h, _ := testLoop(t, cap)
	l.extProvider = cap

	doc := &hub.DocumentAttachment{
		Name:       "invoice.pdf",
		MimeType:   "application/pdf",
		Pages:      1,
		Text:       "Invoice #123\nTotal: $500\nDate: Jan 1",
		ShownPages: 1,
	}

	// First message with document.
	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "What's the total?", Document: doc})
	readOut(t, h)
	l.Wait()

	// Follow-up without document.
	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "What's the date?"})
	readOut(t, h)
	l.Wait()

	// Check all captured calls for one that has both the document and the follow-up question.
	found := false
	for _, msgs := range cap.captured {
		hasDoc := false
		hasFollowUp := false
		for _, m := range msgs {
			if m.Role == "system" && strings.Contains(m.Content, "Invoice #123") {
				hasDoc = true
			}
			if m.Role == "user" && m.Content == "What's the date?" {
				hasFollowUp = true
			}
		}
		if hasDoc && hasFollowUp {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected follow-up call to include document from history")
	}
}

func TestSummarizationCompactsDocumentText(t *testing.T) {
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create loop with summarize=true and limit=4.
	l := NewLoop(h, &mockProvider{name: "test"}, db, 4, 8000, 0, true, false, "", nil)

	// Fill history: a document system message + 3 regular messages = 4 total.
	longDocText := "--- Document: invoice.pdf (2 pages) ---\n" + strings.Repeat("Invoice line item. ", 500) + "\n--- End of document ---"
	_ = db.Append(1, provider.Message{Role: "system", Content: longDocText}, 50)
	_ = db.Append(1, provider.Message{Role: "user", Content: "What's the total?"}, 50)
	_ = db.Append(1, provider.Message{Role: "assistant", Content: "The total is $500."}, 50)
	_ = db.Append(1, provider.Message{Role: "user", Content: "msg-3"}, 50)

	// Provider: 1st = chat response, 2nd = summarization call.
	cap := &capturingProvider{responses: []string{"chat reply", "Summary: user discussed an invoice."}}
	l.provider = cap
	l.extProvider = cap

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Text: "hi"})
	readOut(t, h)
	l.Wait()

	// The summarization call is the second captured call (first is the main chat response).
	// It should contain the compacted header, NOT the full document text.
	if len(cap.captured) < 2 {
		t.Fatalf("expected at least 2 provider calls (chat + summarization), got %d", len(cap.captured))
	}
	for _, m := range cap.captured[1] {
		if strings.Contains(m.Content, "Invoice line item.") {
			t.Error("summarization call should not contain full document text, expected compacted header only")
		}
	}
}

func TestParseFactsJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:  "valid array",
			input: `["prefers Go", "lives in Warsaw"]`,
			want:  []string{"prefers Go", "lives in Warsaw"},
		},
		{
			name:  "empty array",
			input: `[]`,
			want:  []string{},
		},
		{
			name:    "malformed JSON",
			input:   `["broken`,
			wantErr: true,
		},
		{
			name:    "non-array JSON object",
			input:   `{"key": "value"}`,
			wantErr: true,
		},
		{
			name:  "JSON with surrounding text",
			input: "Here are the facts:\n```json\n[\"prefers Go\"]\n```\nDone.",
			want:  []string{"prefers Go"},
		},
		{
			name:    "no array at all",
			input:   "No facts found.",
			wantErr: true,
		},
		{
			name:    "array of non-strings",
			input:   `[1, 2, 3]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFactsJSON(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("expected %d facts, got %d", len(tt.want), len(got))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("fact[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestConversationsClearCommand(t *testing.T) {
	mock := &mockProvider{name: "test"}
	l, h, db := testLoop(t, mock)

	// Archive a conversation.
	_ = db.Append(1, provider.Message{Role: "user", Content: "hello"}, 50)
	_, _ = db.ArchiveConversation(1)

	convs, _ := db.ListArchived(1)
	if len(convs) != 1 {
		t.Fatalf("expected 1 archive, got %d", len(convs))
	}

	l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/conversations", Text: "clear"})

	out := readOut(t, h)
	if out.Text != "All archived conversations cleared." {
		t.Errorf("expected clear confirmation, got %q", out.Text)
	}

	convs, _ = db.ListArchived(1)
	if len(convs) != 0 {
		t.Errorf("expected 0 archives after clear, got %d", len(convs))
	}
}

func TestArchivePrunesOldConversations(t *testing.T) {
	h := hub.New()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create loop with maxArchived=2.
	l := NewLoop(h, &mockProvider{name: "test", response: "ok"}, db, 50, 8000, 2, false, false, "", nil)

	// Archive 3 conversations via handleNew.
	for i := 0; i < 3; i++ {
		_ = db.Append(1, provider.Message{Role: "user", Content: fmt.Sprintf("conv-%d", i)}, 50)
		l.handle(context.Background(), hub.InMessage{ChatID: 1, Command: "/new"})
		readOut(t, h)
	}

	convs, _ := db.ListArchived(1)
	if len(convs) != 2 {
		t.Errorf("expected 2 archives after pruning (limit=2), got %d", len(convs))
	}
}
