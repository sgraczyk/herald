package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type stubName struct{ name string }

func (s *stubName) Name() string { return s.name }

type stubProvider struct {
	name       string
	authStatus string
}

func (s *stubProvider) Name() string       { return s.name }
func (s *stubProvider) AuthStatus() string { return s.authStatus }

func TestHandleHealth(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	srv := NewServer(0, "v0.1.0", start, &stubName{"claude-cli"}, nil, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Version != "v0.1.0" {
		t.Errorf("expected version 'v0.1.0', got %q", resp.Version)
	}
	if resp.Provider != "claude-cli" {
		t.Errorf("expected provider 'claude-cli', got %q", resp.Provider)
	}
	if resp.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
	if resp.TokenExpires != "" {
		t.Error("expected empty token_expires when not set")
	}
	if resp.ClaudeStatus != "" {
		t.Error("expected empty claude_status when provider is nil")
	}
}

func TestHandleHealthWithTokenExpiry(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	srv := NewServer(0, "v0.1.0", start, &stubName{"claude-cli"}, nil, "2027-03-05", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.TokenExpires != "2027-03-05" {
		t.Errorf("expected token_expires '2027-03-05', got %q", resp.TokenExpires)
	}
}

func TestHandleHealthWithClaudeAuthError(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	claude := &stubProvider{name: "claude", authStatus: "auth_error"}
	srv := NewServer(0, "v0.1.0", start, &stubName{"claude"}, claude, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ClaudeStatus != "auth_error" {
		t.Errorf("expected claude_status 'auth_error', got %q", resp.ClaudeStatus)
	}
}

func TestHandleHealthDynamicProviderName(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	np := &stubName{"provider-a"}
	srv := NewServer(0, "v0.1.0", start, np, nil, "", nil)

	// First request returns initial name.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.handleHealth(rec, req)

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Provider != "provider-a" {
		t.Errorf("expected provider 'provider-a', got %q", resp.Provider)
	}

	// Simulate provider switch.
	np.name = "provider-b"

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rec = httptest.NewRecorder()
	srv.handleHealth(rec, req)

	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Provider != "provider-b" {
		t.Errorf("expected provider 'provider-b', got %q", resp.Provider)
	}
}

func TestHandleHealthWithClaudeOK(t *testing.T) {
	start := time.Now().Add(-10 * time.Second)
	claude := &stubProvider{name: "claude", authStatus: "ok"}
	srv := NewServer(0, "v0.1.0", start, &stubName{"claude"}, claude, "", nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.handleHealth(rec, req)

	var resp response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ClaudeStatus != "ok" {
		t.Errorf("expected claude_status 'ok', got %q", resp.ClaudeStatus)
	}
}
