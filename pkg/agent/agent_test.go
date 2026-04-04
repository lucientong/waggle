package agent_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

func TestFuncAgent_Name(t *testing.T) {
	a := agent.Func("my-agent", func(_ context.Context, s string) (string, error) {
		return s, nil
	})
	if got := a.Name(); got != "my-agent" {
		t.Errorf("Name() = %q, want %q", got, "my-agent")
	}
}

func TestFuncAgent_Run_Success(t *testing.T) {
	a := agent.Func("upper", func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})

	result, err := a.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "HELLO" {
		t.Errorf("Run() = %q, want %q", result, "HELLO")
	}
}

func TestFuncAgent_Run_Error(t *testing.T) {
	sentinel := errors.New("something went wrong")
	a := agent.Func("failing", func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	})

	_, err := a.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Run() error = %v, want %v", err, sentinel)
	}
}

func TestFuncAgent_Run_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	a := agent.Func("ctx-aware", func(ctx context.Context, s string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return s, nil
	})

	_, err := a.Run(ctx, "hello")
	if err == nil {
		t.Fatal("Run() expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestFuncAgent_IntTypes(t *testing.T) {
	double := agent.Func[int, int]("double", func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	})

	result, err := double.Run(context.Background(), 21)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("Run() = %d, want 42", result)
	}
}

// TestErase verifies that Erase wraps a typed agent and RunUntyped works correctly.
func TestErase_Success(t *testing.T) {
	a := agent.Func("echo", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	untyped := agent.Erase(a)
	if untyped.Name() != "echo" {
		t.Errorf("Name() = %q, want %q", untyped.Name(), "echo")
	}

	out, err := untyped.RunUntyped(context.Background(), "waggle")
	if err != nil {
		t.Fatalf("RunUntyped() unexpected error: %v", err)
	}
	if out != "waggle" {
		t.Errorf("RunUntyped() = %v, want %q", out, "waggle")
	}
}

// TestErase_TypeMismatch verifies that RunUntyped returns ErrTypeMismatch for wrong input types.
func TestErase_TypeMismatch(t *testing.T) {
	a := agent.Func("echo", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	untyped := agent.Erase(a)
	_, err := untyped.RunUntyped(context.Background(), 42) // int instead of string
	if err == nil {
		t.Fatal("RunUntyped() expected type mismatch error, got nil")
	}

	var mismatch *agent.ErrTypeMismatch
	if !errors.As(err, &mismatch) {
		t.Errorf("RunUntyped() error type = %T, want *agent.ErrTypeMismatch", err)
	}
	if mismatch.AgentName != "echo" {
		t.Errorf("ErrTypeMismatch.AgentName = %q, want %q", mismatch.AgentName, "echo")
	}
}
