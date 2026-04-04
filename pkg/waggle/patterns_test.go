package waggle_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// ---- Parallel ---------------------------------------------------------------

func TestParallel_AllSucceed(t *testing.T) {
	a1 := agent.Func[string, string]("a1", func(_ context.Context, s string) (string, error) {
		return s + "-1", nil
	})
	a2 := agent.Func[string, string]("a2", func(_ context.Context, s string) (string, error) {
		return s + "-2", nil
	})
	a3 := agent.Func[string, string]("a3", func(_ context.Context, s string) (string, error) {
		return s + "-3", nil
	})

	pa := waggle.Parallel("test", a1, a2, a3)
	res, err := pa.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(res.Results) != 3 {
		t.Errorf("Results length = %d, want 3", len(res.Results))
	}
}

func TestParallel_PartialFailure(t *testing.T) {
	good := agent.Func[string, string]("good", func(_ context.Context, s string) (string, error) {
		return s, nil
	})
	bad := agent.Func[string, string]("bad", func(_ context.Context, _ string) (string, error) {
		return "", errors.New("bad agent")
	})

	pa := waggle.Parallel("test", good, bad)
	res, err := pa.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Run() unexpected error (parallel never errors out): %v", err)
	}
	// One should succeed, one should fail.
	successCount := 0
	errCount := 0
	for i := range res.Results {
		if res.Errors[i] == nil {
			successCount++
		} else {
			errCount++
		}
	}
	if successCount != 1 || errCount != 1 {
		t.Errorf("expected 1 success and 1 error, got %d success and %d errors", successCount, errCount)
	}
}

// ---- Race -------------------------------------------------------------------

func TestRace_FastestWins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race timing test in short mode")
	}

	var fastCalled, slowCalled atomic.Bool

	fast := agent.Func[string, string]("fast", func(_ context.Context, s string) (string, error) {
		fastCalled.Store(true)
		return s + "-fast", nil
	})
	slow := agent.Func[string, string]("slow", func(ctx context.Context, s string) (string, error) {
		slowCalled.Store(true)
		select {
		case <-time.After(200 * time.Millisecond):
			return s + "-slow", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	ra := waggle.Race("test", fast, slow)
	result, err := ra.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "input-fast" {
		t.Errorf("Run() = %q, want %q", result, "input-fast")
	}
	if !fastCalled.Load() {
		t.Error("fast agent was not called")
	}
}

func TestRace_AllFail(t *testing.T) {
	a1 := agent.Func[string, string]("a1", func(_ context.Context, _ string) (string, error) {
		return "", errors.New("a1 failed")
	})
	a2 := agent.Func[string, string]("a2", func(_ context.Context, _ string) (string, error) {
		return "", errors.New("a2 failed")
	})

	ra := waggle.Race("test", a1, a2)
	_, err := ra.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error when all agents fail, got nil")
	}
}

func TestRace_FallbackWins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race fallback test in short mode")
	}

	primary := agent.Func[string, string]("primary", func(ctx context.Context, s string) (string, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return s + "-primary", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})
	backup := agent.Func[string, string]("backup", func(_ context.Context, s string) (string, error) {
		return s + "-backup", nil
	})

	ra := waggle.Race("test", primary, backup)
	result, err := ra.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "x-backup" {
		t.Errorf("Run() = %q, want %q", result, "x-backup")
	}
}

// ---- Vote -------------------------------------------------------------------

func TestVote_MajorityWins(t *testing.T) {
	// Two judges return "good", one returns "bad". "good" should win.
	good1 := agent.Func[string, string]("g1", func(_ context.Context, _ string) (string, error) {
		return "good", nil
	})
	good2 := agent.Func[string, string]("g2", func(_ context.Context, _ string) (string, error) {
		return "good", nil
	})
	bad := agent.Func[string, string]("bad", func(_ context.Context, _ string) (string, error) {
		return "bad", nil
	})

	va := waggle.Vote("test", waggle.MajorityVote[string](), good1, good2, bad)
	result, err := va.Run(context.Background(), "article")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "good" {
		t.Errorf("Run() = %q, want %q", result, "good")
	}
}

func TestVote_NoMajority(t *testing.T) {
	// All judges return different values — no majority.
	a := agent.Func[string, string]("a", func(_ context.Context, _ string) (string, error) { return "x", nil })
	b := agent.Func[string, string]("b", func(_ context.Context, _ string) (string, error) { return "y", nil })
	c := agent.Func[string, string]("c", func(_ context.Context, _ string) (string, error) { return "z", nil })

	va := waggle.Vote("test", waggle.MajorityVote[string](), a, b, c)
	_, err := va.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error when no majority, got nil")
	}
}

func TestVote_AllFail(t *testing.T) {
	a := agent.Func[string, string]("a", func(_ context.Context, _ string) (string, error) {
		return "", errors.New("fail")
	})

	va := waggle.Vote("test", waggle.MajorityVote[string](), a)
	_, err := va.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error when all agents fail, got nil")
	}
}

// ---- Router -----------------------------------------------------------------

func TestRouter_RouteToCorrectBranch(t *testing.T) {
	bugHandler := agent.Func[string, string]("bug", func(_ context.Context, s string) (string, error) {
		return "bug:" + s, nil
	})
	featureHandler := agent.Func[string, string]("feature", func(_ context.Context, s string) (string, error) {
		return "feature:" + s, nil
	})

	routeFn := func(_ context.Context, s string) (string, error) {
		if strings.Contains(s, "bug") {
			return "bug", nil
		}
		return "feature", nil
	}

	ra := waggle.Router("triage", routeFn, map[string]agent.Agent[string, string]{
		"bug":     bugHandler,
		"feature": featureHandler,
	})

	r1, _ := ra.Run(context.Background(), "fix the bug in login")
	if r1 != "bug:fix the bug in login" {
		t.Errorf("router result = %q, want prefix 'bug:'", r1)
	}

	r2, _ := ra.Run(context.Background(), "add new feature")
	if r2 != "feature:add new feature" {
		t.Errorf("router result = %q, want prefix 'feature:'", r2)
	}
}

