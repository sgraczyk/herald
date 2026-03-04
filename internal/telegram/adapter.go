package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sgraczyk/herald/internal/hub"
)

// Adapter connects Telegram to the Hub via long polling.
type Adapter struct {
	bot        *bot.Bot
	hub        *hub.Hub
	allowedIDs map[int64]bool
}

// New creates a new Telegram adapter.
func New(token string, h *hub.Hub, allowedUserIDs []int64) (*Adapter, error) {
	a := &Adapter{
		hub:        h,
		allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
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
	// Dispatch outgoing messages in background.
	go a.dispatchOut(ctx)

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

	cmd, arg := parseCommand(text)

	a.hub.In <- hub.InMessage{
		ChatID:  chatID,
		UserID:  userID,
		Text:    arg,
		Command: cmd,
	}
}

// parseCommand extracts a command and its argument from text.
// Returns ("", text) for regular messages, or (command, arg) for commands.
// Strips @botname suffixes from commands like /clear@herald_bot.
func parseCommand(text string) (cmd, arg string) {
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	parts := strings.SplitN(text, " ", 2)
	cmd = strings.SplitN(parts[0], "@", 2)[0]
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}
	return cmd, arg
}

func (a *Adapter) dispatchOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.hub.Out:
			_, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: msg.ChatID,
				Text:   msg.Text,
			})
			if err != nil {
				log.Printf("send message to chat %d: %v", msg.ChatID, err)
			}
		}
	}
}
