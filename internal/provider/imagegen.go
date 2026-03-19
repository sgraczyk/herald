package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ImageProvider generates images from text prompts.
type ImageProvider interface {
	// Generate creates an image from a text prompt and returns the raw PNG bytes.
	Generate(ctx context.Context, prompt string) ([]byte, error)
}

// dalleTimeout is the HTTP client timeout for DALL-E image generation.
const dalleTimeout = 60 * time.Second

// DallE generates images using the OpenAI DALL-E 3 API.
type DallE struct {
	apiKey string
	client *http.Client
}

// NewDallE creates a new DALL-E 3 image provider.
func NewDallE(apiKey string) *DallE {
	return &DallE{
		apiKey: apiKey,
		client: &http.Client{Timeout: dalleTimeout},
	}
}

// Generate creates an image from a text prompt using DALL-E 3.
func (d *DallE) Generate(ctx context.Context, prompt string) ([]byte, error) {
	reqBody := dalleRequest{
		Model:          "dall-e-3",
		Prompt:         prompt,
		N:              1,
		Size:           "1024x1024",
		ResponseFormat: "b64_json",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		if isTimeoutError(ctx, err) {
			return nil, fmt.Errorf("send request: %w: %w", ErrTimeout, err)
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(respBody) > maxResponseSize {
		return nil, fmt.Errorf("response body exceeds %d bytes limit", maxResponseSize)
	}

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, fmt.Errorf("API error (status %d): %s: %w", resp.StatusCode, respBody, ErrAuthFailure)
		}
		return nil, &HTTPStatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	var result dalleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("empty response from DALL-E")
	}

	imgBytes, err := base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return imgBytes, nil
}

type dalleRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	ResponseFormat string `json:"response_format"`
}

type dalleResponse struct {
	Data []dalleImage `json:"data"`
}

type dalleImage struct {
	B64JSON string `json:"b64_json"`
}
