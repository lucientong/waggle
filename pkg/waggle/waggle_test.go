package waggle_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/waggle"
)

// makeErasedUpper creates a type-erased string-to-string uppercase agent.
func makeErasedUpper(name string) agent.UntypedAgent {
	return agent.Erase(agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return strings.ToUpper(s), nil
	}))
}

// makeErasedExclaim creates a type-erased agent that appends "!" to a string.
func makeErasedExclaim(name string) agent.UntypedAgent {
	return agent.Erase(agent.Func[string, string](name, func(_ context.Context, s string) (string, error) {
		return s + "!", nil
	}))
}

// TestWaggle_Register verifies that agents can be registered.
func TestWaggle_Register(t *testing.T) {
	w := waggle.New()
	a := makeErasedUpper("upper")
	if err := w.Register(a); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}

	info := w.DAGInfo()
	if info.NodeCount != 1 {
		t.Errorf("NodeCount = %d, want 1", info.NodeCount)
	}
}

// TestWaggle_Register_Duplicate verifies duplicate names are rejected.
func TestWaggle_Register_Duplicate(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("upper")) //nolint
	if err := w.Register(makeErasedUpper("upper")); err == nil {
		t.Error("Register() expected error for duplicate name, got nil")
	}
}

// TestWaggle_Connect verifies that edges can be added.
func TestWaggle_Connect(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("upper"), makeErasedExclaim("exclaim")) //nolint
	if err := w.Connect("upper", "exclaim"); err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}

	info := w.DAGInfo()
	if info.EdgeCount != 1 {
		t.Errorf("EdgeCount = %d, want 1", info.EdgeCount)
	}
}

// TestWaggle_Connect_UnknownAgent verifies that connecting unregistered agents fails.
func TestWaggle_Connect_UnknownAgent(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("upper")) //nolint
	if err := w.Connect("upper", "ghost"); err == nil {
		t.Error("Connect() expected error for unknown agent, got nil")
	}
}

// TestWaggle_Connect_Cycle verifies that creating a cycle is rejected.
func TestWaggle_Connect_Cycle(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("a"), makeErasedUpper("b")) //nolint
	w.Connect("a", "b")                                    //nolint
	err := w.Connect("b", "a")
	if err == nil {
		t.Fatal("Connect() expected cycle error, got nil")
	}
	if !errors.Is(err, waggle.ErrCycleDetected) {
		t.Errorf("error = %v, want ErrCycleDetected", err)
	}
}

// TestWaggle_RunFrom_SingleAgent verifies running a single-node workflow.
func TestWaggle_RunFrom_SingleAgent(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("upper")) //nolint

	result, err := w.RunFrom(context.Background(), "upper", "hello")
	if err != nil {
		t.Fatalf("RunFrom() unexpected error: %v", err)
	}
	if result != "HELLO" {
		t.Errorf("RunFrom() = %v, want %q", result, "HELLO")
	}
}

// TestWaggle_RunFrom_LinearPipeline verifies a 2-stage linear workflow.
func TestWaggle_RunFrom_LinearPipeline(t *testing.T) {
	w := waggle.New()
	w.Register(makeErasedUpper("upper"), makeErasedExclaim("exclaim")) //nolint
	w.Connect("upper", "exclaim")                                      //nolint

	result, err := w.RunFrom(context.Background(), "upper", "hello")
	if err != nil {
		t.Fatalf("RunFrom() unexpected error: %v", err)
	}
	if result != "HELLO!" {
		t.Errorf("RunFrom() = %v, want %q", result, "HELLO!")
	}
}

// TestWaggle_RunFrom_AgentError verifies that an agent failure is propagated.
func TestWaggle_RunFrom_AgentError(t *testing.T) {
	sentinel := errors.New("agent exploded")
	failing := agent.Erase(agent.Func[string, string]("failing", func(_ context.Context, _ string) (string, error) {
		return "", sentinel
	}))

	w := waggle.New()
	w.Register(failing) //nolint

	_, err := w.RunFrom(context.Background(), "failing", "input")
	if err == nil {
		t.Fatal("RunFrom() expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("RunFrom() error = %v, want to wrap %v", err, sentinel)
	}
}

// TestWaggle_RunFrom_ContextCanceled verifies that context cancellation is respected.
func TestWaggle_RunFrom_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := waggle.New()
	w.Register(makeErasedUpper("upper")) //nolint

	_, err := w.RunFrom(ctx, "upper", "hello")
	if err == nil {
		t.Fatal("RunFrom() expected error for cancelled context, got nil")
	}
}
