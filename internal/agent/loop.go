package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
)

// Loop reads messages from the hub, calls the provider, and writes responses back.
type Loop struct {
	hub          *hub.Hub
	provider     provider.LLMProvider
	extProvider  provider.LLMProvider // provider used for memory extraction
	store        *store.DB
	historyLimit int
	systemPrompt string
	startTime    time.Time
	wg           sync.WaitGroup
}

// NewLoop creates a new agent loop. If systemPrompt is empty, the default
// hardcoded prompt is used.
func NewLoop(h *hub.Hub, p provider.LLMProvider, s *store.DB, historyLimit int, systemPrompt string) *Loop {
	return &Loop{
		hub:          h,
		provider:     p,
		extProvider:  pickExtractionProvider(p),
		store:        s,
		historyLimit: historyLimit,
		systemPrompt: systemPrompt,
		startTime:    time.Now(),
	}
}

// pickExtractionProvider selects the provider to use for background memory
// extraction. It prefers an OpenAI-compatible HTTP provider over claude -p
// to avoid spawning a second Node.js process on limited RAM.
func pickExtractionProvider(p provider.LLMProvider) provider.LLMProvider {
	fb, ok := p.(*provider.Fallback)
	if !ok {
		return p
	}
	for _, pp := range fb.Providers() {
		if _, ok := pp.(*provider.OpenAI); ok {
			return pp
		}
	}
	return p
}

// StartTime returns when the loop was created.
func (l *Loop) StartTime() time.Time { return l.startTime }

// Wait blocks until all background memory extractions complete.
func (l *Loop) Wait() { l.wg.Wait() }

// Run starts the agent loop. It blocks until ctx is cancelled.
// On shutdown, it waits up to 10 seconds for in-flight memory extractions.
func (l *Loop) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			l.drainExtractions()
			return
		case msg := <-l.hub.In:
			l.handle(ctx, msg)
		}
	}
}

// drainExtractions waits for in-flight memory extractions to finish,
// with a 10-second timeout to avoid blocking shutdown indefinitely.
func (l *Loop) drainExtractions() {
	done := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		slog.Warn("timed out waiting for memory extractions to finish")
	}
}

// handle routes commands to their handlers.
// Keep in sync with knownCommands in telegram.Adapter (internal/telegram/adapter.go).
func (l *Loop) handle(ctx context.Context, msg hub.InMessage) {
	switch msg.Command {
	case "/clear":
		l.handleClear(msg)
	case "/status":
		l.handleStatus(msg)
	case "/model":
		l.handleModel(msg)
	case "/remember":
		l.handleRemember(msg)
	case "/forget":
		l.handleForget(msg)
	case "/memories":
		l.handleMemories(msg)
	default:
		l.handleMessage(ctx, msg)
	}
}

func (l *Loop) handleClear(msg hub.InMessage) {
	if err := l.store.Clear(msg.ChatID); err != nil {
		slog.Error("clear chat failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to clear history."}
		return
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "History cleared."}
}

func (l *Loop) handleStatus(msg hub.InMessage) {
	count, err := l.store.Count(msg.ChatID)
	if err != nil {
		slog.Error("count chat failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}
	uptime := time.Since(l.startTime).Truncate(time.Second)
	text := fmt.Sprintf("Provider: %s\nMessages: %d/%d\nUptime: %s", l.provider.Name(), count, l.historyLimit, uptime)
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: text}
}

func (l *Loop) handleModel(msg hub.InMessage) {
	fb, ok := l.provider.(*provider.Fallback)

	// Switch provider if an argument was given.
	if msg.Text != "" {
		if !ok {
			l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Provider switching not available."}
			return
		}
		if err := fb.SetActive(msg.Text); err != nil {
			l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: fmt.Sprintf("Error: %v", err)}
			return
		}
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: fmt.Sprintf("Switched to %s.", fb.Name())}
		return
	}

	// Show current status.
	text := fmt.Sprintf("Active: %s", l.provider.Name())
	if ok {
		text += "\nAvailable:"
		for _, p := range fb.Providers() {
			text += fmt.Sprintf("\n- %s", p.Name())
		}
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: text}
}

func (l *Loop) handleRemember(msg hub.InMessage) {
	if msg.Text == "" {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Usage: /remember <fact>"}
		return
	}

	mem := store.Memory{
		Fact:      msg.Text,
		Source:    "explicit",
		Timestamp: time.Now(),
	}
	if err := l.store.AddMemory(msg.ChatID, mem); err != nil {
		slog.Error("add memory failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to save memory."}
		return
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: fmt.Sprintf("Remembered: %s", msg.Text)}
}

