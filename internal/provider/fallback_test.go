package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
	}, 0)

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
	}, 0)

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
	}, 0)

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
}

func TestFallbackSetActive(t *testing.T) {
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "a", response: "from-a"},
		&stubProvider{name: "b", response: "from-b"},
	}, 0)

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
	}, 0)

	if err := fb.SetActive("nonexistent"); err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestFallbackAuthErrorPropagated(t *testing.T) {
	authErr := fmt.Errorf("claude: token expired: %w", ErrAuthFailure)
	fb := NewFallback([]LLMProvider{
		&stubProvider{name: "claude", err: authErr},
		&stubProvider{name: "backup", err: fmt.Errorf("also down")},
	}, 0)

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
	}, 0)

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
	}, 0)

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
	}, 0)

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
	fb := NewFallback(providers, 0)

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
	}, 0)

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
	}, 0)

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
	}, 0)

	// Text-only message should use normal fallback (claude first).
	got, err := fb.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from claude" {
		t.Errorf("expected 'from claude', got %q", got)
	}
}

// countingProvider tracks call count and returns an error for the first N calls.
type countingProvider struct {
	name      string
	failCount int // number of initial calls that fail
	failErr   error
	response  string
	calls     int
}

func (c *countingProvider) Name() string { return c.name }
func (c *countingProvider) Chat(_ context.Context, _ []Message) (string, error) {
	c.calls++
	if c.calls <= c.failCount {
		return "", c.failErr
	}
	return c.response, nil
}

func TestRetryTransientError(t *testing.T) {
	p := &countingProvider{
		name:      "openai",
		failCount: 1,
		failErr:   fmt.Errorf("API error (status 503): service unavailable"),
		response:  "ok",
	}
	fb := NewFallback([]LLMProvider{p}, 2)

	got, err := fb.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("expected 'ok', got %q", got)
	}
	if p.calls != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", p.calls)
	}
}

func TestNoRetryNonTransientError(t *testing.T) {
	p := &countingProvider{
		name:      "openai",
		failCount: 10,
		failErr:   fmt.Errorf("bad request: %w", ErrAuthFailure),
		response:  "ok",
	}
	fb := NewFallback([]LLMProvider{p}, 2)

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if p.calls != 1 {
		t.Errorf("expected 1 call (no retry for non-transient), got %d", p.calls)
	}
}

func TestRetryDisabledWithZero(t *testing.T) {
	p := &countingProvider{
		name:      "openai",
		failCount: 1,
		failErr:   fmt.Errorf("API error (status 503): service unavailable"),
		response:  "ok",
	}
	fb := NewFallback([]LLMProvider{p}, 0)

	_, err := fb.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error with retry disabled")
	}
	if p.calls != 1 {
		t.Errorf("expected 1 call (retry disabled), got %d", p.calls)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	p := &countingProvider{
		name:      "openai",
		failCount: 10,
		failErr:   fmt.Errorf("API error (status 500): internal error"),
		response:  "ok",
	}
	fb := NewFallback([]LLMProvider{p}, 5)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately so the backoff select picks up ctx.Done().
	cancel()

	_, err := fb.Chat(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestRetryBackoffDuration(t *testing.T) {
	p := &countingProvider{
		name:      "openai",
		failCount: 2,
		failErr:   fmt.Errorf("API error (status 502): bad gateway"),
		response:  "ok",
	}
	fb := NewFallback([]LLMProvider{p}, 3)

	start := time.Now()
	got, err := fb.Chat(context.Background(), nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Errorf("expected 'ok', got %q", got)
	}
	// Backoff: 1s + 2s = 3s minimum.
	if elapsed < 3*time.Second {
		t.Errorf("expected at least 3s backoff, got %v", elapsed)
	}
	if p.calls != 3 {
		t.Errorf("expected 3 calls, got %d", p.calls)
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		transient bool
	}{
		{"ErrTimeout", fmt.Errorf("wrapped: %w", ErrTimeout), true},
		{"status 500", fmt.Errorf("API error (status 500): oops"), true},
		{"status 502", fmt.Errorf("API error (status 502): bad gw"), true},
		{"status 503", fmt.Errorf("API error (status 503): unavail"), true},
		{"status 504", fmt.Errorf("API error (status 504): timeout"), true},
		{"status 400", fmt.Errorf("API error (status 400): bad req"), false},
		{"status 429", fmt.Errorf("API error (status 429): rate limit"), false},
		{"ErrAuthFailure", ErrAuthFailure, false},
		{"generic error", fmt.Errorf("something broke"), false},
		{"net.Error timeout", &netTimeoutErr{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTransient(tt.err)
			if got != tt.transient {
				t.Errorf("isTransient(%v) = %v, want %v", tt.err, got, tt.transient)
			}
		})
	}
}

// netTimeoutErr implements net.Error with Timeout() == true.
type netTimeoutErr struct{}

func (e *netTimeoutErr) Error() string   { return "network timeout" }
func (e *netTimeoutErr) Timeout() bool   { return true }
func (e *netTimeoutErr) Temporary() bool { return true }

// Ensure netTimeoutErr satisfies net.Error.
var _ net.Error = (*netTimeoutErr)(nil)
