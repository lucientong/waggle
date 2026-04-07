package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	defaultOllamaBaseURL = "http://localhost:11434"
	defaultOllamaModel   = "llama3.2"
)

// OllamaOption is a functional option for configuring the Ollama provider.
type OllamaOption func(*ollamaProvider)

// WithOllamaBaseURL sets the Ollama server URL (default: http://localhost:11434).
func WithOllamaBaseURL(url string) OllamaOption {
	return func(p *ollamaProvider) {
		p.baseURL = url
	}
}

// WithOllamaModel sets the model name.
func WithOllamaModel(model string) OllamaOption {
	return func(p *ollamaProvider) {
		p.model = model
	}
}

// WithOllamaHTTPClient replaces the default HTTP client (useful for testing).
func WithOllamaHTTPClient(client *http.Client) OllamaOption {
	return func(p *ollamaProvider) {
		p.httpClient = client
	}
}

// ollamaProvider implements the Provider interface using the Ollama local API.
type ollamaProvider struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOllama creates a new Ollama provider for local model inference.
//
// Ollama must be running locally (or at the configured URL).
// See https://ollama.ai for installation instructions.
//
// Example:
//
//	provider := llm.NewOllama(
//	    llm.WithOllamaModel("llama3.2"),
//	    llm.WithOllamaBaseURL("http://localhost:11434"),
//	)
func NewOllama(opts ...OllamaOption) Provider {
	p := &ollamaProvider{
		baseURL:    defaultOllamaBaseURL,
		model:      defaultOllamaModel,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Info returns metadata about this Ollama provider instance.
func (p *ollamaProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:              "ollama",
		Model:             p.model,
		CostPer1KTokens:   0, // local — no cost
		AvgLatencyMs:      2000,
		MaxContextTokens:  8192,
		SupportsStreaming: true,
		Tags:              []string{"local", "free", "private"},
	}
}

// ollamaChatRequest is the request body for the Ollama /api/chat endpoint.
type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatResponse is a single response chunk from the Ollama streaming API.
type ollamaChatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error,omitempty"`
}

// Chat sends messages to the local Ollama server and returns the full response.
func (p *ollamaProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	ch, err := p.ChatStream(ctx, messages)
	if err != nil {
		return "", err
	}

	var sb []byte
	for token := range ch {
		sb = append(sb, token...)
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("ollama: context cancelled: %w", err)
	}

	return string(sb), nil
}

// ChatStream sends messages to Ollama and streams the response tokens via a channel.
func (p *ollamaProvider) ChatStream(ctx context.Context, messages []Message) (<-chan string, error) {
	msgs := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		msgs[i] = ollamaMessage{Role: string(m.Role), Content: m.Content}
	}

	reqBody := ollamaChatRequest{
		Model:    p.model,
		Messages: msgs,
		Stream:   true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("ollama: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var chunk ollamaChatResponse
			if err := json.Unmarshal(line, &chunk); err != nil {
				continue
			}
			if chunk.Error != "" {
				return
			}

			if chunk.Message.Content != "" {
				select {
				case ch <- chunk.Message.Content:
				case <-ctx.Done():
					return
				}
			}
			if chunk.Done {
				return
			}
		}
	}()

	return ch, nil
}
