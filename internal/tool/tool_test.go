package tool

import (
	"context"
	"testing"
)

// mockTool implements Tool for testing.
type mockTool struct {
	name   string
	desc   string
	params []Parameter
	result *Result
	err    error
}

func (m *mockTool) Name() string                   { return m.name }
func (m *mockTool) Description() string             { return m.desc }
func (m *mockTool) Parameters() []Parameter         { return m.params }
func (m *mockTool) Execute(_ context.Context, _ map[string]string) (*Result, error) {
	return m.result, m.err
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", desc: "A test tool"}

	if err := r.Register(tool); err != nil {
		t.Fatalf("Register() error: %v", err)
	}

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Get() returned false for registered tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("Get() name = %q, want %q", got.Name(), "test_tool")
	}
}

func TestRegistryGetMissing(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get() returned true for missing tool")
	}
}

func TestRegistryDuplicateErrors(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "dup"}
	if err := r.Register(tool); err != nil {
		t.Fatalf("first Register() error: %v", err)
	}
	if err := r.Register(tool); err == nil {
		t.Error("expected error on duplicate Register()")
	}
}

func TestRegistryAllStableOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "charlie"})
	r.Register(&mockTool{name: "alpha"})
	r.Register(&mockTool{name: "bravo"})

	all := r.All()
	if len(all) != 3 {
		t.Fatalf("All() returned %d tools, want 3", len(all))
	}
	if all[0].Name() != "alpha" || all[1].Name() != "bravo" || all[2].Name() != "charlie" {
		t.Errorf("All() not sorted: %s, %s, %s", all[0].Name(), all[1].Name(), all[2].Name())
	}
}

func TestRegistryAllEmpty(t *testing.T) {
	r := NewRegistry()
	all := r.All()
	if len(all) != 0 {
		t.Errorf("All() returned %d tools for empty registry, want 0", len(all))
	}
}
