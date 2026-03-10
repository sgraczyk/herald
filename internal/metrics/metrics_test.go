package metrics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestIncrement(t *testing.T) {
	m := New([]string{"claude", "chutes"})

	m.IncReceived()
	m.IncReceived()
	m.IncSent()
	m.IncFailed()

	m.IncProviderCall("claude")
	m.IncProviderCall("claude")
	m.IncProviderCall("chutes")
	m.IncProviderError("chutes")

	m.ObserveLatency("claude", 100*time.Millisecond)
	m.ObserveLatency("claude", 200*time.Millisecond)

	m.IncExtraction()
	m.IncExtraction()
	m.IncExtractionFailure()

	snap := m.Snapshot()

	if got := snap["messages_received"].(int64); got != 2 {
		t.Errorf("messages_received = %d, want 2", got)
	}
	if got := snap["responses_sent"].(int64); got != 1 {
		t.Errorf("responses_sent = %d, want 1", got)
	}
	if got := snap["messages_failed"].(int64); got != 1 {
		t.Errorf("messages_failed = %d, want 1", got)
	}

	calls := snap["provider_calls"].(map[string]int64)
	if got := calls["claude"]; got != 2 {
		t.Errorf("provider_calls[claude] = %d, want 2", got)
	}
	if got := calls["chutes"]; got != 1 {
		t.Errorf("provider_calls[chutes] = %d, want 1", got)
	}

	errs := snap["provider_errors"].(map[string]int64)
	if got := errs["chutes"]; got != 1 {
		t.Errorf("provider_errors[chutes] = %d, want 1", got)
	}

	latTotal := snap["provider_latency_ms_total"].(map[string]int64)
	if got := latTotal["claude"]; got != 300 {
		t.Errorf("provider_latency_ms_total[claude] = %d, want 300", got)
	}

	latCount := snap["provider_latency_ms_count"].(map[string]int64)
	if got := latCount["claude"]; got != 2 {
		t.Errorf("provider_latency_ms_count[claude] = %d, want 2", got)
	}

	if got := snap["memory_extractions"].(int64); got != 2 {
		t.Errorf("memory_extractions = %d, want 2", got)
	}
	if got := snap["memory_extraction_failures"].(int64); got != 1 {
		t.Errorf("memory_extraction_failures = %d, want 1", got)
	}
}

func TestSnapshotContainsUptime(t *testing.T) {
	m := New(nil)
	snap := m.Snapshot()

	if _, ok := snap["started_at"]; !ok {
		t.Error("snapshot missing started_at")
	}
	if _, ok := snap["uptime_seconds"]; !ok {
		t.Error("snapshot missing uptime_seconds")
	}
}

func TestSnapshotIsolation(t *testing.T) {
	m := New([]string{"p1"})
	m.IncProviderCall("p1")

	snap := m.Snapshot()
	calls := snap["provider_calls"].(map[string]int64)
	calls["p1"] = 999

	snap2 := m.Snapshot()
	calls2 := snap2["provider_calls"].(map[string]int64)
	if calls2["p1"] != 1 {
		t.Errorf("snapshot not isolated: provider_calls[p1] = %d, want 1", calls2["p1"])
	}
}

func TestConcurrentIncrements(t *testing.T) {
	m := New([]string{"p1"})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.IncReceived()
			m.IncSent()
			m.IncFailed()
			m.IncProviderCall("p1")
			m.IncProviderError("p1")
			m.ObserveLatency("p1", time.Millisecond)
			m.IncExtraction()
			m.IncExtractionFailure()
		}()
	}
	wg.Wait()

	snap := m.Snapshot()
	if got := snap["messages_received"].(int64); got != 100 {
		t.Errorf("messages_received = %d, want 100", got)
	}
	if got := snap["responses_sent"].(int64); got != 100 {
		t.Errorf("responses_sent = %d, want 100", got)
	}
	calls := snap["provider_calls"].(map[string]int64)
	if got := calls["p1"]; got != 100 {
		t.Errorf("provider_calls[p1] = %d, want 100", got)
	}
}

func TestHandler(t *testing.T) {
	m := New([]string{"claude"})
	m.IncReceived()
	m.IncProviderCall("claude")

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler()(w, r)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var result map[string]any
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got := result["messages_received"].(float64); got != 1 {
		t.Errorf("messages_received = %v, want 1", got)
	}
}
