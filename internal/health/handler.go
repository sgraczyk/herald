package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

type response struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	Uptime       string `json:"uptime"`
	Provider     string `json:"provider"`
	TokenExpires string `json:"token_expires,omitempty"`
}

// Server serves the health endpoint.
type Server struct {
	port         int
	version      string
	startTime    time.Time
	provider     string
	tokenExpires string
}

// NewServer creates a health server.
func NewServer(port int, version string, startTime time.Time, provider string, tokenExpires string) *Server {
	return &Server{
		port:         port,
		version:      version,
		startTime:    startTime,
		provider:     provider,
		tokenExpires: tokenExpires,
	}
}

// Start binds the listener and serves in the background. It returns an error
// if the port cannot be bound. The server shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)

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
			log.Printf("health server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := response{
		Status:       "ok",
		Version:      s.version,
		Uptime:       time.Since(s.startTime).Truncate(time.Second).String(),
		Provider:     s.provider,
		TokenExpires: s.tokenExpires,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
