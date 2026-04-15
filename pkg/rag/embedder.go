package rag

import "context"

// Embedder converts text strings into vector embeddings.
type Embedder interface {
	// Embed computes embedding vectors for the given texts.
	// Returns one vector per input text.
	Embed(ctx context.Context, texts []string) ([][]float64, error)

	// Dimensions returns the dimensionality of the embedding vectors.
	Dimensions() int
}
