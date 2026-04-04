package agent_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

// TestWithCache_CacheMiss verifies the underlying agent is called on first access.
func TestWithCache_CacheMiss(t *testing.T) {
	var calls atomic.Int32
	a := agent.Func[string, string]("compute", func(_ context.Context, s string) (string, error) {
		calls.Add(1)
		return s + "-computed", nil
	})

	cached := agent.WithCache(a, func(s string) string { return s })
	result, err := cached.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "hello-computed" {
		t.Errorf("Run() = %q, want %q", result, "hello-computed")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
}

// TestWithCache_CacheHit verifies the underlying agent is only called once for the same key.
func TestWithCache_CacheHit(t *testing.T) {
	var calls atomic.Int32
	a := agent.Func[string, string]("compute", func(_ context.Context, s string) (string, error) {
		calls.Add(1)
		return s + "-computed", nil
	})

	cached := agent.WithCache(a, func(s string) string { return s })

	// First call: cache miss
	r1, err := cached.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("first Run() unexpected error: %v", err)
	}

	// Second call: cache hit — should return same result, no new call
	r2, err := cached.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("second Run() unexpected error: %v", err)
	}

	if r1 != r2 {
		t.Errorf("cache hit returned different result: %q vs %q", r1, r2)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 underlying call, got %d", calls.Load())
	}
}

// TestWithCache_DifferentKeys verifies different keys are cached independently.
func TestWithCache_DifferentKeys(t *testing.T) {
	var calls atomic.Int32
	a := agent.Func[string, string]("echo", func(_ context.Context, s string) (string, error) {
		calls.Add(1)
		return s, nil
	})

	cached := agent.WithCache(a, func(s string) string { return s })

	cached.Run(context.Background(), "foo") //nolint
	cached.Run(context.Background(), "bar") //nolint
	cached.Run(context.Background(), "foo") // cache hit
	cached.Run(context.Background(), "bar") // cache hit

	if calls.Load() != 2 {
		t.Errorf("expected 2 underlying calls (foo + bar), got %d", calls.Load())
	}
}

// TestWithCache_ErrorCached verifies that errors are cached too.
func TestWithCache_ErrorCached(t *testing.T) {
	sentinel := errors.New("compute failed")
	var calls atomic.Int32

	a := agent.Func[string, string]("fail", func(_ context.Context, _ string) (string, error) {
		calls.Add(1)
		return "", sentinel
	})

	cached := agent.WithCache(a, func(s string) string { return s })

	_, err1 := cached.Run(context.Background(), "key")
	_, err2 := cached.Run(context.Background(), "key") // should be cached error

	if !errors.Is(err1, sentinel) || !errors.Is(err2, sentinel) {
		t.Errorf("expected sentinel error on both calls")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 underlying call (error cached), got %d", calls.Load())
	}
}

// TestWithCache_ConcurrentSafe verifies concurrent access does not cause data races.
func TestWithCache_ConcurrentSafe(t *testing.T) {
	var calls atomic.Int32
	a := agent.Func[string, string]("compute", func(_ context.Context, s string) (string, error) {
		calls.Add(1)
		return s, nil
	})

	cached := agent.WithCache(a, func(s string) string { return s })

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cached.Run(context.Background(), "shared-key") //nolint
		}()
	}

	wg.Wait()
	// All goroutines shared the same key — underlying agent may be called more than once
	// due to concurrent cache misses before the first store, but must not panic or race.
	t.Logf("underlying calls for concurrent test: %d", calls.Load())
}

// TestWithCache_Name verifies the name is prefixed with "cache:".
func TestWithCache_Name(t *testing.T) {
	a := agent.Func[string, string]("base", func(_ context.Context, s string) (string, error) { return s, nil })
	cached := agent.WithCache(a, func(s string) string { return s })
	if cached.Name() != "cache:base" {
		t.Errorf("Name() = %q, want %q", cached.Name(), "cache:base")
	}
}
