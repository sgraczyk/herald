package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChutes_Generate(t *testing.T) {
	fakeImage := []byte("fake-jpeg-image-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/generate" {
			t.Errorf("expected path /generate, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content-type: %s", r.Header.Get("Content-Type"))
		}

		var req struct {
			InputArgs struct {
				Prompt string `json:"prompt"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
			} `json:"input_args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.InputArgs.Prompt != "a cute cat" {
			t.Errorf("expected prompt 'a cute cat', got %s", req.InputArgs.Prompt)
		}
		if req.InputArgs.Width != 1024 {
			t.Errorf("expected width 1024, got %d", req.InputArgs.Width)
		}
		if req.InputArgs.Height != 1024 {
			t.Errorf("expected height 1024, got %d", req.InputArgs.Height)
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(fakeImage)
	}))
	defer srv.Close()

	c := NewChutes("test", srv.URL, "test-key")

	result, err := c.Generate(context.Background(), "a cute cat")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(result) != string(fakeImage) {
		t.Errorf("expected %q, got %q", fakeImage, result)
	}
}

func TestChutes_GenerateAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "invalid prompt"}`))
	}))
	defer srv.Close()

	c := NewChutes("test", srv.URL, "test-key")

	_, err := c.Generate(context.Background(), "bad prompt")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestChutes_GenerateAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer srv.Close()

	c := NewChutes("test", srv.URL, "bad-key")

	_, err := c.Generate(context.Background(), "a cat")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !errors.Is(err, ErrAuthFailure) {
		t.Errorf("expected ErrAuthFailure, got %v", err)
	}
}

func TestChutes_GenerateEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		// Write nothing — empty body.
	}))
	defer srv.Close()

	c := NewChutes("test", srv.URL, "test-key")

	_, err := c.Generate(context.Background(), "a cat")
	if err == nil {
		t.Fatal("expected error for empty response body")
	}
}

// stubImageProvider is a minimal ImageProvider for fallback testing.
type stubImageProvider struct {
	name string
	data []byte
	err  error
}

func (s *stubImageProvider) Name() string { return s.name }
func (s *stubImageProvider) Generate(_ context.Context, _ string) ([]byte, error) {
	return s.data, s.err
}

func TestImageFallback_FirstSucceeds(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", data: []byte("img1")}
	p2 := &stubImageProvider{name: "p2", data: []byte("img2")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	result, err := fb.Generate(context.Background(), "cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "img1" {
		t.Errorf("expected img1, got %q", result)
	}
}

func TestImageFallback_FirstFailsSecondSucceeds(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", err: fmt.Errorf("p1 down")}
	p2 := &stubImageProvider{name: "p2", data: []byte("img2")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	result, err := fb.Generate(context.Background(), "cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "img2" {
		t.Errorf("expected img2, got %q", result)
	}
}

func TestImageFallback_AllFail(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", err: fmt.Errorf("p1 down")}
	p2 := &stubImageProvider{name: "p2", err: fmt.Errorf("p2 down")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	_, err := fb.Generate(context.Background(), "cat")
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}
	if !strings.Contains(err.Error(), "p1") || !strings.Contains(err.Error(), "p2") {
		t.Errorf("expected combined error with both provider names, got: %v", err)
	}
}

func TestImageFallback_PreservesAuthSentinel(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", err: fmt.Errorf("auth: %w", ErrAuthFailure)}
	p2 := &stubImageProvider{name: "p2", err: fmt.Errorf("p2 down")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	_, err := fb.Generate(context.Background(), "cat")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrAuthFailure) {
		t.Errorf("expected ErrAuthFailure sentinel, got: %v", err)
	}
}

func TestImageFallback_PreservesTimeoutSentinel(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", err: fmt.Errorf("slow: %w", ErrTimeout)}
	p2 := &stubImageProvider{name: "p2", err: fmt.Errorf("p2 down")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	_, err := fb.Generate(context.Background(), "cat")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout sentinel, got: %v", err)
	}
}

func TestImageFallback_SingleProvider(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", data: []byte("img1")}
	fb := NewImageFallback([]ImageProvider{p1})

	result, err := fb.Generate(context.Background(), "cat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "img1" {
		t.Errorf("expected img1, got %q", result)
	}
}

func TestImageFallback_AuthFailureFallsThrough(t *testing.T) {
	p1 := &stubImageProvider{name: "p1", err: fmt.Errorf("auth: %w", ErrAuthFailure)}
	p2 := &stubImageProvider{name: "p2", data: []byte("img2")}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	result, err := fb.Generate(context.Background(), "cat")
	if err != nil {
		t.Fatalf("expected fallback to succeed after auth failure, got: %v", err)
	}
	if string(result) != "img2" {
		t.Errorf("expected img2, got %q", result)
	}
}

func TestImageFallback_Name(t *testing.T) {
	p1 := &stubImageProvider{name: "z-image"}
	p2 := &stubImageProvider{name: "flux"}
	fb := NewImageFallback([]ImageProvider{p1, p2})

	if fb.Name() != "z-image" {
		t.Errorf("expected name 'z-image', got %q", fb.Name())
	}
}
