package hub

import "testing"

func TestNew(t *testing.T) {
	h := New()

	if h.In == nil {
		t.Fatal("In channel is nil")
	}
	if h.Out == nil {
		t.Fatal("Out channel is nil")
	}
	if cap(h.In) != 64 {
		t.Fatalf("In channel capacity = %d, want 64", cap(h.In))
	}
	if cap(h.Out) != 64 {
		t.Fatalf("Out channel capacity = %d, want 64", cap(h.Out))
	}
}
