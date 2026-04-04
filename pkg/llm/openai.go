package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultOpenAIModel   = "gpt-4o"
)

// OpenAIOption is a functional option for configuring the OpenAI provider.
type OpenAIOption func(*openAIProvider)

// WithOpenAIBaseURL overrides the API base URL (useful for OpenAI-compatible APIs
// like Azure OpenAI, LM Studio, or local proxies).
func WithOpenAIBaseURL(url string) OpenAIOption {
	return func(p *openAIProvider) {
		p.baseURL = strings.TrimRight(url, "/")
	}
}

// WithOpenAIModel sets the model name.
func WithOpenAIModel(model string) OpenAIOption {
	return func(p *openAIProvider) {
		p.model = model
	}
}

// WithOpenAIHTTPClient replaces the default HTTP client (useful for testing).
func WithOpenAIHTTPClient(client *http.Client) OpenAIOption {
	return func(p *openAIProvider) {
		p.httpClient = client
	}
}

// openAIProvider implements the Provider interface using the OpenAI Chat API.
type openAIProvider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// NewOpenAI creates a new OpenAI provider.
//
// Example:
//
//	provider := llm.NewOpenAI("sk-...",
//	    llm.WithOpenAIModel("gpt-4o-mini"),
//	)
func NewOpenAI(apiKey string, opts ...OpenAIOption) Provider {
	p := &openAIProvider{
		apiKey:     apiKey,
		baseURL:    defaultOpenAIBaseURL,
		model:      defaultOpenAIModel,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Info returns metadata about this OpenAI provider instance.
func (p *openAIProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:              "openai",
		Model:             p.model,
		CostPer1KTokens:   0.005, // approximate for gpt-4o
		AvgLatencyMs:      800,
		MaxContextTokens:  128000,
		SupportsStreaming: true,
		Tags:              []string{"powerful", "reliable"},
	}
}

// openAIChatRequest is the request body for the OpenAI Chat Completions API.
type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is the non-streaming response body.
type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// openAIStreamChunk is a single Server-Sent Event chunk in streaming mode.
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// Chat sends messages to the OpenAI Chat Completions API and returns the full response.
func (p *openAIProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	reqBody := openAIChatRequest{
		Model:    p.model,
		Messages: convertMessages(messages),
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("openai: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("openai: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}

	var result openAIChatResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("openai API error: %s (type: %s)", result.Error.Message, result.Error.Type)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai: no choices in response (status %d)", resp.StatusCode)
	}

	return result.Choices[0].Message.Content, nil
}

// ChatStream sends messages and streams the response as tokens via a channel.
// The channel is closed when the stream ends or an error occurs.
func (p *openAIProvider) ChatStream(ctx context.Context, messages []Message) (<-chan string, error) {
	reqBody := openAIChatRequest{
		Model:    p.model,
		Messages: convertMessages(messages),
		Stream:   true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai: create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: http stream request: %w", err)
	}

	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if payload == "[DONE]" {
				return
			}

			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			token := chunk.Choices[0].Delta.Content
			if token == "" {
				continue
			}
			select {
			case ch <- token:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// convertMessages converts Provider-agnostic Messages to OpenAI message format.
func convertMessages(msgs []Message) []openAIMessage {
	result := make([]openAIMessage, len(msgs))
	for i, m := range msgs {
		result[i] = openAIMessage{Role: string(m.Role), Content: m.Content}
	}
	return result
}
