package agent

import (
	"context"
	"fmt"
	"log"
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

	// Load history.
	history, err := l.store.List(msg.ChatID)
	if err != nil {
		log.Printf("load history: %v", err)
	}

	// Build messages and call provider.
	messages := buildMessages(history, msg.Text)
	response, err := l.provider.Chat(ctx, messages)
	if err != nil {
		log.Printf("provider error: %v", err)
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Sorry, I couldn't process your message. Please try again."}
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
}
