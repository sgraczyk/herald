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
	Status   string `json:"status"`
	Version  string `json:"version"`
	Uptime   string `json:"uptime"`
	Provider string `json:"provider"`
}

// Server serves the health endpoint.
type Server struct {
	port      int
	version   string
	startTime time.Time
	provider  string
}

// NewServer creates a health server.
func NewServer(port int, version string, startTime time.Time, provider string) *Server {
	return &Server{
		port:      port,
		version:   version,
		startTime: startTime,
		provider:  provider,
	}
}

// Run starts the HTTP server. It blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
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

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("health server error: %v", err)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := response{
		Status:   "ok",
		Version:  s.version,
		Uptime:   time.Since(s.startTime).Truncate(time.Second).String(),
		Provider: s.provider,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
