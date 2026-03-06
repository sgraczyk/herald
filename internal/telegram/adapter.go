package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sgraczyk/herald/internal/format"
	"github.com/sgraczyk/herald/internal/hub"
)

// Adapter connects Telegram to the Hub via long polling.
type Adapter struct {
	bot        *bot.Bot
	hub        *hub.Hub
	allowedIDs map[int64]bool

	mu      sync.Mutex
	typing  map[int64]context.CancelFunc // active typing indicators per chat
}

// New creates a new Telegram adapter.
func New(token string, h *hub.Hub, allowedUserIDs []int64) (*Adapter, error) {
	a := &Adapter{
		hub:        h,
		allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
		typing:     make(map[int64]context.CancelFunc),
	}

	for _, id := range allowedUserIDs {
		a.allowedIDs[id] = true
	}

	b, err := bot.New(token,
		bot.WithDefaultHandler(a.handleUpdate),
	)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	a.bot = b

	return a, nil
}

// Start begins long polling and dispatches outgoing messages.
// Blocks until ctx is cancelled.
func (a *Adapter) Start(ctx context.Context) {
	go a.dispatchOut(ctx)
	go a.dispatchTyping(ctx)

	// Start long polling (blocks).
	a.bot.Start(ctx)
}

func (a *Adapter) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	userID := msg.From.ID
	chatID := msg.Chat.ID

	// Reject unauthorized users.
	if len(a.allowedIDs) > 0 && !a.allowedIDs[userID] {
		log.Printf("rejected message from unauthorized user %d", userID)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	in := hub.InMessage{
		ChatID: chatID,
		UserID: userID,
		Text:   text,
	}

	// Extract command.
	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text, " ", 2)
		// Strip @botname suffix from commands like /clear@herald_bot.
		cmd := strings.SplitN(parts[0], "@", 2)[0]
		in.Command = cmd
		if len(parts) > 1 {
			in.Text = strings.TrimSpace(parts[1])
		}
	}

	a.hub.In <- in
}

func (a *Adapter) dispatchOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.hub.Out:
			a.stopTyping(msg.ChatID)

			formatted := format.TelegramHTML(msg.Text)
			_, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID:    msg.ChatID,
				Text:      formatted,
				ParseMode: models.ParseModeHTML,
			})
			if err != nil {
				// Fallback: send as plain text if HTML parsing fails.
				log.Printf("send HTML message to chat %d failed, retrying as plain text: %v", msg.ChatID, err)
				_, err = a.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: msg.ChatID,
					Text:   msg.Text,
				})
				if err != nil {
					log.Printf("send message to chat %d: %v", msg.ChatID, err)
				}
			}
		}
	}
}

func (a *Adapter) dispatchTyping(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case chatID := <-a.hub.Typing:
			a.startTyping(ctx, chatID)
		}
	}
}

func (a *Adapter) startTyping(ctx context.Context, chatID int64) {
	a.mu.Lock()
	// Cancel any existing typing indicator for this chat.
	if cancel, ok := a.typing[chatID]; ok {
		cancel()
	}
	typingCtx, cancel := context.WithCancel(ctx)
	a.typing[chatID] = cancel
	a.mu.Unlock()

	go func() {
		// Send immediately.
		a.sendTypingAction(typingCtx, chatID)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				a.sendTypingAction(typingCtx, chatID)
			}
		}
	}()
}

func (a *Adapter) stopTyping(chatID int64) {
	a.mu.Lock()
	if cancel, ok := a.typing[chatID]; ok {
		cancel()
		delete(a.typing, chatID)
	}
	a.mu.Unlock()
}

func (a *Adapter) sendTypingAction(ctx context.Context, chatID int64) {
	_, err := a.bot.SendChatAction(ctx, &bot.SendChatActionParams{
		ChatID: chatID,
		Action: models.ChatActionTyping,
	})
	if err != nil {
		log.Printf("send typing action to chat %d: %v", chatID, err)
	}
}
