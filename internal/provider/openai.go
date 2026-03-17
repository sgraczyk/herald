package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// maxResponseSize is the maximum allowed response body size (10 MB).
const maxResponseSize = 10 << 20

// openAITimeout is the HTTP client timeout for OpenAI-compatible providers.
const openAITimeout = 60 * time.Second

// OpenAI is an OpenAI-compatible HTTP provider.
// Works with any API that implements the OpenAI chat completions endpoint
// (Chutes.ai, Groq, Gemini, DeepSeek, etc.).
type OpenAI struct {
	name         string
	baseURL      string
	model        string
	apiKey       string
	client       *http.Client
	streamClient *http.Client // no timeout — streaming lifecycle controlled by ctx
}

// NewOpenAI creates a new OpenAI-compatible provider.
func NewOpenAI(name, baseURL, model, apiKey string) *OpenAI {
	return &OpenAI{
		name:         name,
		baseURL:      baseURL,
		model:        model,
		apiKey:       apiKey,
		client:       &http.Client{Timeout: openAITimeout},
		streamClient: &http.Client{},
	}
}

// Name returns the configured provider name.
func (o *OpenAI) Name() string { return o.name }

// Chat sends a conversation to the OpenAI-compatible endpoint and returns the response.
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
		if isTimeoutError(ctx, err) {
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
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("API error (status %d): %s: %w", resp.StatusCode, respBody, ErrAuthFailure)
		}
		return "", &HTTPStatusError{Code: resp.StatusCode, Body: string(respBody)}
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

// ChatStream sends a conversation to the OpenAI-compatible endpoint in streaming
// mode and calls fn with text deltas as they arrive.
func (o *OpenAI) ChatStream(ctx context.Context, messages []Message, fn func(delta string)) (string, error) {
	reqBody := openaiStreamRequest{
		Model:    o.model,
		Messages: make([]openaiMessage, len(messages)),
		Stream:   true,
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

	resp, err := o.streamClient.Do(req)
	if err != nil {
		if isTimeoutError(ctx, err) {
			return "", fmt.Errorf("send request: %w: %w", ErrTimeout, err)
		}
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("API error (status %d): %s: %w", resp.StatusCode, respBody, ErrAuthFailure)
		}
		return "", &HTTPStatusError{Code: resp.StatusCode, Body: string(respBody)}
	}

	var full strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				fn(delta)
				full.WriteString(delta)
			}
		}
	}

	result := full.String()
	if result == "" {
		return "", fmt.Errorf("empty response from %s stream", o.name)
	}
	return result, nil
}

type openaiStreamRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openaiStreamChunk struct {
	Choices []openaiStreamChoice `json:"choices"`
}

type openaiStreamChoice struct {
	Delta struct {
		Content string `json:"content"`
	} `json:"delta"`
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

// isTimeoutError returns true if the error indicates a timeout, either from
// the request context or the HTTP client's own timeout.
func isTimeoutError(ctx context.Context, err error) bool {
	if ctx.Err() == context.DeadlineExceeded {
		return true
	}
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}
