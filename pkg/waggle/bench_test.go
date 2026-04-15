package waggle_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func noopAgent(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return s, nil
	})
}

func cpuAgent(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s) + "!", nil
	})
}

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
// Parallel benchmarks
// ---------------------------------------------------------------------------

func BenchmarkParallel_2Agents(b *testing.B) {
	pa := waggle.Parallel("par2", noopAgent("a"), noopAgent("b"))
	ctx := context.Background()

	for b.Loop() {
		_, _ = pa.Run(ctx, "hello")
	}
}

func BenchmarkParallel_5Agents(b *testing.B) {
	agents := make([]agent.Agent[string, string], 5)
	for i := range agents {
		agents[i] = noopAgent(fmt.Sprintf("a%d", i))
	}
	pa := waggle.Parallel("par5", agents...)
	ctx := context.Background()

	for b.Loop() {
		_, _ = pa.Run(ctx, "hello")
	}
}

func BenchmarkParallel_10Agents(b *testing.B) {
	agents := make([]agent.Agent[string, string], 10)
	for i := range agents {
		agents[i] = noopAgent(fmt.Sprintf("a%d", i))
	}
	pa := waggle.Parallel("par10", agents...)
	ctx := context.Background()

	for b.Loop() {
		_, _ = pa.Run(ctx, "hello")
	}
}

func BenchmarkParallel_CPU(b *testing.B) {
	pa := waggle.Parallel("par-cpu", cpuAgent("a"), cpuAgent("b"), cpuAgent("c"))
	ctx := context.Background()

	for b.Loop() {
		_, _ = pa.Run(ctx, "hello waggle")
	}
}

// BenchmarkParallelScaling measures goroutine overhead as the fan-out width increases.
func BenchmarkParallelScaling(b *testing.B) {
	for _, n := range []int{1, 2, 5, 10, 20, 50} {
		b.Run(fmt.Sprintf("agents=%d", n), func(b *testing.B) {
			agents := make([]agent.Agent[string, string], n)
			for i := range agents {
				agents[i] = noopAgent(fmt.Sprintf("a%d", i))
			}
			pa := waggle.Parallel("par", agents...)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = pa.Run(ctx, "x")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Race benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRace_2Agents(b *testing.B) {
	ra := waggle.Race("race2", noopAgent("a"), noopAgent("b"))
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "hello")
	}
}

func BenchmarkRace_5Agents(b *testing.B) {
	agents := make([]agent.Agent[string, string], 5)
	for i := range agents {
		agents[i] = noopAgent(fmt.Sprintf("a%d", i))
	}
	ra := waggle.Race("race5", agents...)
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "hello")
	}
}

// BenchmarkRace_LatencyHedge simulates a real latency hedging scenario
// where agents have different sleep durations.
func BenchmarkRace_LatencyHedge(b *testing.B) {
	ra := waggle.Race("hedge",
		sleepAgent("fast", 1*time.Millisecond),
		sleepAgent("medium", 10*time.Millisecond),
		sleepAgent("slow", 100*time.Millisecond),
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "hello")
	}
}

// ---------------------------------------------------------------------------
// Vote benchmarks
// ---------------------------------------------------------------------------

func BenchmarkVote_3Agents(b *testing.B) {
	// All agents return the same value → majority always found.
	va := waggle.Vote("vote3", waggle.MajorityVote[string](),
		noopAgent("j1"), noopAgent("j2"), noopAgent("j3"),
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = va.Run(ctx, "hello")
	}
}

func BenchmarkVote_5Agents(b *testing.B) {
	agents := make([]agent.Agent[string, string], 5)
	for i := range agents {
		agents[i] = noopAgent(fmt.Sprintf("j%d", i))
	}
	va := waggle.Vote("vote5", waggle.MajorityVote[string](), agents...)
	ctx := context.Background()

	for b.Loop() {
		_, _ = va.Run(ctx, "hello")
	}
}

// ---------------------------------------------------------------------------
// Router benchmarks
// ---------------------------------------------------------------------------

func BenchmarkRouter_2Branches(b *testing.B) {
	routeFn := func(_ context.Context, s string) (string, error) {
		if strings.HasPrefix(s, "a") {
			return "branch-a", nil
		}
		return "branch-b", nil
	}
	ra := waggle.Router("router2", routeFn,
		map[string]agent.Agent[string, string]{
			"branch-a": noopAgent("a"),
			"branch-b": noopAgent("b"),
		},
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "alpha")
	}
}

func BenchmarkRouter_WithFallback(b *testing.B) {
	routeFn := func(_ context.Context, _ string) (string, error) {
		return "unknown", nil
	}
	ra := waggle.Router("router-fb", routeFn,
		map[string]agent.Agent[string, string]{
			"known": noopAgent("known"),
		},
		waggle.WithFallback[string, string](noopAgent("fallback")),
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "anything")
	}
}

// ---------------------------------------------------------------------------
// Loop benchmarks
// ---------------------------------------------------------------------------

