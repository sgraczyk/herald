package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sgraczyk/herald/internal/format"
	"github.com/sgraczyk/herald/internal/hub"
)

// knownCommands is the set of commands handled by the agent loop.
// Unknown commands keep full original text so the LLM sees the user's intent.
var knownCommands = map[string]bool{
	"/clear":    true,
	"/model":    true,
	"/status":   true,
	"/remember": true,
	"/forget":   true,
	"/memories": true,
}

// Adapter connects Telegram to the Hub via long polling.
type Adapter struct {
	bot        *bot.Bot
	hub        *hub.Hub
	allowedIDs map[int64]bool

	mu      sync.Mutex
	typing  map[int64]context.CancelFunc // active typing indicators per chat
}

// New creates a new Telegram adapter.
// It returns an error if allowedUserIDs is empty, enforcing fail-closed access control.
func New(token string, h *hub.Hub, allowedUserIDs []int64) (*Adapter, error) {
	a := &Adapter{
		hub:        h,
		allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
		typing:     make(map[int64]context.CancelFunc),
	}

	for _, id := range allowedUserIDs {
		if id > 0 {
			a.allowedIDs[id] = true
		}
	}

	if len(a.allowedIDs) == 0 {
		return nil, fmt.Errorf("no valid allowed user IDs configured")
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
	if !a.allowedIDs[userID] {
		slog.Warn("rejected message from unauthorized user", slog.Int64("user_id", userID))
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	in := parseMessage(chatID, userID, text)
	a.hub.In <- in
}

// parseMessage builds an InMessage from raw text, extracting any command.
// Known commands get only the argument portion in Text; unknown commands
// keep the full original text so the LLM sees the user's intent.
func parseMessage(chatID, userID int64, text string) hub.InMessage {
	in := hub.InMessage{
		ChatID: chatID,
		UserID: userID,
		Text:   text,
	}

	if strings.HasPrefix(text, "/") {
		parts := strings.SplitN(text, " ", 2)
		// Strip @botname suffix from commands like /clear@herald_bot.
		cmd := strings.SplitN(parts[0], "@", 2)[0]
		in.Command = cmd
		// Only strip the command prefix for known commands so their
		// handlers receive the argument portion in Text. Unknown
		// commands keep the full original text for the LLM.
		if knownCommands[cmd] && len(parts) > 1 {
			in.Text = strings.TrimSpace(parts[1])
		}
	}

	return in
}

func (a *Adapter) dispatchOut(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-a.hub.Out:
			a.stopTyping(msg.ChatID)

			formatted := format.TelegramHTML(msg.Text)
			for _, chunk := range format.Split(formatted, 4096) {
				_, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID:    msg.ChatID,
					Text:      chunk,
					ParseMode: models.ParseModeHTML,
				})
				if err != nil {
					slog.Warn("send HTML message failed, retrying as plain text", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
					_, err = a.bot.SendMessage(ctx, &bot.SendMessageParams{
						ChatID: msg.ChatID,
						Text:   chunk,
					})
					if err != nil {
						slog.Error("send message failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
					}
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
	if err != nil && ctx.Err() == nil {
		slog.Debug("send typing action failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
	}
}
