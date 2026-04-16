package agent

import (
	"context"
	"fmt"
	"sync"
)

// PipelineContext is a typed key-value bag that flows alongside pipeline stages.
// It allows stages to share data without polluting intermediate types.
//
// PipelineContext is stored in the standard context.Context via WithPipelineCtx
// and retrieved via PipelineCtxFrom. It is safe for concurrent use.
//
// Example:
//
//	pctx := agent.NewPipelineContext()
//	pctx.Set("pr_ref", prRef)
//	ctx := agent.WithPipelineCtx(ctx, pctx)
//
//	// In a downstream agent:
//	pctx := agent.PipelineCtxFrom(ctx)
//	ref, ok := agent.PipelineGet[PRRef](pctx, "pr_ref")
type PipelineContext struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewPipelineContext creates an empty PipelineContext.
func NewPipelineContext() *PipelineContext {
	return &PipelineContext{data: make(map[string]any)}
}

// Set stores a value under the given key.
func (p *PipelineContext) Set(key string, value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[key] = value
}

// Get retrieves a value by key. Returns (value, true) if found, (nil, false) if not.
func (p *PipelineContext) Get(key string) (any, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	v, ok := p.data[key]
	return v, ok
}

// PipelineGet is a typed helper that retrieves and asserts a value from PipelineContext.
// Returns (zero, false) if the key is missing or the type assertion fails.
func PipelineGet[T any](p *PipelineContext, key string) (T, bool) {
	v, ok := p.Get(key)
	if !ok {
		var zero T
		return zero, false
	}
	typed, ok := v.(T)
	return typed, ok
}

// pipelineCtxKey is the context key for PipelineContext.
type pipelineCtxKey struct{}

// WithPipelineCtx attaches a PipelineContext to a standard context.
func WithPipelineCtx(ctx context.Context, p *PipelineContext) context.Context {
	return context.WithValue(ctx, pipelineCtxKey{}, p)
}

// PipelineCtxFrom retrieves the PipelineContext from context.
// Returns nil if not present.
func PipelineCtxFrom(ctx context.Context) *PipelineContext {
	p, _ := ctx.Value(pipelineCtxKey{}).(*PipelineContext)
	return p
}

// Pipeline is a builder for composing arbitrary-length agent chains using
// UntypedAgent. It trades compile-time type safety for flexibility when
// you need more than 5 stages.
//
// For 2-5 stages, prefer Chain2-Chain5 which are fully type-safe.
//
// Example:
//
//	result, err := agent.NewPipeline("review-pipeline").
//	    Add(agent.Erase(fetchAgent)).
//	    Add(agent.Erase(splitAgent)).
//	    Add(agent.Erase(reviewAgent)).
//	    Add(agent.Erase(summaryAgent)).
//	    Add(agent.Erase(postAgent)).
//	    Run(ctx, prURL)
type Pipeline struct {
	name   string
	stages []UntypedAgent
}

// NewPipeline creates a named Pipeline builder.
func NewPipeline(name string) *Pipeline {
	return &Pipeline{name: name}
}

// Add appends an UntypedAgent stage to the pipeline.
// Use agent.Erase() to convert typed agents.
func (p *Pipeline) Add(stage UntypedAgent) *Pipeline {
	p.stages = append(p.stages, stage)
	return p
}

// Name returns the pipeline name.
func (p *Pipeline) Name() string { return p.name }

// RunUntyped executes the pipeline, passing output of each stage as input to the next.
func (p *Pipeline) RunUntyped(ctx context.Context, input any) (any, error) {
	if len(p.stages) == 0 {
		return input, nil
	}

	current := input
	for i, stage := range p.stages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var err error
		current, err = stage.RunUntyped(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("pipeline %q stage %d (%s): %w", p.name, i, stage.Name(), err)
		}
	}
	return current, nil
}

// Run is a convenience that executes the pipeline and returns the untyped result.
// Callers must type-assert the result.
func (p *Pipeline) Run(ctx context.Context, input any) (any, error) {
	return p.RunUntyped(ctx, input)
}
