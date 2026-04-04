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
	defaultAnthropicBaseURL    = "https://api.anthropic.com/v1"
	defaultAnthropicModel      = "claude-3-5-sonnet-20241022"
	anthropicAPIVersion        = "2023-06-01"
	defaultAnthropicMaxTokens  = 8096
)

// AnthropicOption is a functional option for configuring the Anthropic provider.
type AnthropicOption func(*anthropicProvider)

// WithAnthropicModel sets the Claude model name.
func WithAnthropicModel(model string) AnthropicOption {
	return func(p *anthropicProvider) {
		p.model = model
	}
}

// WithAnthropicMaxTokens sets the maximum number of tokens in the response.
func WithAnthropicMaxTokens(n int) AnthropicOption {
	return func(p *anthropicProvider) {
		p.maxTokens = n
	}
}

// WithAnthropicHTTPClient replaces the default HTTP client (useful for testing).
func WithAnthropicHTTPClient(client *http.Client) AnthropicOption {
	return func(p *anthropicProvider) {
		p.httpClient = client
	}
}

// anthropicProvider implements the Provider interface using the Anthropic Messages API.
type anthropicProvider struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// NewAnthropic creates a new Anthropic Claude provider.
//
// Example:
//
//	provider := llm.NewAnthropic("sk-ant-...",
//	    llm.WithAnthropicModel("claude-3-haiku-20240307"),
//	)
func NewAnthropic(apiKey string, opts ...AnthropicOption) Provider {
	p := &anthropicProvider{
		apiKey:     apiKey,
		baseURL:    defaultAnthropicBaseURL,
		model:      defaultAnthropicModel,
		maxTokens:  defaultAnthropicMaxTokens,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Info returns metadata about this Anthropic provider instance.
func (p *anthropicProvider) Info() ProviderInfo {
	return ProviderInfo{
		Name:             "anthropic",
		Model:            p.model,
		CostPer1KTokens:  0.003, // approximate for claude-3-5-sonnet
		AvgLatencyMs:     600,
		MaxContextTokens: 200000,
		SupportsStreaming: true,
		Tags:             []string{"powerful", "long-context"},
	}
}

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
	Stream    bool                `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the non-streaming response body.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// anthropicStreamEvent is a single SSE event in streaming mode.
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// Chat sends messages to the Anthropic Messages API and returns the full response.
// System messages are extracted and sent separately as the "system" field.
func (p *anthropicProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	system, msgs := extractSystem(messages)

	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    system,
		Messages:  msgs,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("anthropic: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("anthropic: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("anthropic: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("anthropic: read response: %w", err)
	}

	var result anthropicResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("anthropic API error: %s (%s)", result.Error.Message, result.Error.Type)
	}

	var sb strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// ChatStream sends messages and streams the response tokens via a channel.
func (p *anthropicProvider) ChatStream(ctx context.Context, messages []Message) (<-chan string, error) {
	system, msgs := extractSystem(messages)

	reqBody := anthropicRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    system,
		Messages:  msgs,
		Stream:    true,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal stream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create stream request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http stream request: %w", err)
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

			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				continue
			}

			if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
				select {
				case ch <- event.Delta.Text:
				case <-ctx.Done():
					return
				}
			}
			if event.Type == "message_stop" {
				return
			}
		}
	}()

	return ch, nil
}

// extractSystem separates the first system message from the conversation messages.
// Anthropic's API requires the system prompt to be sent as a separate top-level field.
func extractSystem(messages []Message) (string, []anthropicMessage) {
	var system string
	result := make([]anthropicMessage, 0, len(messages))
	for _, m := range messages {
		if m.Role == RoleSystem {
			system = m.Content
		} else {
			result = append(result, anthropicMessage{Role: string(m.Role), Content: m.Content})
		}
	}
	return system, result
}
