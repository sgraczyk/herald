package agent

import (
	"testing"
	"time"

	"github.com/sgraczyk/herald/internal/provider"
)

func TestBuildMessages(t *testing.T) {
	history := []provider.Message{
		{Role: "user", Content: "hello", Timestamp: time.Now()},
		{Role: "assistant", Content: "hi there", Timestamp: time.Now()},
	}

	msgs := buildMessages(history, "how are you?")

	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4", len(msgs))
	}

	if msgs[0].Role != "system" || msgs[0].Content != systemPrompt {
		t.Errorf("first message should be system prompt, got role=%q", msgs[0].Role)
	}

	if msgs[1].Role != "user" || msgs[1].Content != "hello" {
		t.Errorf("second message should be first history entry, got %q", msgs[1].Content)
	}

	if msgs[2].Role != "assistant" || msgs[2].Content != "hi there" {
		t.Errorf("third message should be second history entry, got %q", msgs[2].Content)
	}

	if msgs[3].Role != "user" || msgs[3].Content != "how are you?" {
		t.Errorf("last message should be current user input, got role=%q content=%q", msgs[3].Role, msgs[3].Content)
	}
}

func TestBuildMessagesEmptyHistory(t *testing.T) {
	msgs := buildMessages(nil, "first message")

	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Errorf("first message should be system prompt")
	}

	if msgs[1].Role != "user" || msgs[1].Content != "first message" {
		t.Errorf("second message should be user input")
	}
}
