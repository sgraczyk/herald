package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// generateWithURL is a test helper that calls the DALL-E API at a custom URL
// so we can point at a test server instead of the real OpenAI endpoint.
func generateWithURL(d *DallE, ctx context.Context, prompt, url string) ([]byte, error) {
	reqBody := dalleRequest{
		Model:          "dall-e-3",
		Prompt:         prompt,
		N:              1,
		Size:           "1024x1024",
		ResponseFormat: "b64_json",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, respBody)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result dalleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	return base64.StdEncoding.DecodeString(result.Data[0].B64JSON)
}

func TestDallE_Generate(t *testing.T) {
	fakeImage := []byte("fake-png-image-data")
	b64 := base64.StdEncoding.EncodeToString(fakeImage)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var req dalleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "dall-e-3" {
			t.Errorf("expected model dall-e-3, got %s", req.Model)
		}
		if req.Prompt != "a cute cat" {
			t.Errorf("expected prompt 'a cute cat', got %s", req.Prompt)
		}
		if req.ResponseFormat != "b64_json" {
			t.Errorf("expected response_format b64_json, got %s", req.ResponseFormat)
		}

		resp := dalleResponse{
			Data: []dalleImage{{B64JSON: b64}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDallE("test-key")

	result, err := generateWithURL(d, context.Background(), "a cute cat", srv.URL+"/v1/images/generations")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(result) != string(fakeImage) {
		t.Errorf("expected %q, got %q", fakeImage, result)
	}
}

func TestDallE_GenerateAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": {"message": "content policy violation"}}`))
	}))
	defer srv.Close()

	d := NewDallE("test-key")

	_, err := generateWithURL(d, context.Background(), "bad prompt", srv.URL+"/v1/images/generations")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestDallE_GenerateEmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := dalleResponse{Data: []dalleImage{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewDallE("test-key")

	_, err := generateWithURL(d, context.Background(), "a cat", srv.URL+"/v1/images/generations")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}
