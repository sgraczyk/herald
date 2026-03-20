// Package agent implements the core agent loop that processes messages and
// calls LLM providers.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sgraczyk/herald/internal/config"
	"github.com/sgraczyk/herald/internal/hub"
	"github.com/sgraczyk/herald/internal/metrics"
	"github.com/sgraczyk/herald/internal/provider"
	"github.com/sgraczyk/herald/internal/store"
)

// Loop reads messages from the hub, calls the provider, and writes responses back.
type Loop struct {
	hub                      *hub.Hub
	provider                 provider.LLMProvider
	extProvider              provider.LLMProvider // provider used for memory extraction and summarization
	imageProvider            provider.ImageProvider
	store                    *store.DB
	metrics                  *metrics.Metrics
	historyLimit             int
	historyTokenBudget       int
	maxArchivedConversations int
	summarize                bool
	streaming                bool
	systemPrompt             string
	statusMessages           *config.StatusMessages
	startTime                time.Time
	wg                       sync.WaitGroup
}

// NewLoop creates a new agent loop. If systemPrompt is empty, the default
// hardcoded prompt is used. The tokenBudget parameter controls the maximum
// estimated tokens for history; a negative value disables token-based trimming
// (zero is treated as "use default" by config loading). If m is nil, no
// metrics are recorded. When summarize is true, old messages are summarized
// before pruning. When streaming is true, providers that implement
// StreamingProvider are used for incremental response delivery. The
// maxArchived parameter limits how many archived conversations are kept per
// chat (0 disables pruning, keeping all archives). The imgProvider parameter
// is optional; when non-nil, the LLM is informed about the generate_image tool.
// The sm parameter provides configurable status messages; when nil, defaults
// from config loading are used.
func NewLoop(h *hub.Hub, p provider.LLMProvider, s *store.DB, historyLimit, tokenBudget, maxArchived int, summarize, streaming bool, systemPrompt string, m *metrics.Metrics, imgProvider provider.ImageProvider, sm *config.StatusMessages) *Loop {
	if sm == nil {
		sm = &config.StatusMessages{
			ImageGenerating: "Generating image...",
			ImageTimeout:    "Image generation took too long. Try a simpler prompt or try again shortly.",
			ImageAuthError:  "Image service configuration issue. The admin has been notified.",
			ImageGenericErr: "Failed to generate image. Please try again.",
			ImageTooLarge:   "Generated image is too large for Telegram.",
			ProvTimeout:     "Response took too long. Try a simpler question or try again shortly.",
			ProvAuthError:   "Service configuration issue. The admin has been notified.",
			ProvGenericErr:  "I'm temporarily unavailable. Please try again shortly.",
		}
	}
	return &Loop{
		hub:                      h,
		provider:                 p,
		extProvider:              pickExtractionProvider(p),
		imageProvider:            imgProvider,
		store:                    s,
		metrics:                  m,
		historyLimit:             historyLimit,
		historyTokenBudget:       tokenBudget,
		maxArchivedConversations: maxArchived,
		summarize:                summarize,
		streaming:                streaming,
		systemPrompt:             systemPrompt,
		statusMessages:           sm,
		startTime:                time.Now(),
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

// Wait blocks until all background operations (memory extraction, summarization) complete.
func (l *Loop) Wait() { l.wg.Wait() }

// Run starts the agent loop. It blocks until ctx is cancelled.
// On shutdown, it drains remaining messages from the hub with a 30-second
// grace period, then waits up to 10 seconds for in-flight memory extractions.
func (l *Loop) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			l.drainMessages()
			l.drainExtractions()
			return
		case msg := <-l.hub.In:
			l.handle(ctx, msg)
		}
	}
}

// drainMessages stops the hub from accepting new messages and processes
// any messages remaining in hub.In with a 30-second grace period.
func (l *Loop) drainMessages() {
	l.hub.StartDrain()

	pending := len(l.hub.In)
	slog.Info("draining", "pending", pending)

	if pending == 0 {
		return
	}

	drainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		select {
		case <-drainCtx.Done():
			slog.Warn("drain grace period expired", "remaining", len(l.hub.In))
			return
		case msg := <-l.hub.In:
			l.handle(drainCtx, msg)
			if len(l.hub.In) == 0 {
				return
			}
		}
	}
}

