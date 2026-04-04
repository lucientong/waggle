package llm

import (
	"context"
	"fmt"
	"sort"
)

// RoutingStrategy determines how the router selects a provider.
type RoutingStrategy int

const (
	// StrategyLowestCost selects the provider with the lowest cost per 1K tokens.
	StrategyLowestCost RoutingStrategy = iota
	// StrategyLowestLatency selects the provider with the lowest average latency.
	StrategyLowestLatency
	// StrategyRoundRobin distributes requests evenly across all providers.
	StrategyRoundRobin
	// StrategyFailover uses the first provider; falls back to the next on error.
	StrategyFailover
)

// routerProvider implements Provider by selecting from multiple providers
// according to a routing strategy.
type routerProvider struct {
	providers  []Provider
	strategy   RoutingStrategy
	roundRobin int // for StrategyRoundRobin
}

// RouterOption configures the LLM router.
type RouterOption func(*routerProvider)

// WithRoutingStrategy sets the provider selection strategy.
func WithRoutingStrategy(s RoutingStrategy) RouterOption {
	return func(r *routerProvider) {
		r.strategy = s
	}
}

// NewRouter creates a Provider that intelligently routes requests across
// multiple underlying providers.
//
// Example:
//
//	router := llm.NewRouter(
//	    []llm.Provider{openaiProvider, anthropicProvider, ollamaProvider},
//	    llm.WithRoutingStrategy(llm.StrategyLowestCost),
//	)
func NewRouter(providers []Provider, opts ...RouterOption) Provider {
	r := &routerProvider{
		providers: providers,
		strategy:  StrategyFailover,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Info returns the info of the primary (first) provider.
func (r *routerProvider) Info() ProviderInfo {
	if len(r.providers) == 0 {
		return ProviderInfo{Name: "router"}
	}
	info := r.providers[0].Info()
	info.Name = "router/" + info.Name
	return info
}

// Chat selects a provider according to the routing strategy and delegates the request.
func (r *routerProvider) Chat(ctx context.Context, messages []Message) (string, error) {
	if len(r.providers) == 0 {
		return "", fmt.Errorf("llm router: no providers configured")
	}

	switch r.strategy {
	case StrategyFailover:
		return r.chatFailover(ctx, messages)
	case StrategyRoundRobin:
		return r.chatRoundRobin(ctx, messages)
	case StrategyLowestCost:
		return r.chatBestBy(ctx, messages, func(a, b ProviderInfo) bool {
			return a.CostPer1KTokens < b.CostPer1KTokens
		})
	case StrategyLowestLatency:
		return r.chatBestBy(ctx, messages, func(a, b ProviderInfo) bool {
			return a.AvgLatencyMs < b.AvgLatencyMs
		})
	default:
		return r.chatFailover(ctx, messages)
	}
}

// ChatStream selects a provider and delegates the streaming request.
// Uses failover strategy: tries providers in order until one succeeds.
func (r *routerProvider) ChatStream(ctx context.Context, messages []Message) (<-chan string, error) {
	if len(r.providers) == 0 {
		return nil, fmt.Errorf("llm router: no providers configured")
	}

	var lastErr error
	for _, p := range r.providers {
		if !p.Info().SupportsStreaming {
			continue
		}
		ch, err := p.ChatStream(ctx, messages)
		if err == nil {
			return ch, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("llm router: all streaming providers failed: %w", lastErr)
}

// chatFailover tries providers in order, returning the first success.
func (r *routerProvider) chatFailover(ctx context.Context, messages []Message) (string, error) {
	var lastErr error
	for _, p := range r.providers {
		result, err := p.Chat(ctx, messages)
		if err == nil {
			return result, nil
		}
		lastErr = err
		// Stop on context cancellation — no point trying other providers.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("llm router: all providers failed: %w", lastErr)
}

// chatRoundRobin selects the next provider in rotation.
func (r *routerProvider) chatRoundRobin(ctx context.Context, messages []Message) (string, error) {
	idx := r.roundRobin % len(r.providers)
	r.roundRobin++
	return r.providers[idx].Chat(ctx, messages)
}

// chatBestBy sorts providers by a comparison function and uses the best one,
// falling back to the next on error.
func (r *routerProvider) chatBestBy(ctx context.Context, messages []Message, less func(a, b ProviderInfo) bool) (string, error) {
	sorted := make([]Provider, len(r.providers))
	copy(sorted, r.providers)
	sort.Slice(sorted, func(i, j int) bool {
		return less(sorted[i].Info(), sorted[j].Info())
	})

	var lastErr error
	for _, p := range sorted {
		result, err := p.Chat(ctx, messages)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return "", fmt.Errorf("llm router: all providers failed: %w", lastErr)
}
