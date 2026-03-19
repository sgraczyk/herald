// Package telegram connects Herald to the Telegram Bot API via long polling.
package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sgraczyk/herald/internal/document"
	"github.com/sgraczyk/herald/internal/format"
	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/provider"
)

// knownCommands is the set of commands handled by the agent loop.
// Keep in sync with the switch in agent.Loop.handle (internal/agent/loop.go).
// Unknown commands keep full original text so the LLM sees the user's intent.
var knownCommands = map[string]bool{
	"/clear":         true,
	"/model":         true,
	"/status":        true,
	"/remember":      true,
	"/forget":        true,
	"/memories":      true,
	"/new":           true,
	"/conversations": true,
}

// Adapter connects Telegram to the Hub via long polling.
type Adapter struct {
	bot        *bot.Bot
	hub        *hub.Hub
	allowedIDs map[int64]bool
	extractor  document.Extractor

	mu         sync.Mutex
	typing     map[int64]context.CancelFunc // active typing indicators per chat
	streamMsgs map[int64]int                // chatID -> Telegram message ID for in-progress stream
}

// New creates a new Telegram adapter.
// It returns an error if allowedUserIDs is empty, enforcing fail-closed access control.
func New(token string, h *hub.Hub, allowedUserIDs []int64, ext document.Extractor) (*Adapter, error) {
	a := &Adapter{
		hub:        h,
		allowedIDs: make(map[int64]bool, len(allowedUserIDs)),
		extractor:  ext,
		typing:     make(map[int64]context.CancelFunc),
		streamMsgs: make(map[int64]int),
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
	go a.dispatchStream(ctx)

	// Start long polling (blocks).
	a.bot.Start(ctx)
}

func (a *Adapter) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	if msg.From == nil {
		slog.Debug("ignoring message with nil From field", slog.Int64("chat_id", msg.Chat.ID))
		return
	}

	userID := msg.From.ID
	chatID := msg.Chat.ID

	// Reject unauthorized users.
	if !a.allowedIDs[userID] {
		slog.Warn("rejected message from unauthorized user", slog.Int64("user_id", userID))
		return
	}

	// Handle photo messages.
	if len(msg.Photo) > 0 {
		a.handlePhoto(ctx, b, msg, chatID, userID)
		return
	}

	// Handle document messages (PDF).
	if msg.Document != nil && msg.Document.MimeType == "application/pdf" {
		a.handleDocument(ctx, b, msg, chatID, userID)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	if a.hub.Draining() {
		slog.Debug("dropping message, hub is draining", slog.Int64("chat_id", chatID))
		return
	}

	in := parseMessage(chatID, userID, text)
	a.hub.In <- in
}

func (a *Adapter) handlePhoto(ctx context.Context, b *bot.Bot, msg *models.Message, chatID, userID int64) {
	// Select largest photo (last element in Telegram's PhotoSize array).
	photo := msg.Photo[len(msg.Photo)-1]

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: photo.FileID})
	if err != nil {
		slog.Error("get photo file failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download image.")
		return
	}

	fileURL := a.bot.FileDownloadLink(file)
	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, fileURL, nil)
	if err != nil {
		slog.Error("create download request failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download image.")
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("download photo failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download image.")
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		slog.Error("read photo data failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download image.")
		return
	}

	mimeType := http.DetectContentType(data)
	imgData, err := provider.PreprocessImage(data, mimeType)
	if err != nil {
		slog.Error("preprocess image failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to process image.")
		return
	}

	text := strings.TrimSpace(msg.Caption)
	if text == "" {
		text = "What's in this image?"
	}

	if a.hub.Draining() {
		slog.Debug("dropping photo message, hub is draining", slog.Int64("chat_id", chatID))
		return
	}

	a.hub.In <- hub.InMessage{
		ChatID: chatID,
		UserID: userID,
		Text:   text,
		Images: []hub.ImageAttachment{
			{Base64: imgData.Base64, MimeType: imgData.MimeType},
		},
	}
}

const maxDocumentSize = 10 << 20 // 10 MB

