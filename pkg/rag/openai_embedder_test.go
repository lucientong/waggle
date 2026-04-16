package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbedder_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var req openAIEmbeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Input) != 2 {
			t.Errorf("expected 2 inputs, got %d", len(req.Input))
		}
		if req.Model != "text-embedding-3-small" {
			t.Errorf("unexpected model: %s", req.Model)
		}

		resp := openAIEmbeddingResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float64{0.1, 0.2, 0.3}, Index: 0},
				{Embedding: []float64{0.4, 0.5, 0.6}, Index: 1},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder("test-key",
		WithEmbeddingHTTPClient(server.Client()),
		WithEmbeddingDimensions(3),
	)
	// Override URL to test server.
	embedder.baseURL = server.URL

	vectors, err := embedder.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if len(vectors[0]) != 3 || vectors[0][0] != 0.1 {
		t.Errorf("unexpected vector[0]: %v", vectors[0])
	}
	if vectors[1][0] != 0.4 {
		t.Errorf("unexpected vector[1]: %v", vectors[1])
	}
}

func TestOpenAIEmbedder_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIEmbeddingResponse{
			Error: &struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			}{Message: "invalid api key", Type: "invalid_request_error"},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder("bad-key")
	embedder.baseURL = server.URL

	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAIEmbedder_EmptyInput(t *testing.T) {
	embedder := NewOpenAIEmbedder("key")
	vectors, err := embedder.Embed(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if vectors != nil {
		t.Errorf("expected nil for empty input, got %v", vectors)
	}
}

func TestOpenAIEmbedder_Dimensions(t *testing.T) {
	e := NewOpenAIEmbedder("key", WithEmbeddingDimensions(768))
	if e.Dimensions() != 768 {
		t.Errorf("expected 768, got %d", e.Dimensions())
	}
}

func TestOpenAIEmbedder_CustomModel(t *testing.T) {
	e := NewOpenAIEmbedder("key", WithEmbeddingModel("text-embedding-ada-002"))
	if e.model != "text-embedding-ada-002" {
		t.Errorf("unexpected model: %s", e.model)
	}
}

func TestOpenAIEmbedder_OutOfOrderIndex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return embeddings in reverse order.
		resp := openAIEmbeddingResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float64{0.4, 0.5}, Index: 1},
				{Embedding: []float64{0.1, 0.2}, Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder("key")
	embedder.baseURL = server.URL

	vectors, err := embedder.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	// Should be reordered by index.
	if vectors[0][0] != 0.1 || vectors[1][0] != 0.4 {
		t.Errorf("vectors not ordered by index: %v", vectors)
	}
}
