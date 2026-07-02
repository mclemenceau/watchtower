// Package openrouter provides the OpenRouter LLM adapter implementing ports.LLMClient.
package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mclemenceau/watchtower/internal/ports"
)

// Compile-time interface satisfaction check.
var _ ports.LLMClient = (*OpenRouterClient)(nil)
var _ ports.LLMClient = (*MockLLMClient)(nil)

const openRouterBaseURL = "https://openrouter.ai/api/v1/chat/completions"

// OpenRouterClient is the real LLM implementation backed by OpenRouter.
type OpenRouterClient struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewClient creates a new OpenRouterClient.
func NewClient(apiKey, model string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Complete calls the OpenRouter API and returns the model's response.
func (c *OpenRouterClient) Complete(ctx context.Context, system, prompt string) (string, error) {
	msgs := []message{}
	if system != "" {
		msgs = append(msgs, message{Role: "system", Content: system})
	}
	msgs = append(msgs, message{Role: "user", Content: prompt})

	body, err := json.Marshal(chatRequest{Model: c.model, Messages: msgs})
	if err != nil {
		return "", fmt.Errorf("Complete: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Complete: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Complete: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Complete: read body: %w", err)
	}

	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("Complete: decode: %w", err)
	}

	if cr.Error != nil {
		return "", fmt.Errorf("Complete: openrouter error: %s", cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Complete: unexpected status %d: %s", resp.StatusCode, string(raw))
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("Complete: empty choices in response")
	}

	return cr.Choices[0].Message.Content, nil
}

// MockLLMClient returns a fixed response for use in tests.
type MockLLMClient struct {
	Response string
	Err      error
}

// Complete returns the pre-configured response (or Err if set).
func (m *MockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	return m.Response, m.Err
}
