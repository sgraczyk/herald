// Package hub provides the message routing channels between the Telegram
// adapter and the agent loop.
package hub

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
	Images  []ImageAttachment // optional image attachments
}

// OutMessage represents an outgoing response to be sent.
type OutMessage struct {
	ChatID int64
	Text   string
}

// Hub routes messages between the Telegram adapter and the agent loop.
type Hub struct {
	In     chan InMessage
	Out    chan OutMessage
	Typing chan int64 // ChatID to send typing indicator for
}

// New creates a new Hub with buffered channels.
func New() *Hub {
	return &Hub{
		In:     make(chan InMessage, 64),
		Out:    make(chan OutMessage, 64),
		Typing: make(chan int64, 64),
	}
}
