package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateOpenAISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("expected /models, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	o := NewOpenAI("test", srv.URL, "model", "test-key")
	// Should not panic or log errors.
	validateOpenAI(context.Background(), o)
}

func TestValidateOpenAIAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	o := NewOpenAI("test", srv.URL, "model", "bad-key")
	// Should log warning but not panic.
	validateOpenAI(context.Background(), o)
}

func TestValidateOpenAIUnreachable(t *testing.T) {
	o := NewOpenAI("test", "http://127.0.0.1:1", "model", "key")
	// Should log warning but not panic.
	validateOpenAI(context.Background(), o)
}

func TestValidateOpenAIServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	o := NewOpenAI("test", srv.URL, "model", "key")
	// Should log warning but not panic.
	validateOpenAI(context.Background(), o)
}

func TestValidateProvidersCallsBoth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	providers := []LLMProvider{
		NewClaude(),
		NewOpenAI("test", srv.URL, "model", "key"),
	}

	// Should not panic. Claude validation may warn if CLI not on PATH.
	ValidateProviders(context.Background(), providers)
}
