// Package rag provides Retrieval-Augmented Generation (RAG) pipeline components.
//
// Key abstractions:
//   - Embedder: converts text to vector embeddings
//   - VectorStore: stores and searches document vectors
//   - Splitter: splits documents into chunks
//   - NewPipeline: composes these into an Agent[string, string]
//
// An in-memory VectorStore with cosine similarity is provided for testing and
// small datasets. The VectorStore interface is simple enough for community
// implementations of pgvector, Milvus, Weaviate, etc.
package rag

import (
	"context"
	"math"
	"sort"
	"sync"
)

// Document represents a text chunk with its embedding vector and metadata.
type Document struct {
	// ID uniquely identifies this document.
	ID string `json:"id"`
	// Content is the text content.
	Content string `json:"content"`
	// Vector is the embedding vector (nil before embedding).
	Vector []float64 `json:"vector,omitempty"`
	// Metadata holds arbitrary key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// SearchResult is a document with its similarity score.
type SearchResult struct {
	Document Document
	Score    float64 // cosine similarity, higher = more similar
}

// VectorStore stores documents with their embedding vectors and supports
// similarity search.
type VectorStore interface {
	// Add stores documents. Documents must have non-nil Vector fields.
	Add(ctx context.Context, docs []Document) error

	// Search finds the topK most similar documents to the query vector.
	// Results are ordered by descending similarity score.
	Search(ctx context.Context, vector []float64, topK int) ([]SearchResult, error)
}

// InMemoryStore is a simple in-memory VectorStore using cosine similarity.
// Suitable for testing and small datasets (< 10K documents).
//
// InMemoryStore is safe for concurrent use.
type InMemoryStore struct {
	mu   sync.RWMutex
	docs []Document
}

// NewInMemoryStore creates an empty in-memory vector store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{}
}

// Add stores documents in memory.
func (s *InMemoryStore) Add(_ context.Context, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append(s.docs, docs...)
	return nil
}

// Search finds the topK most similar documents using cosine similarity.
func (s *InMemoryStore) Search(_ context.Context, vector []float64, topK int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]SearchResult, 0, len(s.docs))
	for _, doc := range s.docs {
		score := cosineSimilarity(vector, doc.Vector)
		results = append(results, SearchResult{Document: doc, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && topK < len(results) {
		results = results[:topK]
	}
	return results, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