// drainExtractions waits for in-flight background operations to finish,
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
	case "/new":
		l.handleNew(msg)
	case "/conversations":
		l.handleConversations(msg)
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
	if err := l.store.ClearSummary(msg.ChatID); err != nil {
		slog.Error("clear summary failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
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

func (l *Loop) handleNew(msg hub.InMessage) {
	archived, err := l.store.ArchiveConversation(msg.ChatID)
	if err != nil {
		slog.Error("archive conversation failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to archive conversation."}
		return
	}
	if !archived {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "No active conversation to archive."}
		return
	}

	if l.maxArchivedConversations > 0 {
		if err := l.store.PruneArchived(msg.ChatID, l.maxArchivedConversations); err != nil {
			slog.Error("prune archives failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		}
	}

	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Conversation archived. Starting fresh."}
}

func (l *Loop) handleConversations(msg hub.InMessage) {
	if strings.TrimSpace(msg.Text) == "clear" {
		l.handleConversationsClear(msg)
		return
	}

	convs, err := l.store.ListArchived(msg.ChatID)
	if err != nil {
		slog.Error("list archived failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to list conversations."}
		return
	}
	if len(convs) == 0 {
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "No archived conversations."}
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Archived conversations (%d):\n", len(convs))
	b.WriteString("Use /conversations clear to remove all.\n")
	for _, c := range convs {
		preview := firstUserPreview(c.Messages)
		fmt.Fprintf(&b, "\n%s — %d msgs", c.Timestamp.Format("2006-01-02 15:04"), len(c.Messages))
		if preview != "" {
			fmt.Fprintf(&b, "\n  %s", preview)
		}
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: b.String()}
}

func (l *Loop) handleConversationsClear(msg hub.InMessage) {
	if err := l.store.ClearArchived(msg.ChatID); err != nil {
		slog.Error("clear archives failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "Failed to clear archives."}
		return
	}
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: "All archived conversations cleared."}
}

// firstUserPreview returns the first 50 characters of the first user message.
func firstUserPreview(msgs []provider.Message) string {
	for _, m := range msgs {
		if m.Role == "user" && m.Content != "" {
			runes := []rune(m.Content)
			if len(runes) > 50 {
				return string(runes[:50]) + "..."
			}
			return m.Content
		}
	}
	return ""
}

func (l *Loop) handleMessage(ctx context.Context, msg hub.InMessage) {
	if l.metrics != nil {
		l.metrics.IncReceived()
	}

	// Load history and memories.
	history, err := l.store.ListWithTokenBudget(msg.ChatID, l.historyLimit, l.historyTokenBudget)
	if err != nil {
		slog.Error("load history failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}
	memories, err := l.store.ListMemories(msg.ChatID)
	if err != nil {
		slog.Error("load memories failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
	}

	// Load conversation summary.
	var summary string
	if l.summarize {
		summary, err = l.store.GetSummary(msg.ChatID)
		if err != nil {
			slog.Error("load summary failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		}
	}

	// Signal typing indicator before calling the provider.
	l.hub.Typing <- msg.ChatID

	// Build messages and call provider.
	messages := buildMessages(history, memories, msg.Text, l.systemPrompt, summary, l.imageProvider != nil)

	// Inject document context as a system message just before the user message.
	if msg.Document != nil {
		docMsg := provider.Message{
			Role:    "system",
			Content: formatDocumentContext(msg.Document),
		}
		// Insert before the last element (the current user message).
		messages = append(messages[:len(messages)-1], append([]provider.Message{docMsg}, messages[len(messages)-1])...)
	}

	// Attach images to the current user message (last in the list).
	if len(msg.Images) > 0 {
		last := &messages[len(messages)-1]
		last.Images = make([]provider.ImageData, len(msg.Images))
		for i, img := range msg.Images {
			last.Images[i] = provider.ImageData{Base64: img.Base64, MimeType: img.MimeType}
		}
	}

	// Streaming path: try streaming if enabled, not draining, and active
	// provider supports it.
	if l.streaming && !l.hub.Draining() {
		if fb, ok := l.provider.(*provider.Fallback); ok {
			if sp, ok := fb.Active().(provider.StreamingProvider); ok {
				response, streamErr := l.handleStream(ctx, sp, messages, msg.ChatID)
				if streamErr == nil {
					// Check for image generation tool call in streamed response.
					if l.imageProvider != nil {
						if prompt, ok := parseImageToolCall(response); ok {
							l.handleImageGeneration(ctx, msg, prompt, response)
							return
						}
					}
					l.saveAndProcess(msg, response)
					return
				}
				slog.Warn("streaming failed, falling back to buffered",
					slog.Int64("chat_id", msg.ChatID),
					slog.String("error", streamErr.Error()),
				)
				// Signal adapter to delete in-progress message.
				l.hub.Stream <- hub.StreamUpdate{ChatID: msg.ChatID, Text: "", Done: true}
				// Fall through to buffered Chat().
			}
		}
	}

	response, err := l.provider.Chat(ctx, messages)
	if err != nil {
		if l.metrics != nil {
			l.metrics.IncFailed()
		}
		slog.Error("provider call failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		var errText string
		switch {
		case errors.Is(err, provider.ErrTimeout):
			errText = l.statusMessages.ProvTimeout
		case errors.Is(err, provider.ErrAuthFailure):
			errText = l.statusMessages.ProvAuthError
		default:
			errText = l.statusMessages.ProvGenericErr
		}
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: errText}
		return
	}

	// Check for image generation tool call.
	if l.imageProvider != nil {
		if prompt, ok := parseImageToolCall(response); ok {
			l.handleImageGeneration(ctx, msg, prompt, response)
			return
		}
	}

	l.saveAndRespond(msg, response)
}

// saveAndRespond saves messages to store, sends the response via hub.Out,
// and triggers background memory extraction and summarization.
func (l *Loop) saveAndRespond(msg hub.InMessage, response string) {
	l.saveAndProcess(msg, response)
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: response}
}

// saveAndProcess saves messages to store and triggers background memory
// extraction and summarization. Unlike saveAndRespond, it does NOT send the
// response to hub.Out — use this when the response was already delivered
// (e.g., via streaming).
func (l *Loop) saveAndProcess(msg hub.InMessage, response string) {
	// Save document as a system-role history message before the user message.
	if msg.Document != nil {
		docMsg := provider.Message{
			Role:      "system",
			Content:   formatDocumentContext(msg.Document),
			Timestamp: time.Now(),
		}
		if err := l.store.Append(msg.ChatID, docMsg, l.historyLimit); err != nil {
			slog.Error("save document message failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		}
	}

	// Save user message. Strip images — store text placeholder instead.
	userContent := msg.Text
	if len(msg.Images) > 0 {
		userContent = "[image] " + msg.Text
	}
	if msg.Document != nil {
		userContent = fmt.Sprintf("[document: %s] %s", msg.Document.Name, userContent)
	}
	userMsg := provider.Message{
		Role:      "user",
		Content:   userContent,
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

	if l.metrics != nil {
		l.metrics.IncSent()
	}

	// Summarize messages about to be pruned in the background.
	if l.summarize {
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in summarization", slog.Int64("chat_id", msg.ChatID), slog.Any("panic", r))
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			l.maybeSummarize(ctx, msg.ChatID)
		}()
	}

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

// handleStream calls ChatStream on the provider and sends streaming updates
// to the hub. Returns the complete response or an error.
func (l *Loop) handleStream(ctx context.Context, sp provider.StreamingProvider, messages []provider.Message, chatID int64) (string, error) {
	var accumulated strings.Builder
	lastEmit := time.Now()

	response, err := sp.ChatStream(ctx, messages, func(delta string) {
		accumulated.WriteString(delta)
		if time.Since(lastEmit) >= time.Second {
			l.hub.Stream <- hub.StreamUpdate{
				ChatID: chatID,
				Text:   accumulated.String() + "...",
				Done:   false,
			}
			lastEmit = time.Now()
		}
	})
	if err != nil {
		return "", err
	}

	// If the response contains an image tool call, delete the streamed
	// message (which would contain raw XML) instead of finalizing it.
	if l.imageProvider != nil {
		if _, ok := parseImageToolCall(response); ok {
			l.hub.Stream <- hub.StreamUpdate{ChatID: chatID, Text: "", Done: true}
			return response, nil
		}
	}

	// Send final update with the complete response (no "...").
	l.hub.Stream <- hub.StreamUpdate{
		ChatID: chatID,
		Text:   response,
		Done:   true,
	}

	return response, nil
}

// maxImageSize is the maximum image size Telegram accepts (20 MB).
const maxImageSize = 20 << 20

// imageToolCallRe matches the XML tool_use block for generate_image.
var imageToolCallRe = regexp.MustCompile(`(?s)<tool_use>\s*<name>generate_image</name>\s*<parameters>\s*<prompt>(.*?)</prompt>\s*</parameters>\s*</tool_use>`)

// parseImageToolCall checks if the LLM response contains a generate_image
// tool call and returns the prompt. Returns ("", false) if no tool call found.
func parseImageToolCall(response string) (string, bool) {
	matches := imageToolCallRe.FindStringSubmatch(response)
	if len(matches) < 2 {
		return "", false
	}
	prompt := strings.TrimSpace(matches[1])
	if prompt == "" {
		return "", false
	}
	return prompt, true
}

// handleImageGeneration sends a placeholder message, generates an image,
// and delivers it as a Telegram photo.
func (l *Loop) handleImageGeneration(ctx context.Context, msg hub.InMessage, prompt, llmResponse string) {
	slog.Info("image generation requested", slog.Int64("chat_id", msg.ChatID), slog.String("prompt", prompt))

	// Send placeholder and re-signal typing for the long generation call.
	l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: l.statusMessages.ImageGenerating}
	l.hub.Typing <- msg.ChatID

	// Generate image.
	imgBytes, err := l.imageProvider.Generate(ctx, prompt)
	if err != nil {
		if l.metrics != nil {
			l.metrics.IncFailed()
		}
		slog.Error("image generation failed", slog.Int64("chat_id", msg.ChatID), slog.String("error", err.Error()))
		var errText string
		switch {
		case errors.Is(err, provider.ErrTimeout):
			errText = l.statusMessages.ImageTimeout
		case errors.Is(err, provider.ErrAuthFailure):
			errText = l.statusMessages.ImageAuthError
		default:
			errText = l.statusMessages.ImageGenericErr
		}
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: errText}
		return
	}

	if len(imgBytes) > maxImageSize {
		slog.Error("generated image too large", slog.Int64("chat_id", msg.ChatID), slog.Int("size", len(imgBytes)))
		l.hub.Out <- hub.OutMessage{ChatID: msg.ChatID, Text: l.statusMessages.ImageTooLarge}
		return
	}

	// Save the tool call as the assistant response only after successful generation.
	l.saveAndProcess(msg, llmResponse)
	l.hub.Image <- hub.ImageMessage{ChatID: msg.ChatID, Data: imgBytes}
}

// isTrivialMessage returns true for messages too short to contain memorable content.
func isTrivialMessage(text string) bool {
	text = strings.TrimSpace(text)
	return len(text) < 10 || !strings.Contains(text, " ")
}

// formatDocumentContext formats a document attachment for injection into
// conversation context as a system message.
func formatDocumentContext(doc *hub.DocumentAttachment) string {
	var header, footer string
	if doc.Truncated {
		header = fmt.Sprintf("--- Document: %s (%d/%d pages shown) ---", doc.Name, doc.ShownPages, doc.Pages)
		omitted := doc.Pages - doc.ShownPages
		footer = fmt.Sprintf("--- End of document (%d pages omitted due to length) ---", omitted)
	} else {
		header = fmt.Sprintf("--- Document: %s (%d pages) ---", doc.Name, doc.Pages)
		footer = "--- End of document ---"
	}
	return header + "\n" + doc.Text + "\n" + footer
}

const extractionPrompt = `Extract notable facts, preferences, or personal details about the user from this exchange. Return ONLY a JSON array of short factual strings, or an empty array [] if nothing is worth remembering.

Rules:
- Only extract durable facts (preferences, background, habits), not transient conversation topics
- Keep each fact short and canonical (e.g., "prefers Go" not "the user mentioned they like Go")
- Do NOT extract what the assistant said, only what reveals something about the user

User: %s
Assistant: %s`

func (l *Loop) extractMemories(ctx context.Context, chatID int64, userText, assistantText string) {
	slog.Debug("memory extraction started", slog.Int64("chat_id", chatID))
	defer slog.Debug("memory extraction finished", slog.Int64("chat_id", chatID))

	prompt := fmt.Sprintf(extractionPrompt, userText, assistantText)
	msgs := []provider.Message{
		{Role: "system", Content: "Extract facts as a JSON array of short strings. Return only valid JSON, no explanation."},
		{Role: "user", Content: prompt},
	}

	resp, err := l.extProvider.Chat(ctx, msgs)
	if err != nil {
		if l.metrics != nil {
			l.metrics.IncExtractionFailure()
		}
		slog.Debug("extract memories failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		return
	}
	if l.metrics != nil {
		l.metrics.IncExtractionSuccess()
	}

	// Parse JSON array from response. The LLM may wrap it in markdown fences
	// or other text, so we scan for the first '[' and use json.Decoder.
	facts, err := parseFactsJSON(resp)
	if err != nil {
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

func (l *Loop) maybeSummarize(ctx context.Context, chatID int64) {
	if l.extProvider == nil {
		return
	}

	pending, err := l.store.PendingPrune(chatID, l.historyLimit)
	if err != nil {
		slog.Error("pending prune check failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		return
	}
	if len(pending) == 0 {
		return
	}

	existing, err := l.store.GetSummary(chatID)
	if err != nil {
		slog.Error("load existing summary failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		return
	}

	var b strings.Builder
	for _, m := range pending {
		content := m.Content
		// Replace large document text with a short placeholder for the summarizer.
		if m.Role == "system" && strings.HasPrefix(content, "--- Document:") {
			if idx := strings.Index(content, "\n"); idx > 0 {
				content = content[:idx]
			}
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Role, content)
	}

	sumCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	systemPrompt := "Summarize this conversation in 2-3 sentences. Preserve key facts, decisions, user preferences, and any commitments made. Be factual and concise. Do not add commentary."
	userContent := b.String()
	if existing != "" {
		systemPrompt = "You are updating a running conversation summary. Merge the existing summary with the new messages into a single coherent summary of 2-4 sentences. Preserve key facts, decisions, user preferences, and any commitments. Be factual and concise. Do not add commentary."
		userContent = fmt.Sprintf("Existing summary:\n%s\n\nNew messages:\n%s", existing, b.String())
	}

	msgs := []provider.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userContent},
	}

	summary, err := l.extProvider.Chat(sumCtx, msgs)
	if err != nil {
		slog.Error("summarization failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
		return
	}

	if err := l.store.SaveSummary(chatID, summary); err != nil {
		slog.Error("save summary failed", slog.Int64("chat_id", chatID), slog.String("error", err.Error()))
	}
}

// parseFactsJSON extracts a JSON string array from an LLM response that may
// contain surrounding text or markdown fences. It scans for the first '['
// and uses json.Decoder to parse the array.
func parseFactsJSON(resp string) ([]string, error) {
	idx := strings.Index(resp, "[")
	if idx < 0 {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	dec := json.NewDecoder(strings.NewReader(resp[idx:]))
	var raw []json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode JSON array: %w", err)
	}

	facts := make([]string, 0, len(raw))
	for _, r := range raw {
		var s string
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, fmt.Errorf("array element is not a string: %w", err)
		}
		facts = append(facts, s)
	}
	return facts, nil
}
