package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/llm"
)

// mockEmbedder returns simple vectors for testing.
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for i, text := range texts {
		v := make([]float64, m.dims)
		// Simple hash-based embedding for deterministic testing.
		for j, ch := range text {
			v[j%m.dims] += float64(ch) / 1000.0
		}
		vectors[i] = v
	}
	return vectors, nil
}

func (m *mockEmbedder) Dimensions() int { return m.dims }

// mockRAGProvider returns fixed responses.
type mockRAGProvider struct {
	response string
}

func (m *mockRAGProvider) Info() llm.ProviderInfo {
	return llm.ProviderInfo{Name: "mock"}
}

func (m *mockRAGProvider) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	// Verify context is included in the prompt.
	for _, msg := range msgs {
		if msg.Role == llm.RoleUser && strings.Contains(msg.Content, "Context:") {
			return m.response, nil
		}
	}
	return "", fmt.Errorf("expected context in prompt")
}

func (m *mockRAGProvider) ChatStream(_ context.Context, _ []llm.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestPipeline_EndToEnd(t *testing.T) {
	ctx := context.Background()
	embedder := &mockEmbedder{dims: 8}
	store := NewInMemoryStore()
	provider := &mockRAGProvider{response: "Go uses goroutines for concurrency."}

	// Ingest some documents.
	docs := []string{
		"Go is a statically typed language.",
		"Go uses goroutines for lightweight concurrency.",
		"Python is dynamically typed.",
	}
	for i, doc := range docs {
		vectors, _ := embedder.Embed(ctx, []string{doc})
		store.Add(ctx, []Document{{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: doc,
			Vector:  vectors[0],
		}})
	}

	pipeline := NewPipeline("test-rag", embedder, store, provider, WithTopK(2))

	answer, err := pipeline.Run(ctx, "How does Go handle concurrency?")
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Go uses goroutines for concurrency." {
		t.Errorf("unexpected answer: %q", answer)
	}
}

func TestIngest(t *testing.T) {
	ctx := context.Background()
	embedder := &mockEmbedder{dims: 4}
	store := NewInMemoryStore()
	splitter := NewTokenSplitter(5, 0)

	text := "one two three four five six seven eight nine ten"
	err := Ingest(ctx, text, "test-doc", embedder, store, splitter)
	if err != nil {
		t.Fatal(err)
	}

	// Should have created at least 2 chunks.
	results, _ := store.Search(ctx, make([]float64, 4), 10)
	if len(results) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(results))
	}
	// Verify metadata.
	for _, r := range results {
		if r.Document.Metadata["source_id"] != "test-doc" {
			t.Errorf("expected source_id=test-doc, got %s", r.Document.Metadata["source_id"])
		}
	}
}

func TestPipeline_WithSystemPrompt(t *testing.T) {
	ctx := context.Background()
	embedder := &mockEmbedder{dims: 4}
	store := NewInMemoryStore()

	var capturedSystem string
	provider := &customProvider{
		chatFn: func(_ context.Context, msgs []llm.Message) (string, error) {
			for _, m := range msgs {
				if m.Role == llm.RoleSystem {
					capturedSystem = m.Content
				}
			}
			return "answer", nil
		},
	}

	store.Add(ctx, []Document{{ID: "1", Content: "doc", Vector: []float64{1, 0, 0, 0}}})
	pipeline := NewPipeline("test", embedder, store, provider, WithSystemPrompt("Custom system prompt"))

	_, err := pipeline.Run(ctx, "question")
	if err != nil {
		t.Fatal(err)
	}
	if capturedSystem != "Custom system prompt" {
		t.Errorf("expected custom system prompt, got %q", capturedSystem)
	}
}

type customProvider struct {
	chatFn func(context.Context, []llm.Message) (string, error)
}

func (p *customProvider) Info() llm.ProviderInfo { return llm.ProviderInfo{Name: "custom"} }
func (p *customProvider) Chat(ctx context.Context, msgs []llm.Message) (string, error) {
	return p.chatFn(ctx, msgs)
}
func (p *customProvider) ChatStream(_ context.Context, _ []llm.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}
