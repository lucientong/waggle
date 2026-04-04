package observe_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/observe"
)

// newBufferedLogger creates a Logger backed by a buffer for capturing output.
func newBufferedLogger(buf *bytes.Buffer) *observe.Logger {
	handler := slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return observe.NewLogger(slog.New(handler))
}

func TestNewLogger_Default(t *testing.T) {
	// NewLogger(nil) should not panic and should return a usable logger.
	l := observe.NewLogger(nil)
	if l == nil {
		t.Fatal("NewLogger(nil) returned nil")
	}
	// Smoke test: calling a method should not panic.
	l.AgentStart("test-agent", "wf-1", 100)
}

func TestNewLogger_Custom(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.AgentStart("my-agent", "wf-42", 256)

	output := buf.String()
	if !strings.Contains(output, "my-agent") {
		t.Errorf("log output missing agent name: %s", output)
	}
	if !strings.Contains(output, "wf-42") {
		t.Errorf("log output missing workflow_id: %s", output)
	}
}

func TestLogger_AgentEnd(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.AgentEnd("summarizer", "wf-1", 50*time.Millisecond, 512)

	output := buf.String()
	if !strings.Contains(output, "agent end") {
		t.Errorf("expected 'agent end' in output: %s", output)
	}
	if !strings.Contains(output, "summarizer") {
		t.Errorf("expected agent name in output: %s", output)
	}
}

func TestLogger_AgentError(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.AgentError("failing-agent", "wf-err", 10*time.Millisecond, errors.New("connection refused"))

	output := buf.String()
	if !strings.Contains(output, "agent error") {
		t.Errorf("expected 'agent error' in output: %s", output)
	}
	if !strings.Contains(output, "connection refused") {
		t.Errorf("expected error message in output: %s", output)
	}
}

func TestLogger_AgentRetry(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.AgentRetry("retry-agent", 2, 5, 200*time.Millisecond, errors.New("timeout"))

	output := buf.String()
	if !strings.Contains(output, "agent retry") {
		t.Errorf("expected 'agent retry' in output: %s", output)
	}
	if !strings.Contains(output, "retry-agent") {
		t.Errorf("expected agent name in output: %s", output)
	}
}

func TestLogger_WorkflowStart(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.WorkflowStart("wf-start", 5)

	output := buf.String()
	if !strings.Contains(output, "workflow start") {
		t.Errorf("expected 'workflow start' in output: %s", output)
	}
}

func TestLogger_WorkflowEnd_Success(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.WorkflowEnd("wf-done", 100*time.Millisecond, nil)

	output := buf.String()
	if !strings.Contains(output, "workflow end") {
		t.Errorf("expected 'workflow end' in output: %s", output)
	}
	// Should be Info level, not Error.
	if strings.Contains(output, "ERROR") {
		t.Errorf("success workflow end should not log at ERROR level: %s", output)
	}
}

func TestLogger_WorkflowEnd_Error(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	l.WorkflowEnd("wf-fail", 200*time.Millisecond, errors.New("dag cycle detected"))

	output := buf.String()
	if !strings.Contains(output, "workflow end") {
		t.Errorf("expected 'workflow end' in output: %s", output)
	}
	if !strings.Contains(output, "dag cycle detected") {
		t.Errorf("expected error message in output: %s", output)
	}
}

func TestLogger_ContextInjection(t *testing.T) {
	l := observe.NewLogger(nil)
	ctx := observe.WithLogger(context.Background(), l)

	retrieved := observe.LoggerFromContext(ctx)
	if retrieved == nil {
		t.Fatal("LoggerFromContext returned nil")
	}
	// Smoke test: use the retrieved logger.
	retrieved.AgentStart("ctx-agent", "wf-ctx", 0)
}

func TestLoggerFromContext_Default(t *testing.T) {
	// When no logger is in context, a default one should be returned.
	l := observe.LoggerFromContext(context.Background())
	if l == nil {
		t.Fatal("LoggerFromContext returned nil for empty context")
	}
	// Should not panic.
	l.AgentStart("fallback-agent", "wf-default", 0)
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	enriched := l.With("component", "executor", "version", "1.0")
	enriched.WorkflowStart("wf-enriched", 3)

	output := buf.String()
	if !strings.Contains(output, "executor") {
		t.Errorf("expected enriched key in output: %s", output)
	}
}

func TestLogger_ConsumeEvents(t *testing.T) {
	var buf bytes.Buffer
	l := newBufferedLogger(&buf)

	ch := make(chan observe.Event, 5)
	ch <- observe.NewAgentStartEvent("wf-1", "agent-a", 100)
	ch <- observe.NewAgentEndEvent("wf-1", "agent-a", 20*time.Millisecond, 200)
	ch <- observe.NewAgentErrorEvent("wf-1", "agent-b", 5*time.Millisecond, errors.New("oops"))
	close(ch)

	l.ConsumeEvents(ch)

	output := buf.String()
	if !strings.Contains(output, "agent start") {
		t.Errorf("expected 'agent start' log entry: %s", output)
	}
	if !strings.Contains(output, "agent end") {
		t.Errorf("expected 'agent end' log entry: %s", output)
	}
	if !strings.Contains(output, "agent error") {
		t.Errorf("expected 'agent error' log entry: %s", output)
	}
}

func TestLogger_ConsumeEvents_EmptyChannel(t *testing.T) {
	l := observe.NewLogger(nil)
	ch := make(chan observe.Event)
	close(ch)

	// Should return immediately without error.
	l.ConsumeEvents(ch)
}
