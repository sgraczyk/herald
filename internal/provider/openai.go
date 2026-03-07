package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// maxResponseSize is the maximum allowed response body size (10 MB).
const maxResponseSize = 10 << 20

// OpenAI is an OpenAI-compatible HTTP provider.
// Works with any API that implements the OpenAI chat completions endpoint
// (Chutes.ai, Groq, Gemini, DeepSeek, etc.).
type OpenAI struct {
	name    string
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// NewOpenAI creates a new OpenAI-compatible provider.
func NewOpenAI(name, baseURL, model, apiKey string) *OpenAI {
	return &OpenAI{
		name:    name,
		baseURL: baseURL,
		model:   model,
		apiKey:  apiKey,
		client:  &http.Client{},
	}
}

func (o *OpenAI) Name() string { return o.name }

func (o *OpenAI) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := openaiRequest{
		Model:    o.model,
		Messages: make([]openaiMessage, len(messages)),
	}
	for i, m := range messages {
		reqBody.Messages[i] = openaiMessage{
			Role:    m.Role,
			Content: buildOpenAIContent(m),
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("send request: %w: %w", ErrTimeout, err)
		}
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if len(respBody) > maxResponseSize {
		return "", fmt.Errorf("response body exceeds %d bytes limit", maxResponseSize)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, respBody)
	}

	var result openaiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("empty response from %s", o.name)
	}

	content, ok := result.Choices[0].Message.Content.(string)
	if !ok {
		return "", fmt.Errorf("unexpected content type in response from %s", o.name)
	}
	return content, nil
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// buildOpenAIContent returns a plain string for text-only messages,
// or a content array with text and image_url parts for image messages.
func buildOpenAIContent(m Message) any {
	if len(m.Images) == 0 {
		return m.Content
	}

	parts := make([]map[string]any, 0, 1+len(m.Images))
	if m.Content != "" {
		parts = append(parts, map[string]any{
			"type": "text",
			"text": m.Content,
		})
	}
	for _, img := range m.Images {
		parts = append(parts, map[string]any{
			"type": "image_url",
			"image_url": map[string]string{
				"url": "data:" + img.MimeType + ";base64," + img.Base64,
			},
		})
	}
	return parts
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}
