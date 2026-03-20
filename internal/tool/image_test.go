package tool

import (
	"context"
	"fmt"
	"testing"
)

// mockImageProvider implements ImageProvider for testing.
type mockImageProvider struct {
	name string
	data []byte
	err  error
}

func (m *mockImageProvider) Name() string { return m.name }
func (m *mockImageProvider) Generate(_ context.Context, _ string) ([]byte, error) {
	return m.data, m.err
}

func TestImageToolName(t *testing.T) {
	it := NewImageTool(&mockImageProvider{name: "test"})
	if got := it.Name(); got != "generate_image" {
		t.Errorf("Name() = %q, want %q", got, "generate_image")
	}
}

func TestImageToolDescription(t *testing.T) {
	it := NewImageTool(&mockImageProvider{name: "test"})
	if got := it.Description(); got == "" {
		t.Error("Description() returned empty string")
	}
}

func TestImageToolParameters(t *testing.T) {
	it := NewImageTool(&mockImageProvider{name: "test"})
	params := it.Parameters()
	if len(params) != 1 {
		t.Fatalf("expected 1 parameter, got %d", len(params))
	}
	if params[0].Name != "prompt" {
		t.Errorf("parameter name = %q, want %q", params[0].Name, "prompt")
	}
	if !params[0].Required {
		t.Error("prompt parameter should be required")
	}
}

func TestImageToolExecuteSuccess(t *testing.T) {
	imgData := []byte("fake-png-data")
	it := NewImageTool(&mockImageProvider{name: "test", data: imgData})

	result, err := it.Execute(context.Background(), map[string]string{"prompt": "a cat"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if string(result.Data) != string(imgData) {
		t.Errorf("Data = %q, want %q", result.Data, imgData)
	}
	if result.Text == "" {
		t.Error("Text should be non-empty on success")
	}
}

func TestImageToolExecuteError(t *testing.T) {
	it := NewImageTool(&mockImageProvider{name: "test", err: fmt.Errorf("generation failed")})

	_, err := it.Execute(context.Background(), map[string]string{"prompt": "a cat"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestImageToolExecuteMissingPrompt(t *testing.T) {
	it := NewImageTool(&mockImageProvider{name: "test", data: []byte("data")})

	_, err := it.Execute(context.Background(), map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing prompt, got nil")
	}
}