func (l *Loop) handleForget(msg hub.InMessage) {
	if msg.Text == "" {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Usage: /forget <fact>"}
		return
	}

	removed, err := l.store.RemoveMemory(msg.ChatID, msg.Text)
	if err != nil {
		slog.Error("remove memory failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to remove memory."}
		return
	}
	if !removed {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "No matching memory found."}
		return
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Memory removed."}
}

func (l *Loop) handleMemories(msg hub.InMessage) {
	mems, err := l.store.ListMemories(msg.ChatID)
	if err != nil {
		slog.Error("list memories failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to list memories."}
		return
	}
	if len(mems) == 0 {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "No memories stored."}
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Memories (%d):\n", len(mems))
	for _, m := range mems {
		fmt.Fprintf(&b, "- %s [%s]\n", m.Fact, m.Source)
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: b.String()}
}

func (l *Loop) handleMessage(ctx context.Context, msg hub.InMessage) {
	// Load history and memories.
	history, err := l.store.List(msg.ChatID)
	if err != nil {
		slog.Error("load history failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}
	memories, err := l.store.ListMemories(msg.ChatID)
	if err != nil {
		slog.Error("load memories failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}

	// Signal typing indicator before calling the provider.
	l.hub.Typing <- msg.ChatID

	// Build messages and call provider.
	messages := buildMessages(history, memories, msg.Text, l.systemPrompt)
	response, err := l.provider.Chat(ctx, messages)
	if err != nil {
		slog.Error("provider call failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		var errText string
		switch {
		case errors.Is(err, provider.ErrTimeout):
			errText = "Response took too long. Try a simpler question or try again shortly."
		case errors.Is(err, provider.ErrAuthFailure):
			errText = "Service configuration issue. The admin has been notified."
		default:
			errText = "I'm temporarily unavailable. Please try again shortly."
		}
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: errText}
		return
	}

	// Save user message.
	userMsg := provider.Message{
		Role:      "user",
		Content:   msg.Text,
		Timestamp: time.Now(),
	}
	if err := l.store.Append(msg.ChatID, userMsg, l.historyLimit); err != nil {
		slog.Error("save user message failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}

	// Save assistant response.
	assistantMsg := provider.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	}
	if err := l.store.Append(msg.ChatID, assistantMsg, l.historyLimit); err != nil {
		slog.Error("save assistant message failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}

	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: response}

	// Skip extraction for trivial messages.
	if isTrivialMessage(msg.Text) {
		return
	}

	// Extract memories in the background so the loop can process the next message.
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in memory extraction", slog.Int64("chat_id", msg.ChatID), slog.Any("panic", r))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		l.extractMemories(ctx, msg.ChatID, msg.Text, response)
	}()
}

// isTrivialMessage returns true for messages too short to contain memorable content.
func isTrivialMessage(text string) bool {
	text = strings.TrimSpace(text)
	return len(text) < 10 || !strings.Contains(text, " ")
}

const extractionPrompt = `Extract notable facts, preferences, or personal details about the user from this exchange. Return ONLY a JSON array of short factual strings, or an empty array [] if nothing is worth remembering.

Rules:
- Only extract durable facts (preferences, background, habits), not transient conversation topics
- Keep each fact short and canonical (e.g., "prefers Go" not "the user mentioned they like Go")
- Do NOT extract what the assistant said, only what reveals something about the user

User: %s
Assistant: %s`

func (l *Loop) extractMemories(ctx context.Context, chatID int64, userText, assistantText string) {
	prompt := fmt.Sprintf(extractionPrompt, userText, assistantText)
	msgs := []provider.Message{
		{Role: "system", Content: "Extract facts as a JSON array of short strings. Return only valid JSON, no explanation."},
		{Role: "user", Content: prompt},
	}

	resp, err := l.extProvider.Chat(ctx, msgs)
	if err != nil {
		slog.Debug("extract memories failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		return
	}

	// Parse JSON array from response. The LLM may wrap it in markdown fences.
	resp = strings.TrimSpace(resp)
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimPrefix(resp, "```")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	var facts []string
	if err := json.Unmarshal([]byte(resp), &facts); err != nil {
		slog.Debug("parse extracted memories failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()), slog.String("response", resp))
		return
	}

	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}

		exists, err := l.store.HasMemory(chatID, fact)
		if err != nil {
			slog.Warn("check memory exists failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
			continue
		}
		if exists {
			continue
		}

		mem := store.Memory{
			Fact:      fact,
			Source:    "auto",
			Timestamp: time.Now(),
		}
		if err := l.store.AddMemory(chatID, mem); err != nil {
			slog.Warn("save extracted memory failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		}
	}
}
