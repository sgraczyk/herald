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
	f.mu.RLock()
	providers := make([]LLMProvider, len(f.providers))
	copy(providers, f.providers)
	f.mu.RUnlock()

	var errs []string

	for _, p := range providers {
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
