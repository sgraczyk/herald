package provider

import (
	"context"
	"fmt"
	"log"
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
	if len(f.providers) == 0 {
		return "", fmt.Errorf("no providers configured")
	}

	var errs []string

	for _, p := range f.providers {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("fallback aborted: %w", err)
		}

		result, err := p.Chat(ctx, messages)
		if err == nil {
			f.mu.Lock()
			f.active = p.Name()
			f.mu.Unlock()

			return result, nil
		}

		log.Printf("provider %s failed: %v", p.Name(), err)
		errs = append(errs, fmt.Sprintf("%s: %v", p.Name(), err))
	}

	return "", fmt.Errorf("all providers failed: %s", strings.Join(errs, "; "))
}

// Providers returns the list of configured providers.
func (f *Fallback) Providers() []LLMProvider {
	return f.providers
}
