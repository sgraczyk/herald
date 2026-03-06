package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
)

// Loop reads messages from the hub, calls the provider, and writes responses back.
type Loop struct {
	hub          *hub.Hub
	provider     provider.LLMProvider
	store        *store.DB
	historyLimit int
	startTime    time.Time
}

// NewLoop creates a new agent loop.
func NewLoop(h *hub.Hub, p provider.LLMProvider, s *store.DB, historyLimit int) *Loop {
	return &Loop{
		hub:          h,
		provider:     p,
		store:        s,
		historyLimit: historyLimit,
		startTime:    time.Now(),
	}
}

// StartTime returns when the loop was created.
func (l *Loop) StartTime() time.Time { return l.startTime }

// Run starts the agent loop. It blocks until ctx is cancelled.
func (l *Loop) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-l.hub.In:
			l.handle(ctx, msg)
		}
	}
}

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
		log.Printf("clear chat %d: %v", msg.ChatID, err)
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to clear history."}
		return
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "History cleared."}
}

func (l *Loop) handleStatus(msg hub.InMessage) {
	count, err := l.store.Count(msg.ChatID)
	if err != nil {
		log.Printf("count chat %d: %v", msg.ChatID, err)
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
		log.Printf("add memory chat %d: %v", msg.ChatID, err)
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
		log.Printf("remove memory chat %d: %v", msg.ChatID, err)
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
		log.Printf("list memories chat %d: %v", msg.ChatID, err)
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
	// Save user message.
	userMsg := provider.Message{
		Role:      "user",
		Content:   msg.Text,
		Timestamp: time.Now(),
	}
	if err := l.store.Append(msg.ChatID, userMsg, l.historyLimit); err != nil {
		log.Printf("save user message: %v", err)
	}

	// Load history and memories.
	history, err := l.store.List(msg.ChatID)
	if err != nil {
		log.Printf("load history: %v", err)
	}
	memories, err := l.store.ListMemories(msg.ChatID)
	if err != nil {
		log.Printf("load memories: %v", err)
	}

	// Build messages and call provider.
	messages := buildMessages(history, memories, msg.Text)
	response, err := l.provider.Chat(ctx, messages)
	if err != nil {
		log.Printf("provider error: %v", err)
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

	// Save assistant response.
	assistantMsg := provider.Message{
		Role:      "assistant",
		Content:   response,
		Timestamp: time.Now(),
	}
	if err := l.store.Append(msg.ChatID, assistantMsg, l.historyLimit); err != nil {
		log.Printf("save assistant message: %v", err)
	}

	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: response}

	// Extract memories from the conversation turn.
	l.extractMemories(ctx, msg.ChatID, msg.Text, response)
}

const extractionPrompt = `Extract any notable facts, preferences, or personal details about the user from this exchange. Return ONLY a JSON array of short strings, or an empty array [] if there is nothing worth remembering.

User: %s
Assistant: %s`

func (l *Loop) extractMemories(ctx context.Context, chatID int64, userText, assistantText string) {
	prompt := fmt.Sprintf(extractionPrompt, userText, assistantText)
	msgs := []provider.Message{
		{Role: "user", Content: prompt},
	}

	resp, err := l.provider.Chat(ctx, msgs)
	if err != nil {
		log.Printf("extract memories: %v", err)
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
		log.Printf("parse extracted memories: %v (response: %q)", err, resp)
		return
	}

	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}

		exists, err := l.store.HasMemory(chatID, fact)
		if err != nil {
			log.Printf("check memory exists: %v", err)
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
			log.Printf("save extracted memory: %v", err)
		}
	}
}
