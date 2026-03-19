// Package hub provides the message routing channels between the Telegram
// adapter and the agent loop.
package hub

import (
	"sync/atomic"

	"github.com/sgraczyk/herald/internal/document"
)

// ImageAttachment holds a base64-encoded image from Telegram.
type ImageAttachment struct {
	Base64   string
	MimeType string
}

// InMessage represents an incoming message to be processed.
type InMessage struct {
	ChatID  int64
	UserID  int64
	Text    string
	Command string           // e.g. "/clear", "/model", "/status" (empty for regular messages)
	Images   []ImageAttachment  // optional image attachments
	Document *document.Document // optional document attachment
}

// OutMessage represents an outgoing response to be sent.
type OutMessage struct {
	ChatID int64
	Text   string
}

// StreamUpdate represents a partial response update for in-place editing.
type StreamUpdate struct {
	ChatID int64
	Text   string // accumulated text so far (not just the delta)
	Done   bool   // final update — apply formatting, clean up state
}

// Hub routes messages between the Telegram adapter and the agent loop.
type Hub struct {
	In     chan InMessage
	Out    chan OutMessage
	Typing chan int64        // ChatID to send typing indicator for
	Stream chan StreamUpdate // streaming response updates

	draining atomic.Bool
}

// New creates a new Hub with buffered channels.
func New() *Hub {
	return &Hub{
		In:     make(chan InMessage, 64),
		Out:    make(chan OutMessage, 64),
		Typing: make(chan int64, 64),
		Stream: make(chan StreamUpdate, 64),
	}
}

// StartDrain signals the hub to stop accepting new incoming messages.
// It is safe to call from any goroutine.
func (h *Hub) StartDrain() {
	h.draining.Store(true)
}

// Draining reports whether the hub is in drain mode.
func (h *Hub) Draining() bool {
	return h.draining.Load()
}
