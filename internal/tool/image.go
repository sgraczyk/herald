package tool

import (
	"context"
	"fmt"
)

// ImageProvider generates images from text prompts. This mirrors
// provider.ImageProvider to avoid a circular import.
type ImageProvider interface {
	Name() string
	Generate(ctx context.Context, prompt string) ([]byte, error)
}

// ImageTool wraps an ImageProvider as a Tool.
type ImageTool struct {
	provider ImageProvider
}

// NewImageTool creates a Tool that generates images via the given provider.
func NewImageTool(p ImageProvider) *ImageTool {
	return &ImageTool{provider: p}
}

// Name returns "generate_image".
func (t *ImageTool) Name() string { return "generate_image" }

// Description returns a description for the LLM.
func (t *ImageTool) Description() string {
	return "Generate an image from a text description. Use this when the user asks you to draw, create, generate, or make an image or picture."
}

// Parameters returns the tool's parameter definitions.
func (t *ImageTool) Parameters() []Parameter {
	return []Parameter{
		{
			Name:        "prompt",
			Type:        "string",
			Description: "A detailed description of the image to generate.",
			Required:    true,
		},
	}
}

// Execute generates an image from the prompt argument. Returns the image bytes
// in Result.Data and a confirmation message in Result.Text.
func (t *ImageTool) Execute(ctx context.Context, args map[string]string) (*Result, error) {
	prompt := args["prompt"]
	if prompt == "" {
		return nil, fmt.Errorf("missing required parameter: prompt")
	}

	data, err := t.provider.Generate(ctx, prompt)
	if err != nil {
		return nil, err
	}

	return &Result{
		Text: "Image generated successfully.",
		Data: data,
	}, nil
}
