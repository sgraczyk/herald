package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubProvider struct {
	name     string
	response string
	err      error
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Chat(_ context.Context, _ []Message) (string, error) {
	return s.response, s.err
}

func TestFallbackFirstSuccess(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "primary", response: "ok"},
		&stubProvider{name: "secondary", response: "fallback"},
	})

	got, err := fb.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("expected 'ok', got %q", got)
	}
	if fb.Name() != "primary" {
		t.Errorf("expected active 'primary', got %q", fb.Name())
	}
}

func TestFallbackToSecond(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "primary", err: fmt.Errorf("down")},
		&stubProvider{name: "secondary", response: "fallback"},
	})

	got, err := fb.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback" {
		t.Errorf("expected 'fallback', got %q", got)
	}
	if fb.Name() != "secondary" {
		t.Errorf("expected active 'secondary', got %q", fb.Name())
	}
}

func TestFallbackAllFail(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "a", err: fmt.Errorf("fail-a")},
		&stubProvider{name: "b", err: fmt.Errorf("fail-b")},
	})

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestFallbackSetActive(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "a", response: "from-a"},
		&stubProvider{name: "b", response: "from-b"},
	})

	if err := fb.SetActive("b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fb.Name() != "b" {
		t.Errorf("expected active 'b', got %q", fb.Name())
	}

	got, _ := fb.Chat(context.Background(), nil)
	if got != "from-b" {
		t.Errorf("expected 'from-b', got %q", got)
	}
}

func TestFallbackSetActiveUnknown(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "a"},
	})

	if err := fb.SetActive("nonexistent"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestFallbackAuthErrorPropagated(t *testing.T) {
	authErr := fmt.Errorf("claude: token expired: %w", ErrAuthFailure)
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", err: authErr},
		&stubProvider{name: "backup", err: fmt.Errorf("also down")},
	})

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !errors.Is(err, ErrAuthFailure) {
		t.Errorf("expected ErrAuthFailure in error chain, got: %v", err)
	}
}

func TestFallbackAuthErrorFallsThrough(t *testing.T) {
	authErr := fmt.Errorf("claude: token expired: %w", ErrAuthFailure)
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", err: authErr},
		&stubProvider{name: "backup", response: "fallback ok"},
	})

	got, err := fb.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "fallback ok" {
		t.Errorf("expected 'fallback ok', got %q", got)
	}
	if fb.Name() != "backup" {
		t.Errorf("expected active 'backup', got %q", fb.Name())
	}
}

func TestFallbackTimeoutPropagated(t *testing.T) {
	timeoutErr := fmt.Errorf("execute claude: %w: deadline exceeded", ErrTimeout)
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", err: timeoutErr},
		&stubProvider{name: "backup", err: fmt.Errorf("also down")},
	})

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout in error chain, got: %v", err)
	}
}

func TestFallbackAuthTakesPrecedenceOverTimeout(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", err: fmt.Errorf("execute claude: %w: deadline exceeded", ErrTimeout)},
		&stubProvider{name: "backup", err: fmt.Errorf("bad key: %w", ErrAuthFailure)},
	})

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !errors.Is(err, ErrAuthFailure) {
		t.Errorf("expected ErrAuthFailure to take precedence, got: %v", err)
	}
	if errors.Is(err, ErrTimeout) {
		t.Error("expected ErrTimeout NOT to be in error chain when auth error is present")
	}
}

func TestFallbackProviders(t *testing.T) {
	providers := []LLMProvider{
		&stubProvider{name: "a"},
		&stubProvider{name: "b"},
	}
	fb := NewFallback(providers)

	got := fb.Providers()
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	if got[0].Name() != "a" || got[1].Name() != "b" {
		t.Errorf("unexpected provider order: %s, %s", got[0].Name(), got[1].Name())
	}
}

func TestFallbackImageRoutesToOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openaiResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "I see an image"}}},
		})
	}))
	defer srv.Close()

	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", response: "from claude"},
		NewOpenAI("openai", srv.URL, "model", "key"),
	})

	// Image message — claude should be skipped, openai should be tried.
	msgs := []Message{{Role: "user", Content: "test", Images: []ImageData{{Base64: "abc", MimeType: "image/jpeg"}}}}
	got, err := fb.Chat(context.Background(), msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "I see an image" {
		t.Errorf("expected 'I see an image', got %q", got)
	}
	if fb.Name() != "openai" {
		t.Errorf("expected active 'openai', got %q", fb.Name())
	}
}

func TestFallbackImageNoVisionProvider(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude"},
	})

	msgs := []Message{{Role: "user", Content: "test", Images: []ImageData{{Base64: "abc", MimeType: "image/jpeg"}}}}
	_, err := fb.Chat(context.Background(), msgs)
	if err == nil {
		t.Fatal("expected error when no vision provider available")
	}
	if err.Error() != "no vision-capable provider configured" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFallbackTextUnaffectedByImageRouting(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", response: "from claude"},
		NewOpenAI("openai", "http://invalid", "model", "key"),
	})

	// Text-only message should use normal fallback (claude first).
	got, err := fb.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from claude" {
		t.Errorf("expected 'from claude', got %q", got)
	}
}
