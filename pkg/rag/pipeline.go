package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/llm"
)

// PipelineOption configures a RAG pipeline.
type PipelineOption func(*pipelineOpts)

type pipelineOpts struct {
	topK         int
	systemPrompt string
}

// WithTopK sets the number of documents to retrieve.
func WithTopK(k int) PipelineOption {
	return func(o *pipelineOpts) {
		if k > 0 {
			o.topK = k
		}
	}
}

// WithSystemPrompt sets a custom system prompt for the RAG pipeline.
func WithSystemPrompt(prompt string) PipelineOption {
	return func(o *pipelineOpts) {
		o.systemPrompt = prompt
	}
}

const defaultRAGSystemPrompt = `You are a helpful assistant. Answer the user's question based on the provided context. If the context doesn't contain enough information to answer, say so honestly.`

// ragPipeline implements the full RAG flow as an Agent[string, string].
type ragPipeline struct {
	name     string
	embedder Embedder
	store    VectorStore
	provider llm.Provider
	opts     pipelineOpts
}

func (p *ragPipeline) Name() string { return p.name }

func (p *ragPipeline) Run(ctx context.Context, query string) (string, error) {
	// 1. Embed the query.
	vectors, err := p.embedder.Embed(ctx, []string{query})
	if err != nil {
		return "", fmt.Errorf("rag pipeline %q: embed query: %w", p.name, err)
	}
	if len(vectors) == 0 {
		return "", fmt.Errorf("rag pipeline %q: embedder returned no vectors", p.name)
	}

	// 2. Search for relevant documents.
	results, err := p.store.Search(ctx, vectors[0], p.opts.topK)
	if err != nil {
		return "", fmt.Errorf("rag pipeline %q: search: %w", p.name, err)
	}

	// 3. Build context from retrieved documents.
	var contextParts []string
	for i, r := range results {
		contextParts = append(contextParts, fmt.Sprintf("[Document %d] (score: %.3f)\n%s", i+1, r.Score, r.Document.Content))
	}
	contextStr := strings.Join(contextParts, "\n\n")

	// 4. Build messages and call LLM.
	systemPrompt := p.opts.systemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultRAGSystemPrompt
	}

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: systemPrompt},
		{Role: llm.RoleUser, Content: fmt.Sprintf("Context:\n%s\n\nQuestion: %s", contextStr, query)},
	}

	answer, err := p.provider.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("rag pipeline %q: chat: %w", p.name, err)
	}

	return answer, nil
}

// NewPipeline creates a complete RAG pipeline as an Agent[string, string].
//
// The pipeline:
//  1. Embeds the query using the Embedder
//  2. Searches the VectorStore for relevant documents
//  3. Constructs a context-augmented prompt
//  4. Sends to the LLM Provider for generation
//
// Example:
//
//	pipeline := rag.NewPipeline(embedder, vectorStore, llmProvider,
//	    rag.WithTopK(5),
//	    rag.WithSystemPrompt("You are a technical documentation assistant."),
//	)
//	answer, err := pipeline.Run(ctx, "How do I configure the router?")
func NewPipeline(name string, embedder Embedder, store VectorStore, provider llm.Provider, options ...PipelineOption) agent.Agent[string, string] {
	p := &ragPipeline{
		name:     name,
		embedder: embedder,
		store:    store,
		provider: provider,
		opts: pipelineOpts{
			topK: 3,
		},
	}
	for _, opt := range options {
		opt(&p.opts)
	}
	return p
}

// Ingest splits a document, embeds the chunks, and adds them to the store.
// This is a convenience function for populating the vector store.
func Ingest(ctx context.Context, text string, id string, embedder Embedder, store VectorStore, splitter Splitter) error {
	chunks := splitter.Split(text)
	if len(chunks) == 0 {
		return nil
	}

	vectors, err := embedder.Embed(ctx, chunks)
	if err != nil {
		return fmt.Errorf("rag ingest: embed: %w", err)
	}

	docs := make([]Document, len(chunks))
	for i, chunk := range chunks {
		docs[i] = Document{
			ID:      fmt.Sprintf("%s-chunk-%d", id, i),
			Content: chunk,
			Vector:  vectors[i],
			Metadata: map[string]string{
				"source_id": id,
				"chunk_idx": fmt.Sprintf("%d", i),
			},
		}
	}

	return store.Add(ctx, docs)
}
