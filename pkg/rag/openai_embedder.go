package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	defaultOpenAIEmbeddingURL   = "https://api.openai.com/v1/embeddings"
	defaultOpenAIEmbeddingModel = "text-embedding-3-small"
	defaultEmbeddingDimensions  = 1536
)

// OpenAIEmbedderOption configures an OpenAIEmbedder.
type OpenAIEmbedderOption func(*OpenAIEmbedder)

// WithEmbeddingModel sets the embedding model name.
func WithEmbeddingModel(model string) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.model = model
	}
}

// WithEmbeddingBaseURL overrides the API base URL (useful for Azure, proxies, etc.).
func WithEmbeddingBaseURL(url string) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.baseURL = strings.TrimRight(url, "/") + "/embeddings"
	}
}

// WithEmbeddingDimensions sets the expected embedding dimensionality.
// This is used by the Dimensions() method. The actual dimensions depend on the model.
func WithEmbeddingDimensions(dims int) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.dims = dims
	}
}

// WithEmbeddingHTTPClient replaces the default HTTP client (useful for testing).
func WithEmbeddingHTTPClient(client *http.Client) OpenAIEmbedderOption {
	return func(e *OpenAIEmbedder) {
		e.httpClient = client
	}
}

// OpenAIEmbedder implements the Embedder interface using the OpenAI Embeddings API.
//
// It works with any OpenAI-compatible embedding endpoint (Azure, proxies, etc.)
// via WithEmbeddingBaseURL.
//
// Example:
//
//	embedder := rag.NewOpenAIEmbedder("sk-...",
//	    rag.WithEmbeddingModel("text-embedding-3-small"),
//	)
//	vectors, err := embedder.Embed(ctx, []string{"Hello world"})
type OpenAIEmbedder struct {
	apiKey     string
	baseURL    string
	model      string
	dims       int
	httpClient *http.Client
}

// NewOpenAIEmbedder creates an Embedder backed by the OpenAI Embeddings API.
func NewOpenAIEmbedder(apiKey string, opts ...OpenAIEmbedderOption) *OpenAIEmbedder {
	e := &OpenAIEmbedder{
		apiKey:     apiKey,
		baseURL:    defaultOpenAIEmbeddingURL,
		model:      defaultOpenAIEmbeddingModel,
		dims:       defaultEmbeddingDimensions,
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Embed computes embedding vectors for the given texts using the OpenAI API.
func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	reqBody := openAIEmbeddingRequest{
		Model: e.model,
		Input: texts,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai embedder: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai embedder: read response: %w", err)
	}

	var result openAIEmbeddingResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("openai embedder: unmarshal: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("openai embedder API error: %s (type: %s)", result.Error.Message, result.Error.Type)
	}

	// Sort by index to ensure correct ordering.
	vectors := make([][]float64, len(texts))
	for _, d := range result.Data {
		if d.Index < len(vectors) {
			vectors[d.Index] = d.Embedding
		}
	}

	// Verify all vectors were returned.
	for i, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("openai embedder: missing embedding for input %d", i)
		}
	}

	return vectors, nil
}

// Dimensions returns the expected dimensionality of the embedding vectors.
func (e *OpenAIEmbedder) Dimensions() int {
	return e.dims
}
