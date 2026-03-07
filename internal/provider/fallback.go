package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Fallback tries providers in order and returns the first successful response.
type Fallback struct {
	providers []LLMProvider

	mu     sync.RWMutex
	active string // name of the last successful provider
}

// NewFallback creates a fallback chain from the given providers.
func NewFallback(providers []LLMProvider) *Fallback {
	name := ""
	if len(providers) > 0 {
		name = providers[0].Name()
	}
	return &Fallback{
		providers: providers,
		active:    name,
	}
}

func (f *Fallback) Name() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.active
}

func (f *Fallback) Chat(ctx context.Context, messages []Message) (string, error) {
	f.mu.RLock()
	providers := make([]LLMProvider, len(f.providers))
	copy(providers, f.providers)
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
		result, err := p.Chat(ctx, messages)
		if err == nil {
			f.mu.Lock()
			f.active = p.Name()
			f.mu.Unlock()

			return result, nil
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
