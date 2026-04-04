package agent_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

// helpers for chain tests

func makeUpper(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})
}

func makeExclaim(name string) agent.Agent[string, string] {
	return agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return s + "!", nil
	})
}

func makeStringToInt(name string) agent.Agent[string, int] {
	return agent.Func[string, int](name, func(_ context.Context, s string) (int, error) {
		return len(s), nil
	})
}

func makeIntToString(name string) agent.Agent[int, string] {
	return agent.Func[int, string](name, func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("%d", n), nil
	})
}

func makeFailer[I, O any](name string, sentinel error) agent.Agent[I, O] {
	return agent.Func[I, O](name, func(_ context.Context, _ I) (O, error) {
		var zero O
		return zero, sentinel
	})
}

// TestChain2_Name verifies the composite name format.
func TestChain2_Name(t *testing.T) {
	a := makeUpper("upper")
	b := makeExclaim("exclaim")
	chain := agent.Chain2(a, b)

	name := chain.Name()
	if !strings.Contains(name, "upper") || !strings.Contains(name, "exclaim") {
		t.Errorf("Chain2.Name() = %q, expected both agent names to be present", name)
	}
}

// TestChain2_Success verifies two same-type agents can be chained.
func TestChain2_Success(t *testing.T) {
	chain := agent.Chain2(makeUpper("upper"), makeExclaim("exclaim"))

	result, err := chain.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Chain2.Run() unexpected error: %v", err)
	}
	if result != "HELLO!" {
		t.Errorf("Chain2.Run() = %q, want %q", result, "HELLO!")
	}
}

// TestChain2_TypeTransition verifies string -> int type transition.
func TestChain2_TypeTransition(t *testing.T) {
	chain := agent.Chain2(makeStringToInt("len"), makeIntToString("fmt"))

	result, err := chain.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Chain2.Run() unexpected error: %v", err)
	}
	if result != "5" {
		t.Errorf("Chain2.Run() = %q, want %q", result, "5")
	}
}

// TestChain2_ErrorShortCircuit verifies that first agent error stops execution.
func TestChain2_ErrorShortCircuit_First(t *testing.T) {
	sentinel := errors.New("first failed")
	first := makeFailer[string, string]("first", sentinel)
	second := makeExclaim("second")

	chain := agent.Chain2(first, second)
	_, err := chain.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Chain2.Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Chain2.Run() error = %v, want to wrap %v", err, sentinel)
	}
}

// TestChain2_ErrorShortCircuit verifies that second agent error is propagated.
func TestChain2_ErrorShortCircuit_Second(t *testing.T) {
	sentinel := errors.New("second failed")
	first := makeUpper("first")
	second := makeFailer[string, string]("second", sentinel)

	chain := agent.Chain2(first, second)
	_, err := chain.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("Chain2.Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("Chain2.Run() error = %v, want to wrap %v", err, sentinel)
	}
}

// TestChain2_ContextCanceled verifies that a pre-canceled context causes immediate return.
func TestChain2_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	chain := agent.Chain2(makeUpper("upper"), makeExclaim("exclaim"))
	_, err := chain.Run(ctx, "hello")
	if err == nil {
		t.Fatal("Chain2.Run() expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Chain2.Run() error = %v, want context.Canceled", err)
	}
}

// TestChain3_Success verifies three agents can be chained.
func TestChain3_Success(t *testing.T) {
	trim := agent.Func[string, string]("trim", func(_ context.Context, s string) (string, error) {
		return strings.TrimSpace(s), nil
	})
	upper := makeUpper("upper")
	exclaim := makeExclaim("exclaim")

	chain := agent.Chain3(trim, upper, exclaim)
	result, err := chain.Run(context.Background(), "  waggle  ")
	if err != nil {
		t.Fatalf("Chain3.Run() unexpected error: %v", err)
	}
	if result != "WAGGLE!" {
		t.Errorf("Chain3.Run() = %q, want %q", result, "WAGGLE!")
	}
}

// TestChain4_Success verifies four agents can be chained.
func TestChain4_Success(t *testing.T) {
	a := agent.Func[int, int]("add1", func(_ context.Context, n int) (int, error) { return n + 1, nil })
	b := agent.Func[int, int]("mul2", func(_ context.Context, n int) (int, error) { return n * 2, nil })
	c := agent.Func[int, int]("sub3", func(_ context.Context, n int) (int, error) { return n - 3, nil })
	d := agent.Func[int, string]("fmt", func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("%d", n), nil
	})

	// input=5: add1->6, mul2->12, sub3->9, fmt->"9"
	chain := agent.Chain4(a, b, c, d)
	result, err := chain.Run(context.Background(), 5)
	if err != nil {
		t.Fatalf("Chain4.Run() unexpected error: %v", err)
	}
	if result != "9" {
		t.Errorf("Chain4.Run() = %q, want %q", result, "9")
	}
}

// TestChain5_Success verifies five agents can be chained.
func TestChain5_Success(t *testing.T) {
	add := func(n int) agent.Agent[int, int] {
		return agent.Func[int, int](fmt.Sprintf("add%d", n), func(_ context.Context, x int) (int, error) {
			return x + n, nil
		})
	}
	toString := agent.Func[int, string]("str", func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("%d", n), nil
	})

	// input=0: +1=1, +2=3, +3=6, +4=10, str="10"
	chain := agent.Chain5(add(1), add(2), add(3), add(4), toString)
	result, err := chain.Run(context.Background(), 0)
	if err != nil {
		t.Fatalf("Chain5.Run() unexpected error: %v", err)
	}
	if result != "10" {
		t.Errorf("Chain5.Run() = %q, want %q", result, "10")
	}
}
