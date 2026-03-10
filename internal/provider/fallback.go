package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/sgraczyk/herald/internal/metrics"
)

// Fallback tries providers in order and returns the first successful response.
type Fallback struct {
	providers  []LLMProvider
	maxRetries int
	metrics    *metrics.Metrics

	mu     sync.RWMutex
	active string // name of the currently active provider
}

// NewFallback creates a fallback chain from the given providers.
// maxRetries controls how many times a transient error is retried per provider
// (0 disables retry). If m is nil, no metrics are recorded.
func NewFallback(providers []LLMProvider, maxRetries int, m *metrics.Metrics) *Fallback {
	name := ""
	if len(providers) > 0 {
		name = providers[0].Name()
	}
	return &Fallback{
		providers:  providers,
		maxRetries: maxRetries,
		metrics:    m,
		active:     name,
	}
}

// Name returns the name of the currently active provider.
func (f *Fallback) Name() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.active
}

// Chat tries each provider in order and returns the first successful response.
func (f *Fallback) Chat(ctx context.Context, messages []Message) (string, error) {
	f.mu.RLock()
	providers := make([]LLMProvider, len(f.providers))
	copy(providers, f.providers)
	maxRetries := f.maxRetries
	f.mu.RUnlock()

	// If any message has images, filter to vision-capable providers only.
	if hasImages(messages) {
		providers = filterVisionProviders(providers)
		if len(providers) == 0 {
			return "", fmt.Errorf("no vision-capable provider configured")
		}
	}

	var errs []string
	var hasAuthErr, hasTimeout bool

	for _, p := range providers {
		// Skip retry for claude-cli provider (its errors are opaque).
		retries := maxRetries
		if _, ok := p.(*Claude); ok {
			retries = 0
		}

		var result string
		var err error
		for attempt := 0; attempt <= retries; attempt++ {
			start := time.Now()
			result, err = p.Chat(ctx, messages)
			elapsed := time.Since(start)
			if err == nil {
				if f.metrics != nil {
					f.metrics.IncProviderCall(p.Name())
					f.metrics.ObserveLatency(p.Name(), elapsed)
				}
				f.mu.Lock()
				f.active = p.Name()
				f.mu.Unlock()

				return result, nil
			}

			if attempt < retries && isTransient(err) {
				delay := time.Second * time.Duration(1<<uint(attempt))
				slog.Warn("retrying transient error",
					slog.String("provider", p.Name()),
					slog.Int("attempt", attempt+1),
					slog.Duration("backoff", delay),
					slog.String("error", err.Error()),
				)
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					return "", ctx.Err()
				}
				continue
			}

			break
		}

		if f.metrics != nil {
			f.metrics.IncProviderError(p.Name())
		}
		if errors.Is(err, ErrAuthFailure) {
			hasAuthErr = true
			slog.Warn("provider auth failure", slog.String("provider", p.Name()))
		} else if errors.Is(err, ErrTimeout) {
			hasTimeout = true
			slog.Warn("provider timed out", slog.String("provider", p.Name()))
		} else {
			slog.Warn("provider failed", slog.String("provider", p.Name()), slog.String("error", err.Error()))
		}
		errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
	}

	combined := fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
	switch {
	case hasAuthErr:
		return "", fmt.Errorf("%w: %w", combined, ErrAuthFailure)
	case hasTimeout:
		return "", fmt.Errorf("%w: %w", combined, ErrTimeout)
	default:
		return "", combined
	}
}

// isTransient returns true if the error is likely transient and worth retrying.
// It checks for net.Error (timeouts, connection errors), ErrTimeout, and
// HTTP 5xx status codes via HTTPStatusError.
func isTransient(err error) bool {
	if errors.Is(err, ErrTimeout) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		return httpErr.Code >= 500 && httpErr.Code < 600
	}

	return false
}

// SetActive reorders the provider list so the named provider is tried first.
func (f *Fallback) SetActive(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, p := range f.providers {
		if strings.EqualFold(p.Name(), name) {
			// Move selected provider to front.
			f.providers[0], f.providers[i] = f.providers[i], f.providers[0]
			f.active = p.Name()
			return nil
		}
	}
	return fmt.Errorf("unknown provider %q", name)
}

// Providers returns a copy of the configured providers list.
func (f *Fallback) Providers() []LLMProvider {
	f.mu.RLock()
	defer f.mu.RUnlock()
	result := make([]LLMProvider, len(f.providers))
	copy(result, f.providers)
	return result
}

// hasImages returns true if any message contains image data.
func hasImages(messages []Message) bool {
	for _, m := range messages {
		if len(m.Images) > 0 {
			return true
		}
	}
	return false
}

// filterVisionProviders returns only providers that support image input.
// Currently only *OpenAI providers support vision.
func filterVisionProviders(providers []LLMProvider) []LLMProvider {
	var vision []LLMProvider
	for _, p := range providers {
		if _, ok := p.(*OpenAI); ok {
			vision = append(vision, p)
		}
	}
	return vision
}
