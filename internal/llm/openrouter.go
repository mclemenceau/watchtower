package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL      = "https://openrouter.ai/api/v1/chat/completions"
	defaultModel = "anthropic/claude-sonnet-4-5"
)

// LLMClient is the interface all activities use — never call OpenRouter directly.
type LLMClient interface {
	Complete(ctx context.Context, system, prompt string) (string, error)
}

// OpenRouterClient is the real implementation backed by OpenRouter.
type OpenRouterClient struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewOpenRouterClient(apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		apiKey: apiKey,
		model:  defaultModel,
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("Complete: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Complete: %w", err)
	}
	defer resp.Body.Close()

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

func (m *MockLLMClient) Complete(_ context.Context, _, _ string) (string, error) {
	return m.Response, m.Err
}
