package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Errorf("unexpected messages count: %d", len(req.Messages))
		}
		// Content is a string for text-only messages.
		content, ok := req.Messages[0].Content.(string)
		if !ok || content != "hello" {
			t.Errorf("unexpected messages: %+v", req.Messages)
		}

		json.NewEncoder(w).Encode(openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "hi there"}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "test-model", "test-key")
	got, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hi there" {
		t.Errorf("expected 'hi there', got %q", got)
	}
}

func TestOpenAIName(t *testing.T) {
	p := NewOpenAI("chutes", "http://localhost", "model", "key")
	if p.Name() != "chutes" {
		t.Errorf("expected 'chutes', got %q", p.Name())
	}
}

func TestOpenAIAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if errors.Is(err, ErrAuthFailure) {
		t.Error("429 should not be an auth failure")
	}
}

func TestOpenAIAuthError(t *testing.T) {
	codes := []int{http.StatusUnauthorized, http.StatusForbidden}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				w.Write([]byte(`{"error":"invalid api key"}`))
			}))
			defer srv.Close()

			p := NewOpenAI("test", srv.URL, "model", "key")
			_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
			if err == nil {
				t.Fatalf("expected error for %d response", code)
			}
			if !errors.Is(err, ErrAuthFailure) {
				t.Errorf("expected ErrAuthFailure for %d, got: %v", code, err)
			}
		})
	}
}

func TestOpenAIContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := p.Chat(ctx, []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

func TestOpenAIClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")
	p.client.Timeout = 100 * time.Millisecond

	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

func TestOpenAIOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", maxResponseSize+1)))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

func TestOpenAIOversizedErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(strings.Repeat("x", maxResponseSize+1)))
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected error for oversized error response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

func TestOpenAIEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openaiResponse{Choices: []openaiChoice{}})
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "model", "key")
	_, err := p.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestBuildOpenAIContentTextOnly(t *testing.T) {
	m := Message{Role: "user", Content: "hello"}
	got := buildOpenAIContent(m)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T", got)
	}
	if s != "hello" {
		t.Errorf("expected 'hello', got %q", s)
	}
}

func TestBuildOpenAIContentWithImage(t *testing.T) {
	m := Message{
		Role:    "user",
		Content: "describe this",
		Images:  []ImageData{{Base64: "abc123", MimeType: "image/jpeg"}},
	}
	got := buildOpenAIContent(m)
	parts, ok := got.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", got)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}

	// First part: text.
	if parts[0]["type"] != "text" {
		t.Errorf("expected text type, got %v", parts[0]["type"])
	}
	if parts[0]["text"] != "describe this" {
		t.Errorf("expected text content, got %v", parts[0]["text"])
	}

	// Second part: image_url.
	if parts[1]["type"] != "image_url" {
		t.Errorf("expected image_url type, got %v", parts[1]["type"])
	}
	imgURL := parts[1]["image_url"].(map[string]string)
	if imgURL["url"] != "data:image/jpeg;base64,abc123" {
		t.Errorf("unexpected image URL: %v", imgURL["url"])
	}
}

func TestOpenAIChatWithImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]any
		json.NewDecoder(r.Body).Decode(&raw)
		msgs := raw["messages"].([]any)
		msg := msgs[0].(map[string]any)

		// Content should be an array for image messages.
		content, ok := msg["content"].([]any)
		if !ok {
			t.Errorf("expected content array for image message, got %T", msg["content"])
		}
		if len(content) != 2 {
			t.Errorf("expected 2 content parts, got %d", len(content))
		}

		json.NewEncoder(w).Encode(openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "I see a cat"}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI("test", srv.URL, "test-model", "test-key")
	got, err := p.Chat(context.Background(), []Message{{
		Role:    "user",
		Content: "What's this?",
		Images:  []ImageData{{Base64: "abc", MimeType: "image/jpeg"}},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "I see a cat" {
		t.Errorf("expected 'I see a cat', got %q", got)
	}
}
