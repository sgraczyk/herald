// Package metrics collects in-memory runtime statistics about Herald's operation.
// Counters reset on restart; the started_at field makes this explicit.
package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Metrics tracks counters for message flow, provider performance, and memory extraction.
type Metrics struct {
	mu        sync.Mutex
	startedAt time.Time

	// Global counters.
	messagesReceived int64
	responsesSent    int64
	messagesFailed   int64

	// Memory extraction counters.
	memoryExtractions        int64
	memoryExtractionFailures int64

	// Per-provider counters.
	providerCalls      map[string]int64
	providerErrors     map[string]int64
	providerLatencyMs  map[string]int64 // total latency in ms
	providerLatencyCnt map[string]int64 // number of latency observations
}

// New creates a Metrics instance with per-provider maps pre-initialized.
func New(providerNames []string) *Metrics {
	m := &Metrics{
		startedAt:          time.Now(),
		providerCalls:      make(map[string]int64, len(providerNames)),
		providerErrors:     make(map[string]int64, len(providerNames)),
		providerLatencyMs:  make(map[string]int64, len(providerNames)),
		providerLatencyCnt: make(map[string]int64, len(providerNames)),
	}
	for _, name := range providerNames {
		m.providerCalls[name] = 0
		m.providerErrors[name] = 0
		m.providerLatencyMs[name] = 0
		m.providerLatencyCnt[name] = 0
	}
	return m
}

// IncReceived increments the messages_received counter.
func (m *Metrics) IncReceived() {
	m.mu.Lock()
	m.messagesReceived++
	m.mu.Unlock()
}

// IncSent increments the responses_sent counter.
func (m *Metrics) IncSent() {
	m.mu.Lock()
	m.responsesSent++
	m.mu.Unlock()
}

// IncFailed increments the messages_failed counter.
func (m *Metrics) IncFailed() {
	m.mu.Lock()
	m.messagesFailed++
	m.mu.Unlock()
}

// IncProviderCall increments the provider_calls counter for the named provider.
func (m *Metrics) IncProviderCall(name string) {
	m.mu.Lock()
	m.providerCalls[name]++
	m.mu.Unlock()
}

// IncProviderError increments the provider_errors counter for the named provider.
func (m *Metrics) IncProviderError(name string) {
	m.mu.Lock()
	m.providerErrors[name]++
	m.mu.Unlock()
}

// ObserveLatency records a provider call duration for the named provider.
func (m *Metrics) ObserveLatency(name string, d time.Duration) {
	ms := d.Milliseconds()
	m.mu.Lock()
	m.providerLatencyMs[name] += ms
	m.providerLatencyCnt[name]++
	m.mu.Unlock()
}

// IncExtractionSuccess increments the memory_extraction_successes counter.
func (m *Metrics) IncExtractionSuccess() {
	m.mu.Lock()
	m.memoryExtractions++
	m.mu.Unlock()
}

// IncExtractionFailure increments the memory_extraction_failures counter.
func (m *Metrics) IncExtractionFailure() {
	m.mu.Lock()
	m.memoryExtractionFailures++
	m.mu.Unlock()
}

// Snapshot returns a JSON-safe copy of all current metrics.
func (m *Metrics) Snapshot() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	providerCalls := make(map[string]int64, len(m.providerCalls))
	for k, v := range m.providerCalls {
		providerCalls[k] = v
	}
	providerErrors := make(map[string]int64, len(m.providerErrors))
	for k, v := range m.providerErrors {
		providerErrors[k] = v
	}
	providerLatencyMs := make(map[string]int64, len(m.providerLatencyMs))
	for k, v := range m.providerLatencyMs {
		providerLatencyMs[k] = v
	}
	providerLatencyCnt := make(map[string]int64, len(m.providerLatencyCnt))
	for k, v := range m.providerLatencyCnt {
		providerLatencyCnt[k] = v
	}

	return map[string]any{
		"started_at":                  m.startedAt.UTC().Format(time.RFC3339),
		"uptime_seconds":              int64(time.Since(m.startedAt).Seconds()),
		"messages_received":           m.messagesReceived,
		"responses_sent":              m.responsesSent,
		"messages_failed":             m.messagesFailed,
		"provider_calls":              providerCalls,
		"provider_errors":             providerErrors,
		"provider_latency_ms_total":   providerLatencyMs,
		"provider_latency_ms_count":   providerLatencyCnt,
		"memory_extraction_successes":  m.memoryExtractions,
		"memory_extraction_failures":  m.memoryExtractionFailures,
	}
}

// LogSummary logs current counters via slog.Info.
func (m *Metrics) LogSummary() {
	snap := m.Snapshot()
	slog.Info("metrics summary",
		slog.Any("started_at", snap["started_at"]),
		slog.Any("uptime_seconds", snap["uptime_seconds"]),
		slog.Any("messages_received", snap["messages_received"]),
		slog.Any("responses_sent", snap["responses_sent"]),
		slog.Any("messages_failed", snap["messages_failed"]),
		slog.Any("provider_calls", snap["provider_calls"]),
		slog.Any("provider_errors", snap["provider_errors"]),
		slog.Any("provider_latency_ms_total", snap["provider_latency_ms_total"]),
		slog.Any("provider_latency_ms_count", snap["provider_latency_ms_count"]),
		slog.Any("memory_extraction_successes", snap["memory_extraction_successes"]),
		slog.Any("memory_extraction_failures", snap["memory_extraction_failures"]),
	)
}

// StartPeriodicLog starts a goroutine that calls LogSummary every interval.
// It stops when ctx is cancelled and logs a final summary before returning.
func (m *Metrics) StartPeriodicLog(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.LogSummary()
			case <-ctx.Done():
				m.LogSummary()
				return
			}
		}
	}()
}

// Handler returns an http.HandlerFunc that serves the metrics snapshot as JSON.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(m.Snapshot()); err != nil {
			slog.Error("encode metrics response failed", slog.String("error", err.Error()))
		}
	}
}
