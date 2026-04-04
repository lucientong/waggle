package agent_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// TestWithTimeout_Success verifies a fast agent completes without timeout.
func TestWithTimeout_Success(t *testing.T) {
	a := agent.Func[string, string]("fast", func(_ context.Context, s string) (string, error) {
		return s + "-done", nil
	})

	bounded := agent.WithTimeout(a, 1*time.Second)
	result, err := bounded.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "hello-done" {
		t.Errorf("Run() = %q, want %q", result, "hello-done")
	}
}

// TestWithTimeout_Timeout verifies a slow agent is terminated by the timeout.
func TestWithTimeout_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout trigger test in short mode")
	}

	a := agent.Func[string, string]("slow", func(ctx context.Context, _ string) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "too-late", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	bounded := agent.WithTimeout(a, 50*time.Millisecond)
	_, err := bounded.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected timeout error, got nil")
	}

	var timeoutErr *agent.TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("error type = %T, want *agent.TimeoutError; err = %v", err, err)
	}
	if timeoutErr.AgentName != "slow" {
		t.Errorf("TimeoutError.AgentName = %q, want %q", timeoutErr.AgentName, "slow")
	}
	if timeoutErr.Duration != 50*time.Millisecond {
		t.Errorf("TimeoutError.Duration = %v, want %v", timeoutErr.Duration, 50*time.Millisecond)
	}
}

// TestWithTimeout_UnwrapDeadlineExceeded verifies errors.Is(err, context.DeadlineExceeded).
func TestWithTimeout_UnwrapDeadlineExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout unwrap test in short mode")
	}

	a := agent.Func[string, string]("slow", func(ctx context.Context, _ string) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "late", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	bounded := agent.WithTimeout(a, 10*time.Millisecond)
	_, err := bounded.Run(context.Background(), "input")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false; err = %v", err)
	}
}

// TestWithTimeout_ParentContextShorter verifies parent context deadline takes precedence.
func TestWithTimeout_ParentContextShorter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping parent context deadline test in short mode")
	}

	a := agent.Func[string, string]("slow", func(ctx context.Context, _ string) (string, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "late", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Parent context expires in 20ms, wrapper timeout is 1s — parent wins.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	bounded := agent.WithTimeout(a, 1*time.Second)
	start := time.Now()
	_, err := bounded.Run(ctx, "input")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	// Should complete well before 1s (the wrapper timeout).
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected early termination, but took %v", elapsed)
	}
}

// TestWithTimeout_Name verifies the name is prefixed with "timeout:".
func TestWithTimeout_Name(t *testing.T) {
	a := agent.Func[string, string]("base", func(_ context.Context, s string) (string, error) { return s, nil })
	bounded := agent.WithTimeout(a, time.Second)
	if bounded.Name() != "timeout:base" {
		t.Errorf("Name() = %q, want %q", bounded.Name(), "timeout:base")
	}
}