func BenchmarkLoop_1Iteration(b *testing.B) {
	// Condition returns false immediately → only init runs + one condition check.
	la := waggle.Loop("loop1",
		noopAgent("init"),
		noopAgent("body"),
		func(_ string) bool { return false },
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = la.Run(ctx, "hello")
	}
}

func BenchmarkLoop_5Iterations(b *testing.B) {
	count := 0
	la := waggle.Loop("loop5",
		noopAgent("init"),
		noopAgent("body"),
		func(_ string) bool {
			count++
			return count%5 != 0 // run 4 body iterations then stop
		},
		waggle.WithMaxIterations[string, string](10),
	)
	ctx := context.Background()

	for b.Loop() {
		count = 0
		_, _ = la.Run(ctx, "hello")
	}
}

func BenchmarkLoop_MaxIterations(b *testing.B) {
	// Always returns true → runs to maxIterations.
	for _, max := range []int{5, 10, 20} {
		b.Run(fmt.Sprintf("max=%d", max), func(b *testing.B) {
			la := waggle.Loop("loop",
				noopAgent("init"),
				noopAgent("body"),
				func(_ string) bool { return true },
				waggle.WithMaxIterations[string, string](max),
			)
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = la.Run(ctx, "hello")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DAG benchmarks
// ---------------------------------------------------------------------------

func BenchmarkDAG_LinearChain(b *testing.B) {
	// Build a linear DAG: A → B → C → D → E
	for _, n := range []int{5, 10, 20} {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := waggle.New()
				agents := make([]agent.UntypedAgent, n)
				for j := range n {
					agents[j] = agent.Erase(noopAgent(fmt.Sprintf("n%d", j)))
				}
				_ = w.Register(agents...)
				for j := 0; j < n-1; j++ {
					_ = w.Connect(fmt.Sprintf("n%d", j), fmt.Sprintf("n%d", j+1))
				}
			}
		})
	}
}

func BenchmarkDAG_DiamondTopology(b *testing.B) {
	// Diamond: A → {B, C} → D

	for b.Loop() {
		w := waggle.New()
		_ = w.Register(
			agent.Erase(noopAgent("A")),
			agent.Erase(noopAgent("B")),
			agent.Erase(noopAgent("C")),
			agent.Erase(noopAgent("D")),
		)
		_ = w.Connect("A", "B")
		_ = w.Connect("A", "C")
		_ = w.Connect("B", "D")
		_ = w.Connect("C", "D")
	}
}

// BenchmarkDAG_RunLinear benchmarks end-to-end execution of a linear pipeline.
func BenchmarkDAG_RunLinear(b *testing.B) {
	for _, n := range []int{3, 5, 10} {
		b.Run(fmt.Sprintf("nodes=%d", n), func(b *testing.B) {
			w := waggle.New()
			agents := make([]agent.UntypedAgent, n)
			for j := range n {
				agents[j] = agent.Erase(noopAgent(fmt.Sprintf("n%d", j)))
			}
			_ = w.Register(agents...)
			for j := 0; j < n-1; j++ {
				_ = w.Connect(fmt.Sprintf("n%d", j), fmt.Sprintf("n%d", j+1))
			}
			ctx := context.Background()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = w.RunFrom(ctx, "n0", "hello")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Composition benchmarks — combining patterns
// ---------------------------------------------------------------------------

// BenchmarkComposition_ParallelThenChain measures a Parallel fan-out followed by
// a join agent and a final processing step.
func BenchmarkComposition_ParallelThenChain(b *testing.B) {
	pa := waggle.Parallel("par", noopAgent("a1"), noopAgent("a2"), noopAgent("a3"))
	join := agent.Func[waggle.ParallelResults[string], string]("join",
		func(_ context.Context, r waggle.ParallelResults[string]) (string, error) {
			return strings.Join(r.Results, ","), nil
		},
	)
	pipeline := agent.Chain2(pa, join)
	ctx := context.Background()

	for b.Loop() {
		_, _ = pipeline.Run(ctx, "hello")
	}
}

// BenchmarkComposition_RouterThenLoop measures a Router that dispatches to
// different Loop agents.
func BenchmarkComposition_RouterThenLoop(b *testing.B) {
	loop1 := waggle.Loop("loop1",
		noopAgent("init1"), noopAgent("body1"),
		func(_ string) bool { return false },
	)
	loop2 := waggle.Loop("loop2",
		noopAgent("init2"), noopAgent("body2"),
		func(_ string) bool { return false },
	)
	routeFn := func(_ context.Context, s string) (string, error) {
		if len(s) > 5 {
			return "long", nil
		}
		return "short", nil
	}
	ra := waggle.Router("router", routeFn,
		map[string]agent.Agent[string, string]{
			"long":  loop1,
			"short": loop2,
		},
	)
	ctx := context.Background()

	for b.Loop() {
		_, _ = ra.Run(ctx, "hello waggle benchmark")
	}
}
