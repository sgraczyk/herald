// Package health serves an HTTP health check endpoint reporting Herald's status.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/sgraczyk/herald/internal/metrics"
)

// NameProvider returns a display name. It is called on every request so the
// value can change at runtime (e.g. after a provider switch).
type NameProvider interface {
	// Name returns the display name of the active provider.
	Name() string
}

// ProviderStatus provides dynamic provider information for the health endpoint.
type ProviderStatus interface {
	// Name returns the provider name.
	Name() string
	// AuthStatus returns the last known auth status ("ok", "auth_error", or "").
	AuthStatus() string
}

type response struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	Uptime       string `json:"uptime"`
	Provider     string `json:"provider"`
	ClaudeStatus string `json:"claude_status,omitempty"`
	TokenExpires string `json:"token_expires,omitempty"`
}

// Server serves the health and metrics endpoints.
type Server struct {
	port         int
	version      string
	startTime    time.Time
	provider     NameProvider
	claude       ProviderStatus
	tokenExpires string
	metrics      *metrics.Metrics
}

// NewServer creates a health server. If m is non-nil, a GET /metrics endpoint
// is registered alongside the health endpoint.
func NewServer(port int, version string, startTime time.Time, provider NameProvider, claude ProviderStatus, tokenExpires string, m *metrics.Metrics) *Server {
	return &Server{
		port:         port,
		version:      version,
		startTime:    startTime,
		provider:     provider,
		claude:       claude,
		tokenExpires: tokenExpires,
		metrics:      m,
	}
}

// Start binds the listener and serves in the background. It returns an error
// if the port cannot be bound. The server shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	if s.metrics != nil {
		mux.HandleFunc("GET /metrics", s.metrics.Handler())
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("health server listen: %w", err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("health server error", slog.String("error", err.Error()))
		}
	}()

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := response{
		Status:       "ok",
		Version:      s.version,
		Uptime:       time.Since(s.startTime).Truncate(time.Second).String(),
		Provider:     s.provider.Name(),
		TokenExpires: s.tokenExpires,
	}

	if s.claude != nil {
		if status := s.claude.AuthStatus(); status != "" {
			resp.ClaudeStatus = status
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
