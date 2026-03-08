package provider

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"time"
)

// validateTimeout is the deadline for each provider health check.
const validateTimeout = 10 * time.Second

// ValidateProviders probes each provider and logs warnings for any that are
// unreachable. It never returns an error — startup is not blocked.
func ValidateProviders(ctx context.Context, providers []LLMProvider) {
	for _, p := range providers {
		switch v := p.(type) {
		case *OpenAI:
			validateOpenAI(ctx, v)
		case *Claude:
			validateClaude(ctx)
		default:
			slog.Warn("unknown provider type, skipping validation", slog.String("provider", p.Name()))
		}
	}
}

// validateOpenAI checks that the OpenAI-compatible endpoint is reachable by
// calling GET /models. A non-200 response or network error logs a warning.
func validateOpenAI(ctx context.Context, o *OpenAI) {
	ctx, cancel := context.WithTimeout(ctx, validateTimeout)
	defer cancel()

	url := o.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("provider validation failed",
			slog.String("provider", o.name),
			slog.String("error", fmt.Sprintf("create request: %v", err)))
		return
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		slog.Warn("provider unreachable",
			slog.String("provider", o.name),
			slog.String("url", url),
			slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		slog.Warn("provider auth failure",
			slog.String("provider", o.name),
			slog.Int("status", resp.StatusCode))
		return
	}

	if resp.StatusCode != http.StatusOK {
		slog.Warn("provider returned unexpected status",
			slog.String("provider", o.name),
			slog.Int("status", resp.StatusCode))
		return
	}

	slog.Info("provider reachable", slog.String("provider", o.name))
}

// validateClaude checks that the claude CLI binary exists on PATH.
func validateClaude(ctx context.Context) {
	path, err := exec.LookPath("claude")
	if err != nil {
		slog.Warn("claude CLI not found on PATH", slog.String("error", err.Error()))
		return
	}
	slog.Info("provider reachable", slog.String("provider", "claude"), slog.String("path", path))
}
