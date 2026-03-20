package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

	c := NewChutes(srv.URL, "test-key")

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

	c := NewChutes(srv.URL, "test-key")

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

	c := NewChutes(srv.URL, "bad-key")

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

	c := NewChutes(srv.URL, "test-key")

	_, err := c.Generate(context.Background(), "a cat")
	if err == nil {
		t.Fatal("expected error for empty response body")
	}
}