func TestRouter_UnknownKey(t *testing.T) {
	routeFn := func(_ context.Context, _ string) (string, error) {
		return "unknown-key", nil
	}

	ra := waggle.Router("test", routeFn, map[string]agent.Agent[string, string]{
		"known": agent.Func[string, string]("known", func(_ context.Context, s string) (string, error) { return s, nil }),
	})

	_, err := ra.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error for unknown key, got nil")
	}
}

func TestRouter_Fallback(t *testing.T) {
	routeFn := func(_ context.Context, _ string) (string, error) {
		return "nonexistent", nil
	}
	fallback := agent.Func[string, string]("fallback", func(_ context.Context, s string) (string, error) {
		return "default:" + s, nil
	})

	ra := waggle.Router("test", routeFn,
		map[string]agent.Agent[string, string]{},
		waggle.WithFallback[string, string](fallback),
	)

	result, err := ra.Run(context.Background(), "anything")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "default:anything" {
		t.Errorf("Run() = %q, want %q", result, "default:anything")
	}
}

// ---- Loop -------------------------------------------------------------------

func TestLoop_TerminatesWhenConditionFalse(t *testing.T) {
	// Start with 0, increment by 1 each iteration. Stop when >= 5.
	init := agent.Func[int, int]("init", func(_ context.Context, n int) (int, error) {
		return n, nil
	})
	body := agent.Func[int, int]("body", func(_ context.Context, n int) (int, error) {
		return n + 1, nil
	})
	condition := func(n int) bool { return n < 5 }

	la := waggle.Loop("counter", init, body, condition, waggle.WithMaxIterations[int, int](20))
	result, err := la.Run(context.Background(), 0)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != 5 {
		t.Errorf("Run() = %d, want 5", result)
	}
}

func TestLoop_NeverIterates(t *testing.T) {
	// Condition is already false from the start — loop should return init result immediately.
	init := agent.Func[string, string]("init", func(_ context.Context, s string) (string, error) {
		return s + "-init", nil
	})
	body := agent.Func[string, string]("body", func(_ context.Context, s string) (string, error) {
		return s + "-iterated", nil
	})
	condition := func(_ string) bool { return false } // always stop

	la := waggle.Loop("no-loop", init, body, condition)
	result, err := la.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "input-init" {
		t.Errorf("Run() = %q, want %q", result, "input-init")
	}
}

func TestLoop_MaxIterationsExceeded(t *testing.T) {
	// Condition never becomes false — should hit max iterations.
	init := agent.Func[int, int]("init", func(_ context.Context, n int) (int, error) { return n, nil })
	body := agent.Func[int, int]("body", func(_ context.Context, n int) (int, error) { return n + 1, nil })
	condition := func(_ int) bool { return true } // never stop

	la := waggle.Loop("infinite", init, body, condition, waggle.WithMaxIterations[int, int](3))
	_, err := la.Run(context.Background(), 0)
	if err == nil {
		t.Fatal("Run() expected error for max iterations, got nil")
	}
}

func TestLoop_InitError(t *testing.T) {
	sentinel := errors.New("init failed")
	init := agent.Func[string, string]("init", func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})
	body := agent.Func[string, string]("body", func(_ context.Context, s string) (string, error) { return s, nil })

	la := waggle.Loop("test", init, body, func(_ string) bool { return true })
	_, err := la.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want to wrap sentinel", err)
	}
}

func TestLoop_BodyError(t *testing.T) {
	sentinel := errors.New("body failed")
	calls := 0
	init := agent.Func[int, int]("init", func(_ context.Context, n int) (int, error) { return n, nil })
	body := agent.Func[int, int]("body", func(_ context.Context, n int) (int, error) {
		calls++
		if calls >= 2 {
			return n, sentinel
		}
		return n + 1, nil
	})
	condition := func(_ int) bool { return true }

	la := waggle.Loop("test", init, body, condition, waggle.WithMaxIterations[int, int](10))
	_, err := la.Run(context.Background(), 0)
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want to wrap sentinel", err)
	}
}

// ---- Composability ----------------------------------------------------------

// TestPatterns_Composable verifies patterns can be composed with agent.Chain2.
func TestPatterns_Composable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping composability test in short mode")
	}

	// Parallel that concatenates results.
	a1 := agent.Func[string, string]("a1", func(_ context.Context, s string) (string, error) { return s + "1", nil })
	a2 := agent.Func[string, string]("a2", func(_ context.Context, s string) (string, error) { return s + "2", nil })
	pa := waggle.Parallel("parallel", a1, a2)

	// Second stage: join results.
	join := agent.Func[waggle.ParallelResults[string], string]("join",
		func(_ context.Context, r waggle.ParallelResults[string]) (string, error) {
			return fmt.Sprintf("[%s,%s]", r.Results[0], r.Results[1]), nil
		})

	// Chain Parallel -> join
	pipeline := agent.Chain2(pa, join)
	result, err := pipeline.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "[x1,x2]" {
		t.Errorf("Run() = %q, want %q", result, "[x1,x2]")
	}
}
