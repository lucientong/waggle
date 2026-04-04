package waggle

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// helper to make a simple string->string untyped agent.
func erasedFn(name string, fn func(string) string) agent.UntypedAgent {
	return agent.Erase(agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return fn(s), nil
	}))
}

// TestExecutor_SingleNode verifies a single-node DAG runs correctly.
func TestExecutor_SingleNode(t *testing.T) {
	d := newDAG()
	d.addNode("upper", "upper", 1) //nolint

	agents := map[string]agent.UntypedAgent{
		"upper": erasedFn("upper", strings.ToUpper),
	}

	ex := newExecutor(d, agents)
	result, err := ex.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "HELLO" {
		t.Errorf("Run() = %v, want %q", result, "HELLO")
	}
}

// TestExecutor_LinearPipeline verifies execution order in a linear chain.
func TestExecutor_LinearPipeline(t *testing.T) {
	d := newDAG()
	d.addNode("a", "a", 1) //nolint
	d.addNode("b", "b", 1) //nolint
	d.addNode("c", "c", 1) //nolint
	d.addEdge("a", "b")    //nolint
	d.addEdge("b", "c")    //nolint

	agents := map[string]agent.UntypedAgent{
		"a": erasedFn("a", func(s string) string { return "[a:" + s + "]" }),
		"b": erasedFn("b", func(s string) string { return "[b:" + s + "]" }),
		"c": erasedFn("c", func(s string) string { return "[c:" + s + "]" }),
	}

	ex := newExecutor(d, agents)
	result, err := ex.Run(context.Background(), "x")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	want := "[c:[b:[a:x]]]"
	if result != want {
		t.Errorf("Run() = %v, want %q", result, want)
	}
}

// TestExecutor_AgentError verifies that a single agent failure cancels execution.
func TestExecutor_AgentError(t *testing.T) {
	sentinel := errors.New("boom")
	var bCalled atomic.Bool

	d := newDAG()
	d.addNode("a", "a", 1) //nolint
	d.addNode("b", "b", 1) //nolint
	d.addEdge("a", "b")    //nolint

	agents := map[string]agent.UntypedAgent{
		"a": agent.Erase(agent.Func[string, string]("a", func(_ context.Context, _ string) (string, error) {
			return "", sentinel
		})),
		"b": agent.Erase(agent.Func[string, string]("b", func(_ context.Context, s string) (string, error) {
			bCalled.Store(true)
			return s, nil
		})),
	}

	ex := newExecutor(d, agents)
	_, err := ex.Run(context.Background(), "input")

	if err == nil {
		t.Fatal("Run() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want to wrap sentinel", err)
	}
	if bCalled.Load() {
		t.Error("agent 'b' should not have been called after 'a' failed")
	}
}

// TestExecutor_ContextCanceled verifies that context cancellation stops execution.
func TestExecutor_ContextCanceled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping executor context cancel test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := newDAG()
	d.addNode("slow", "slow", 1) //nolint

	agents := map[string]agent.UntypedAgent{
		"slow": agent.Erase(agent.Func[string, string]("slow", func(ctx context.Context, _ string) (string, error) {
			select {
			case <-time.After(5 * time.Second):
				return "late", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})),
	}

	// Cancel after a short time.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	ex := newExecutor(d, agents)
	_, err := ex.Run(ctx, "input")
	if err == nil {
		t.Fatal("Run() expected error from context cancellation, got nil")
	}
}

// TestExecutor_EmptyDAG verifies that an empty DAG returns the input unchanged.
func TestExecutor_EmptyDAG(t *testing.T) {
	d := newDAG()
	ex := newExecutor(d, map[string]agent.UntypedAgent{})
	result, err := ex.Run(context.Background(), "passthrough")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result != "passthrough" {
		t.Errorf("Run() = %v, want %q", result, "passthrough")
	}
}
