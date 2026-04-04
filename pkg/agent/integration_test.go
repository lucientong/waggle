// Package agent_test contains integration tests that verify combinations of
// agents, chains, and wrappers work correctly together.
package agent_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// TestIntegration_ChainWithTimeout verifies Chain + Timeout wrapper works correctly.
// A slow first stage should be cut short, preventing the second stage from running.
func TestIntegration_ChainWithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration timing test in short mode")
	}

	slow := agent.Func[string, string]("slow", func(ctx context.Context, s string) (string, error) {
		select {
		case <-time.After(200 * time.Millisecond):
			return strings.ToUpper(s), nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	exclaim := agent.Func[string, string]("exclaim", func(_ context.Context, s string) (string, error) {
		return s + "!", nil
	})

	// Wrap the slow stage with a 50ms timeout, then chain with exclaim.
	pipeline := agent.Chain2(agent.WithTimeout(slow, 50*time.Millisecond), exclaim)

	_, err := pipeline.Run(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	var timeoutErr *agent.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Errorf("error type = %T, want *agent.TimeoutError", err)
	}
}

// TestIntegration_ChainWithRetry verifies Chain + Retry works correctly.
// A flaky first stage should be retried until success, then pass to the second stage.
func TestIntegration_ChainWithRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration retry test in short mode")
	}

	var calls atomic.Int32
	sentinel := errors.New("transient")

	flaky := agent.Func[string, string]("flaky", func(_ context.Context, s string) (string, error) {
		n := calls.Add(1)
		if n < 3 {
			return "", sentinel
		}
		return strings.ToUpper(s), nil
	})

	exclaim := agent.Func[string, string]("exclaim", func(_ context.Context, s string) (string, error) {
		return s + "!", nil
	})

	pipeline := agent.Chain2(
		agent.WithRetry(flaky, agent.WithMaxAttempts(5), agent.WithBaseDelay(1*time.Millisecond), agent.WithJitter(false)),
		exclaim,
	)

	result, err := pipeline.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "HELLO!" {
		t.Errorf("Run() = %q, want %q", result, "HELLO!")
	}
}

// TestIntegration_ChainWithCache verifies Chain + Cache works correctly.
// The second invocation should be served from cache.
func TestIntegration_ChainWithCache(t *testing.T) {
	var stage1Calls, stage2Calls atomic.Int32

	upper := agent.Func[string, string]("upper", func(_ context.Context, s string) (string, error) {
		stage1Calls.Add(1)
		return strings.ToUpper(s), nil
	})

	exclaim := agent.Func[string, string]("exclaim", func(_ context.Context, s string) (string, error) {
		stage2Calls.Add(1)
		return s + "!", nil
	})

	// Cache wraps the entire pipeline by caching the output of stage1 keyed on original input.
	pipeline := agent.Chain2(
		agent.WithCache(upper, func(s string) string { return s }),
		exclaim,
	)

	r1, err := pipeline.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("first Run() error: %v", err)
	}

	r2, err := pipeline.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("second Run() error: %v", err)
	}

	if r1 != r2 {
		t.Errorf("results differ: %q vs %q", r1, r2)
	}

	// stage1 should only be called once (cached), stage2 still called each time.
	if stage1Calls.Load() != 1 {
		t.Errorf("stage1 calls = %d, want 1", stage1Calls.Load())
	}
	if stage2Calls.Load() != 2 {
		t.Errorf("stage2 calls = %d, want 2", stage2Calls.Load())
	}
}

// TestIntegration_RetryThenTimeout verifies stacking Retry on top of Timeout.
// Each attempt is bounded by the timeout; all attempts failing produces RetryExhaustedError.
func TestIntegration_RetryThenTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration retry+timeout test in short mode")
	}

	slow := agent.Func[string, string]("slow", func(ctx context.Context, _ string) (string, error) {
		select {
		case <-time.After(100 * time.Millisecond):
			return "ok", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// WithTimeout wraps slow (each call has 20ms budget).
	// WithRetry tries 3 times — all will timeout.
	wrapped := agent.WithRetry(
		agent.WithTimeout(slow, 20*time.Millisecond),
		agent.WithMaxAttempts(3),
		agent.WithBaseDelay(1*time.Millisecond),
		agent.WithJitter(false),
	)

	_, err := wrapped.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be RetryExhaustedError wrapping TimeoutError.
	var retryErr *agent.RetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("error type = %T, want *agent.RetryExhaustedError", err)
	}
	if retryErr.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", retryErr.Attempts)
	}

	var timeoutErr *agent.TimeoutError
	if !errors.As(retryErr.LastErr, &timeoutErr) {
		t.Errorf("LastErr type = %T, want *agent.TimeoutError", retryErr.LastErr)
	}
}

// TestIntegration_Chain3_WithDecorators verifies a 3-stage chain where each stage
// has a different decorator applied.
func TestIntegration_Chain3_WithDecorators(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration chain3 decorators test in short mode")
	}

	fetch := agent.Func[string, string]("fetch", func(_ context.Context, url string) (string, error) {
		return "content:" + url, nil
	})

	summarize := agent.Func[string, string]("summarize", func(_ context.Context, content string) (string, error) {
		return "summary[" + content + "]", nil
	})

	review := agent.Func[string, string]("review", func(_ context.Context, summary string) (string, error) {
		return "reviewed:" + summary, nil
	})

	pipeline := agent.Chain3(
		agent.WithCache(fetch, func(url string) string { return url }),
		agent.WithTimeout(summarize, 1*time.Second),
		agent.WithRetry(review, agent.WithMaxAttempts(2), agent.WithBaseDelay(1*time.Millisecond)),
	)

	result, err := pipeline.Run(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	want := "reviewed:summary[content:https://example.com]"
	if result != want {
		t.Errorf("Run() = %q, want %q", result, want)
	}
}
