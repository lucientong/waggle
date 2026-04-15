package agent_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// noopStringAgent returns a minimal Agent[string, string] with negligible work.
func noopStringAgent(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return s, nil
	})
}

// cpuStringAgent returns an Agent that performs a small but measurable CPU task.
func cpuStringAgent(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s) + "!", nil
	})
}

// sleepAgent returns an Agent that sleeps for the given duration.
func sleepAgent(name string, d time.Duration) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(ctx context.Context, s string) (string, error) {
		select {
		case <-time.After(d):
			return s, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
}

// ---------------------------------------------------------------------------
// Func Agent benchmarks
// ---------------------------------------------------------------------------

func BenchmarkFuncAgent_Noop(b *testing.B) {
	a := noopStringAgent("noop")
	ctx := context.Background()

	for b.Loop() {
		_, _ = a.Run(ctx, "hello")
	}
}

func BenchmarkFuncAgent_CPU(b *testing.B) {
	a := cpuStringAgent("cpu")
	ctx := context.Background()

	for b.Loop() {
		_, _ = a.Run(ctx, "hello waggle")
	}
}

func BenchmarkFuncAgent_IntTypes(b *testing.B) {
	a := agent.Func[int, int]("double", func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})
	ctx := context.Background()

	for b.Loop() {
		_, _ = a.Run(ctx, 42)
	}
}

// ---------------------------------------------------------------------------
// Chain benchmarks
// ---------------------------------------------------------------------------

func BenchmarkChain2_Noop(b *testing.B) {
	chain := agent.Chain2(noopStringAgent("a"), noopStringAgent("b"))
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = chain.Run(ctx, "hello")
	}
}

func BenchmarkChain2_CPU(b *testing.B) {
	chain := agent.Chain2(cpuStringAgent("a"), cpuStringAgent("b"))
	ctx := context.Background()

	for b.Loop() {
		_, _ = chain.Run(ctx, "hello")
	}
}

func BenchmarkChain3_Noop(b *testing.B) {
	chain := agent.Chain3(noopStringAgent("a"), noopStringAgent("b"), noopStringAgent("c"))
	ctx := context.Background()

	for b.Loop() {
		_, _ = chain.Run(ctx, "hello")
	}
}

func BenchmarkChain4_Noop(b *testing.B) {
	chain := agent.Chain4(
		noopStringAgent("a"), noopStringAgent("b"),
		noopStringAgent("c"), noopStringAgent("d"),
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = chain.Run(ctx, "hello")
	}
}

func BenchmarkChain5_Noop(b *testing.B) {
	chain := agent.Chain5(
		noopStringAgent("a"), noopStringAgent("b"),
		noopStringAgent("c"), noopStringAgent("d"),
		noopStringAgent("e"),
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = chain.Run(ctx, "hello")
	}
}

// BenchmarkChainScaling measures Chain overhead as the pipeline length increases.
func BenchmarkChainScaling(b *testing.B) {
	for _, stages := range []int{2, 3, 4, 5} {
		b.Run(fmt.Sprintf("stages=%d", stages), func(b *testing.B) {
			// Build chain of the given length.
			var a agent.Agent[string, string]
			a = noopStringAgent("s0")
			for i := 1; i < stages; i++ {
				a = agent.Chain2(a, noopStringAgent(fmt.Sprintf("s%d", i)))
			}
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = a.Run(ctx, "x")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Erase (type erasure) benchmarks
// ---------------------------------------------------------------------------

func BenchmarkErase_Overhead(b *testing.B) {
	typed := noopStringAgent("typed")
	untyped := agent.Erase(typed)
	ctx := context.Background()

	b.Run("Typed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = typed.Run(ctx, "hello")
		}
	})
	b.Run("Untyped", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = untyped.RunUntyped(ctx, "hello")
		}
	})
}

// ---------------------------------------------------------------------------
// Decorator benchmarks
// ---------------------------------------------------------------------------

func BenchmarkWithRetry_NoFailure(b *testing.B) {
	// Retry wrapper with no failures — measures decorator overhead only.
	base := noopStringAgent("base")
	retried := agent.WithRetry(base, agent.WithMaxAttempts(3))
	ctx := context.Background()

	for b.Loop() {
		_, _ = retried.Run(ctx, "hello")
	}
}

func BenchmarkWithTimeout_NoTimeout(b *testing.B) {
	base := noopStringAgent("base")
	bounded := agent.WithTimeout(base, 5*time.Second)
	ctx := context.Background()

	for b.Loop() {
		_, _ = bounded.Run(ctx, "hello")
	}
}

func BenchmarkWithCache_Hit(b *testing.B) {
	callCount := 0
	base := agent.Func[string, string]("base", func(_ context.Context, s string) (string, error) {
		callCount++
		return strings.ToUpper(s), nil
	})
	cached := agent.WithCache(base, func(s string) string { return s })
	ctx := context.Background()

	// Warm up the cache.
	_, _ = cached.Run(ctx, "hello")

	for b.Loop() {
		_, _ = cached.Run(ctx, "hello")
	}
}

func BenchmarkWithCache_Miss(b *testing.B) {
	base := noopStringAgent("base")
	cached := agent.WithCache(base, func(s string) string { return s })
	ctx := context.Background()

	for i := 0; b.Loop(); i++ {
		// Each iteration uses a unique key → cache miss.
		_, _ = cached.Run(ctx, fmt.Sprintf("key-%d", i))
	}
}

// BenchmarkDecoratorStack measures the overhead of stacking all three decorators.
func BenchmarkDecoratorStack(b *testing.B) {
	base := noopStringAgent("base")
	stacked := agent.WithCache(
		agent.WithRetry(
			agent.WithTimeout(base, 5*time.Second),
			agent.WithMaxAttempts(3),
		),
		func(s string) string { return s },
	)
	ctx := context.Background()

	// Warm cache.
	_, _ = stacked.Run(ctx, "hello")

	for b.Loop() {
		_, _ = stacked.Run(ctx, "hello")
	}
}
