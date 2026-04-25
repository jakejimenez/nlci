package backend

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

// OpenAIBackend implements the OpenAI-compatible /v1/chat/completions API.
// It handles Ollama, llama.cpp, and LM Studio — all three use the same endpoint.
type OpenAIBackend struct {
	name     string
	baseURL  string
	model    string
	healthFn func(ctx context.Context, client *http.Client, baseURL string) error
	client   *http.Client
}

// NewOllama creates a backend for Ollama (localhost:11434).
func NewOllama(host, model string) *OpenAIBackend {
	return &OpenAIBackend{
		name:    "ollama",
		baseURL: "http://" + host,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
		healthFn: func(ctx context.Context, c *http.Client, base string) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/", nil)
			resp, err := c.Do(req)
			if err != nil {
				return fmt.Errorf("%w: ollama not reachable at %s", ErrBackendUnavailable, base)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if !strings.Contains(string(body), "Ollama is running") {
				return fmt.Errorf("%w: ollama health check failed", ErrBackendUnavailable)
			}
			return nil
		},
	}
}

// NewLlamaCpp creates a backend for llama.cpp server (localhost:8080).
func NewLlamaCpp(host, model string) *OpenAIBackend {
	return &OpenAIBackend{
		name:    "llamacpp",
		baseURL: "http://" + host,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
		healthFn: func(ctx context.Context, c *http.Client, base string) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
			resp, err := c.Do(req)
			if err != nil {
				return fmt.Errorf("%w: llama.cpp not reachable at %s", ErrBackendUnavailable, base)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%w: llama.cpp health check returned %d", ErrBackendUnavailable, resp.StatusCode)
			}
			return nil
		},
	}
}

// NewLMStudio creates a backend for LM Studio (localhost:1234).
func NewLMStudio(host, model string) *OpenAIBackend {
	return &OpenAIBackend{
		name:    "lmstudio",
		baseURL: "http://" + host,
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
		healthFn: func(ctx context.Context, c *http.Client, base string) error {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
			resp, err := c.Do(req)
			if err != nil {
				return fmt.Errorf("%w: LM Studio not reachable at %s", ErrBackendUnavailable, base)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("%w: LM Studio health check returned %d", ErrBackendUnavailable, resp.StatusCode)
			}
			return nil
		},
	}
}

func (o *OpenAIBackend) Name() string { return o.name }

func (o *OpenAIBackend) Ping(ctx context.Context) error {
	return o.healthFn(ctx, o.client, o.baseURL)
}

// chatRequest is the OpenAI-compatible request body.
type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Stream         bool          `json:"stream"`          // always false
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
	Temperature    float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// commandOutput is the structured JSON we ask the model to return.
type commandOutput struct {
	Command     string `json:"command"`
	Explanation string `json:"explanation"`
}

func (o *OpenAIBackend) Generate(ctx context.Context, r Request) (Response, error) {
	// The system prompt (r.System) already contains the schema and the user
	// prompt (r.Intent) already contains examples — no augmentation needed here.
	userContent := r.Intent + "\n\nRespond with JSON: {\"command\": \"...\", \"explanation\": \"...\"}"

	body := chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: r.System},
			{Role: "user", Content: userContent},
		},
		Stream:         false, // EXPLICIT: Ollama defaults to stream:true
		ResponseFormat: &respFormat{Type: "json_object"},
		Temperature:    0.1,
	}

	raw, err := o.post(ctx, "/v1/chat/completions", body)
	if err != nil {
		return Response{}, err
	}

	var chatResp chatResponse
	if err := json.Unmarshal(raw, &chatResp); err != nil {
		return Response{}, fmt.Errorf("%s: parse response: %w", o.name, err)
	}

	if len(chatResp.Choices) == 0 {
		return Response{}, fmt.Errorf("%s: empty response choices", o.name)
	}

	content := chatResp.Choices[0].Message.Content

	// Parse the JSON command output
	var out commandOutput
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		// Fallback: treat the entire content as the command
		return Response{Command: strings.TrimSpace(content)}, nil
	}

	return Response{
		Command:     strings.TrimSpace(out.Command),
		Explanation: strings.TrimSpace(out.Explanation),
	}, nil
}

func (o *OpenAIBackend) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal request: %w", o.name, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: build request: %w", o.name, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", o.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", o.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", o.name, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
