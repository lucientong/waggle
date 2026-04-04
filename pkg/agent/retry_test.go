package agent_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// TestWithRetry_SuccessFirstAttempt verifies no retry happens when agent succeeds.
func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	a := agent.Func[string, string]("echo", func(_ context.Context, s string) (string, error) {
		calls.Add(1)
		return s, nil
	})

	retried := agent.WithRetry(a, agent.WithMaxAttempts(3), agent.WithJitter(false))
	result, err := retried.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("Run() = %q, want %q", result, "hello")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
}

// TestWithRetry_SuccessAfterRetries verifies the agent succeeds after initial failures.
func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping retry timing test in short mode")
	}

	var calls atomic.Int32
	sentinel := errors.New("transient error")

	a := agent.Func[string, string]("flaky", func(_ context.Context, s string) (string, error) {
		n := calls.Add(1)
		if n < 3 {
			return "", sentinel
		}
		return s + "-ok", nil
	})

	retried := agent.WithRetry(a,
		agent.WithMaxAttempts(5),
		agent.WithBaseDelay(1*time.Millisecond),
		agent.WithJitter(false),
	)

	result, err := retried.Run(context.Background(), "waggle")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "waggle-ok" {
		t.Errorf("Run() = %q, want %q", result, "waggle-ok")
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

// TestWithRetry_Exhausted verifies RetryExhaustedError is returned when all attempts fail.
func TestWithRetry_Exhausted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping retry exhaustion test in short mode")
	}

	sentinel := errors.New("always fails")
	var calls atomic.Int32

	a := agent.Func[string, string]("always-fail", func(_ context.Context, _ string) (string, error) {
		calls.Add(1)
		return "", sentinel
	})

	retried := agent.WithRetry(a,
		agent.WithMaxAttempts(3),
		agent.WithBaseDelay(1*time.Millisecond),
		agent.WithJitter(false),
	)

	_, err := retried.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}

	var retryErr *agent.RetryExhaustedError
	if !errors.As(err, &retryErr) {
		t.Fatalf("error type = %T, want *agent.RetryExhaustedError", err)
	}
	if retryErr.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", retryErr.Attempts)
	}
	if !errors.Is(retryErr.LastErr, sentinel) {
		t.Errorf("LastErr = %v, want %v", retryErr.LastErr, sentinel)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 calls, got %d", calls.Load())
	}
}

// TestWithRetry_ContextCanceled verifies that context cancellation stops retrying.
func TestWithRetry_ContextCanceled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping retry cancel test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32

	a := agent.Func[string, string]("slow-fail", func(_ context.Context, _ string) (string, error) {
		n := calls.Add(1)
		if n == 2 {
			cancel() // cancel on second attempt
		}
		return "", errors.New("transient")
	})

	retried := agent.WithRetry(a,
		agent.WithMaxAttempts(10),
		agent.WithBaseDelay(1*time.Millisecond),
		agent.WithJitter(false),
	)

	_, err := retried.Run(ctx, "input")
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	// Should get context.Canceled, not RetryExhaustedError
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestWithRetry_Name verifies the name is prefixed with "retry:".
func TestWithRetry_Name(t *testing.T) {
	a := agent.Func[string, string]("base", func(_ context.Context, s string) (string, error) { return s, nil })
	retried := agent.WithRetry(a)
	if retried.Name() != "retry:base" {
		t.Errorf("Name() = %q, want %q", retried.Name(), "retry:base")
	}
}

// TestWithRetry_UnwrapError verifies errors.Is traverses through RetryExhaustedError.
func TestWithRetry_UnwrapError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping retry unwrap test in short mode")
	}

	sentinel := errors.New("root cause")
	a := agent.Func[string, string]("fail", func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})

	retried := agent.WithRetry(a,
		agent.WithMaxAttempts(2),
		agent.WithBaseDelay(1*time.Millisecond),
		agent.WithJitter(false),
	)

	_, err := retried.Run(context.Background(), "")
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false, want true; err = %v", err)
	}
}
