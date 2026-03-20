package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ImageProvider generates images from text prompts.
type ImageProvider interface {
	// Name returns the provider's identifier.
	Name() string
	// Generate creates an image from a text prompt and returns the raw image bytes.
	Generate(ctx context.Context, prompt string) ([]byte, error)
}

// chutesTimeout is the HTTP client timeout for Chutes.ai image generation.
const chutesTimeout = 60 * time.Second

// maxImageResponseSize is the maximum allowed image response body size (20 MB).
// This matches Telegram's sendPhoto file size limit and is intentionally
// separate from maxResponseSize which caps LLM text responses.
const maxImageResponseSize = 20 << 20

// Chutes generates images using the Chutes.ai API (e.g. FLUX.1-schnell).
type Chutes struct {
	name    string
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewChutes creates a new Chutes.ai image provider. The baseURL is the chute
// API base (e.g. "https://api.chutes.ai/chutes/<id>"); "/generate" is appended
// internally.
func NewChutes(name, baseURL, apiKey string) *Chutes {
	return &Chutes{
		name:    name,
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: chutesTimeout},
	}
}

// Name returns the provider's identifier.
func (c *Chutes) Name() string { return c.name }

// Generate creates an image from a text prompt using the Chutes.ai API.
func (c *Chutes) Generate(ctx context.Context, prompt string) ([]byte, error) {
	reqBody := chutesRequest{
		InputArgs: chutesInputArgs{
			Prompt: prompt,
			Width:  1024,
			Height: 1024,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/generate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		if isTimeoutError(ctx, err) {
			return nil, fmt.Errorf("send request: %w: %w", ErrTimeout, err)
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("API error (status %d): %s: %w", resp.StatusCode, respBody, ErrAuthFailure)
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	imgBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxImageResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(imgBytes) > maxImageResponseSize {
		return nil, fmt.Errorf("response body exceeds %d bytes limit", maxImageResponseSize)
	}
	if len(imgBytes) == 0 {
		return nil, fmt.Errorf("empty response from Chutes.ai")
	}

	return imgBytes, nil
}

type chutesRequest struct {
	InputArgs chutesInputArgs `json:"input_args"`
}

type chutesInputArgs struct {
	Prompt string `json:"prompt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
