package rag

import (
	"context"
	"math"
	"testing"
)

func TestInMemoryStore_AddAndSearch(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	docs := []Document{
		{ID: "1", Content: "Go is great", Vector: []float64{1, 0, 0}},
		{ID: "2", Content: "Python is popular", Vector: []float64{0, 1, 0}},
		{ID: "3", Content: "Go concurrency", Vector: []float64{0.9, 0.1, 0}},
	}
	if err := store.Add(ctx, docs); err != nil {
		t.Fatal(err)
	}

	// Query vector close to doc 1 and 3.
	results, err := store.Search(ctx, []float64{1, 0, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// First result should be doc 1 (exact match).
	if results[0].Document.ID != "1" {
		t.Errorf("expected doc 1 first, got %s", results[0].Document.ID)
	}
	if math.Abs(results[0].Score-1.0) > 0.001 {
		t.Errorf("expected score ~1.0, got %f", results[0].Score)
	}
	// Second result should be doc 3.
	if results[1].Document.ID != "3" {
		t.Errorf("expected doc 3 second, got %s", results[1].Document.ID)
	}
}

func TestInMemoryStore_SearchEmpty(t *testing.T) {
	store := NewInMemoryStore()
	results, err := store.Search(context.Background(), []float64{1, 0}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float64
		expected float64
		tol      float64
	}{
		{"identical", []float64{1, 0}, []float64{1, 0}, 1.0, 0.001},
		{"orthogonal", []float64{1, 0}, []float64{0, 1}, 0.0, 0.001},
		{"opposite", []float64{1, 0}, []float64{-1, 0}, -1.0, 0.001},
		{"different lengths", []float64{1, 0}, []float64{1}, 0.0, 0.001},
		{"empty", []float64{}, []float64{}, 0.0, 0.001},
		{"zero vector", []float64{0, 0}, []float64{1, 0}, 0.0, 0.001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := cosineSimilarity(tt.a, tt.b)
			if math.Abs(score-tt.expected) > tt.tol {
				t.Errorf("expected %f, got %f", tt.expected, score)
			}
		})
	}
}

func TestInMemoryStore_TopKLimit(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryStore()

	for i := 0; i < 10; i++ {
		v := make([]float64, 3)
		v[0] = float64(i) / 10.0
		store.Add(ctx, []Document{{ID: string(rune('a' + i)), Content: "doc", Vector: v}})
	}

	results, _ := store.Search(ctx, []float64{1, 0, 0}, 3)
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}
