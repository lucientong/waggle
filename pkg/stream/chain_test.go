package stream

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
)

func TestObservableChain2_Success(t *testing.T) {
	upper := agent.Func[string, string]("upper", func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})
	exclaim := agent.Func[string, string]("exclaim", func(_ context.Context, s string) (string, error) {
		return s + "!!!", nil
	})

	collector := &Collector{}
	chain := ObservableChain2(upper, exclaim, collector)

	result, err := chain.Run(context.Background(), "hello")
	if err != nil {
		t.Fatal(err)
	}
	if result != "HELLO!!!" {
		t.Errorf("expected HELLO!!!, got %q", result)
	}

	// Should have 4 steps: started+completed for each agent.
	if len(collector.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d: %+v", len(collector.Steps), collector.Steps)
	}
	if collector.Steps[0].Type != StepStarted || collector.Steps[0].AgentName != "upper" {
		t.Errorf("step 0: %+v", collector.Steps[0])
	}
	if collector.Steps[1].Type != StepCompleted || collector.Steps[1].AgentName != "upper" {
		t.Errorf("step 1: %+v", collector.Steps[1])
	}
	if collector.Steps[2].Type != StepStarted || collector.Steps[2].AgentName != "exclaim" {
		t.Errorf("step 2: %+v", collector.Steps[2])
	}
	if collector.Steps[3].Type != StepCompleted || collector.Steps[3].AgentName != "exclaim" {
		t.Errorf("step 3: %+v", collector.Steps[3])
	}
}

func TestObservableChain2_FirstError(t *testing.T) {
	failing := agent.Func[string, string]("fail", func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("boom")
	})
	second := agent.Func[string, string]("ok", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	collector := &Collector{}
	chain := ObservableChain2(failing, second, collector)

	_, err := chain.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}

	// Should have 2 steps: started + error for first agent.
	if len(collector.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(collector.Steps))
	}
	if collector.Steps[1].Type != StepError {
		t.Errorf("expected error step, got %+v", collector.Steps[1])
	}
}

func TestObservableChain2_SecondError(t *testing.T) {
	first := agent.Func[string, string]("ok", func(_ context.Context, s string) (string, error) {
		return s, nil
	})
	failing := agent.Func[string, string]("fail", func(_ context.Context, _ string) (string, error) {
		return "", fmt.Errorf("boom")
	})

	collector := &Collector{}
	chain := ObservableChain2(first, failing, collector)

	_, err := chain.Run(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error")
	}

	// Should have 4 steps: started+completed for first, started+error for second.
	if len(collector.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(collector.Steps))
	}
	if collector.Steps[3].Type != StepError {
		t.Errorf("expected error step, got %+v", collector.Steps[3])
	}
}

func TestObservableChain2_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	first := agent.Func[string, string]("first", func(_ context.Context, s string) (string, error) {
		cancel() // cancel after first completes
		return s, nil
	})
	second := agent.Func[string, string]("second", func(_ context.Context, s string) (string, error) {
		return s, nil
	})

	collector := &Collector{}
	chain := ObservableChain2(first, second, collector)

	_, err := chain.Run(ctx, "x")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestObservableChain3_Success(t *testing.T) {
	a := agent.Func[int, string]("itoa", func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("%d", n), nil
	})
	b := agent.Func[string, string]("upper", func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	})
	c := agent.Func[string, int]("len", func(_ context.Context, s string) (int, error) {
		return len(s), nil
	})

	collector := &Collector{}
	chain := ObservableChain3(a, b, c, collector)

	result, err := chain.Run(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if result != 2 { // "42" → "42" → len=2
		t.Errorf("expected 2, got %d", result)
	}

	// Should have multiple steps recorded.
	if len(collector.Steps) < 4 {
		t.Errorf("expected at least 4 steps, got %d", len(collector.Steps))
	}
}