func (a *Adapter) handleDocument(ctx context.Context, b *bot.Bot, msg *models.Message, chatID, userID int64) {
	if msg.Document.FileSize > maxDocumentSize {
		a.sendError(ctx, chatID, "PDF too large (max 10 MB).")
		return
	}

	file, err := b.GetFile(ctx, &bot.GetFileParams{FileID: msg.Document.FileID})
	if err != nil {
		slog.Error("get document file failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	fileURL := a.bot.FileDownloadLink(file)
	dlCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, fileURL, nil)
	if err != nil {
		slog.Error("create document download request failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("download document failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDocumentSize))
	if err != nil {
		slog.Error("read document data failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		a.sendError(ctx, chatID, "Failed to download the file.")
		return
	}

	r := bytes.NewReader(data)
	doc, err := a.extractor.Extract(r, int64(len(data)), msg.Document.FileName)
	if err != nil {
		slog.Warn("document extraction failed",
			slog.Int64("chat_id", chatID),
			slog.String("file", msg.Document.FileName),
			slog.String("error", err.Error()),
		)
		a.sendError(ctx, chatID, documentErrorMessage(err))
		return
	}

	text := strings.TrimSpace(msg.Caption)
	if text == "" {
		text = "What's in this document?"
	}

	if a.hub.Draining() {
		slog.Debug("dropping document message, hub is draining", slog.Int64("chat_id", chatID))
		return
	}

	a.hub.In <- hub.InMessage{
		ChatID:   chatID,
		UserID:   userID,
		Text:     text,
		Document: doc,
	}
}

func documentErrorMessage(err error) string {
	switch {
	case errors.Is(err, document.ErrEncrypted):
		return "Sorry, I can't read encrypted PDFs."
	case errors.Is(err, document.ErrNoText), errors.Is(err, document.ErrGarbled):
		return "This PDF appears to be scanned/image-based. Text extraction isn't supported yet."
	case errors.Is(err, document.ErrMalformed):
		return "Couldn't process this PDF. The file may be corrupted."
	default:
		return "Couldn't process this PDF. Try a different file."
	}
}

func (a *Adapter) sendError(ctx context.Context, chatID int64, text string) {
	_, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	if err != nil {
		slog.Error("send error message failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
	}
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

func (a *Adapter) dispatchStream(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case update := <-a.hub.Stream:
			a.stopTyping(update.ChatID)

			a.mu.Lock()
			msgID, exists := a.streamMsgs[update.ChatID]
			a.mu.Unlock()

			// Error case: empty text + done means delete in-progress message.
			if update.Text == "" && update.Done {
				if exists {
					_, err := a.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
						ChatID:    update.ChatID,
						MessageID: msgID,
					})
					if err != nil {
						slog.Warn("delete stream message failed", slog.Int64("chat_id", update.ChatID), slog.String("error", err.Error()))
					}
					a.mu.Lock()
					delete(a.streamMsgs, update.ChatID)
					a.mu.Unlock()
				}
				continue
			}

			if !exists {
				// First update: send a new message (plain text, no ParseMode).
				sent, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
					ChatID: update.ChatID,
					Text:   update.Text,
				})
				if err != nil {
					slog.Warn("send stream message failed", slog.Int64("chat_id", update.ChatID), slog.String("error", err.Error()))
					continue
				}
				a.mu.Lock()
				a.streamMsgs[update.ChatID] = sent.ID
				a.mu.Unlock()
				msgID = sent.ID
			} else if !update.Done {
				// Mid-stream edit: plain text, no ParseMode.
				_, err := a.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
					ChatID:    update.ChatID,
					MessageID: msgID,
					Text:      update.Text,
				})
				if err != nil {
					slog.Debug("edit stream message failed", slog.Int64("chat_id", update.ChatID), slog.String("error", err.Error()))
				}
			}

			if update.Done {
				// Final edit: format with HTML.
				formatted := format.TelegramHTML(update.Text)
				chunks := format.Split(formatted, 4096)

				if len(chunks) == 1 {
					_, err := a.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
						ChatID:    update.ChatID,
						MessageID: msgID,
						Text:      chunks[0],
						ParseMode: models.ParseModeHTML,
					})
					if err != nil {
						slog.Warn("edit stream HTML failed, retrying plain text", slog.Int64("chat_id", update.ChatID), slog.String("error", err.Error()))
						a.bot.EditMessageText(ctx, &bot.EditMessageTextParams{
							ChatID:    update.ChatID,
							MessageID: msgID,
							Text:      update.Text,
						})
					}
				} else {
					// Multiple chunks: delete stream message and send fresh ones.
					a.bot.DeleteMessage(ctx, &bot.DeleteMessageParams{
						ChatID:    update.ChatID,
						MessageID: msgID,
					})
					for _, chunk := range chunks {
						_, err := a.bot.SendMessage(ctx, &bot.SendMessageParams{
							ChatID:    update.ChatID,
							Text:      chunk,
							ParseMode: models.ParseModeHTML,
						})
						if err != nil {
							slog.Warn("send stream chunk HTML failed, retrying plain text", slog.Int64("chat_id", update.ChatID), slog.String("error", err.Error()))
							a.bot.SendMessage(ctx, &bot.SendMessageParams{
								ChatID: update.ChatID,
								Text:   chunk,
							})
						}
					}
				}

				a.mu.Lock()
				delete(a.streamMsgs, update.ChatID)
				a.mu.Unlock()
			}
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
