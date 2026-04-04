package observe_test

import (
	"errors"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/observe"
)

// TestNewAgentStartEvent verifies event fields are set correctly.
func TestNewAgentStartEvent(t *testing.T) {
	e := observe.NewAgentStartEvent("wf-1", "fetcher", 128)

	if e.Type != observe.EventAgentStart {
		t.Errorf("Type = %q, want %q", e.Type, observe.EventAgentStart)
	}
	if e.AgentName != "fetcher" {
		t.Errorf("AgentName = %q, want %q", e.AgentName, "fetcher")
	}
	if e.WorkflowID != "wf-1" {
		t.Errorf("WorkflowID = %q, want %q", e.WorkflowID, "wf-1")
	}
	if e.InputSize != 128 {
		t.Errorf("InputSize = %d, want 128", e.InputSize)
	}
	if e.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
}

func TestNewAgentEndEvent(t *testing.T) {
	e := observe.NewAgentEndEvent("wf-2", "summarizer", 50*time.Millisecond, 256)

	if e.Type != observe.EventAgentEnd {
		t.Errorf("Type = %q, want %q", e.Type, observe.EventAgentEnd)
	}
	if e.Duration != 50*time.Millisecond {
		t.Errorf("Duration = %v, want 50ms", e.Duration)
	}
	if e.OutputSize != 256 {
		t.Errorf("OutputSize = %d, want 256", e.OutputSize)
	}
}

func TestNewAgentErrorEvent(t *testing.T) {
	err := errors.New("something broke")
	e := observe.NewAgentErrorEvent("wf-3", "reviewer", 10*time.Millisecond, err)

	if e.Type != observe.EventAgentError {
		t.Errorf("Type = %q, want %q", e.Type, observe.EventAgentError)
	}
	if e.Error != "something broke" {
		t.Errorf("Error = %q, want %q", e.Error, "something broke")
	}
}

func TestNewDataFlowEvent(t *testing.T) {
	e := observe.NewDataFlowEvent("wf-4", "agent-a", "agent-b", 512)

	if e.Type != observe.EventDataFlow {
		t.Errorf("Type = %q, want %q", e.Type, observe.EventDataFlow)
	}
	if e.Metadata["from"] != "agent-a" {
		t.Errorf("Metadata[from] = %v, want %q", e.Metadata["from"], "agent-a")
	}
	if e.Metadata["to"] != "agent-b" {
		t.Errorf("Metadata[to] = %v, want %q", e.Metadata["to"], "agent-b")
	}
}

// ---- Metrics tests ----------------------------------------------------------

func TestMetrics_RecordAndQuery(t *testing.T) {
	m := observe.NewMetrics()

	m.RecordStart("agent-a", 100)
	m.RecordSuccess("agent-a", 50*time.Millisecond, 200)

	m.RecordStart("agent-a", 150)
	m.RecordError("agent-a", 10*time.Millisecond)

	ag := m.Agent("agent-a")
	if ag.TotalRuns != 2 {
		t.Errorf("TotalRuns = %d, want 2", ag.TotalRuns)
	}
	if ag.SuccessRuns != 1 {
		t.Errorf("SuccessRuns = %d, want 1", ag.SuccessRuns)
	}
	if ag.ErrorRuns != 1 {
		t.Errorf("ErrorRuns = %d, want 1", ag.ErrorRuns)
	}
	if ag.ErrorRate() != 0.5 {
		t.Errorf("ErrorRate() = %f, want 0.5", ag.ErrorRate())
	}
	if ag.TotalInputBytes != 250 {
		t.Errorf("TotalInputBytes = %d, want 250", ag.TotalInputBytes)
	}
}

func TestMetrics_UnknownAgent(t *testing.T) {
	m := observe.NewMetrics()
	ag := m.Agent("ghost")
	if ag.TotalRuns != 0 {
		t.Errorf("unknown agent TotalRuns = %d, want 0", ag.TotalRuns)
	}
	if ag.ErrorRate() != 0 {
		t.Errorf("unknown agent ErrorRate = %f, want 0", ag.ErrorRate())
	}
}

func TestMetrics_ConsumeEvents(t *testing.T) {
	m := observe.NewMetrics()
	ch := make(chan observe.Event, 10)

	ch <- observe.NewAgentStartEvent("wf", "agent-x", 50)
	ch <- observe.NewAgentEndEvent("wf", "agent-x", 20*time.Millisecond, 100)
	ch <- observe.NewAgentStartEvent("wf", "agent-x", 60)
	ch <- observe.NewAgentErrorEvent("wf", "agent-x", 5*time.Millisecond, errors.New("oops"))
	close(ch)

	m.ConsumeEvents(ch)

	ag := m.Agent("agent-x")
	if ag.TotalRuns != 2 {
		t.Errorf("TotalRuns = %d, want 2", ag.TotalRuns)
	}
	if ag.SuccessRuns != 1 {
		t.Errorf("SuccessRuns = %d, want 1", ag.SuccessRuns)
	}
	if ag.ErrorRuns != 1 {
		t.Errorf("ErrorRuns = %d, want 1", ag.ErrorRuns)
	}
}

func TestMetrics_All(t *testing.T) {
	m := observe.NewMetrics()
	m.RecordStart("a", 10)
	m.RecordSuccess("a", time.Millisecond, 20)
	m.RecordStart("b", 30)
	m.RecordError("b", time.Millisecond)

	all := m.All()
	if len(all) != 2 {
		t.Errorf("All() returned %d items, want 2", len(all))
	}
}

// ---- Tracer tests ----------------------------------------------------------

func TestTracer_SpanLifecycle(t *testing.T) {
	tracer := observe.NewTracer("trace-1", nil)

	span := tracer.StartSpan("fetch", "", map[string]any{"url": "https://example.com"})
	if span.SpanID() == "" {
		t.Error("SpanID() should not be empty")
	}

	span.SetAttribute("response_size", 1024)
	span.End()

	spans := tracer.Spans()
	if len(spans) != 1 {
		t.Fatalf("Spans() returned %d spans, want 1", len(spans))
	}

	s := spans[0]
	if s.TraceID != "trace-1" {
		t.Errorf("TraceID = %q, want %q", s.TraceID, "trace-1")
	}
	if s.Name != "fetch" {
		t.Errorf("Name = %q, want %q", s.Name, "fetch")
	}
	if s.Status != observe.SpanStatusOK {
		t.Errorf("Status = %d, want SpanStatusOK", s.Status)
	}
	if s.IsRunning() {
		t.Error("span should not be running after End()")
	}
	if s.Duration() <= 0 {
		t.Error("Duration should be > 0 after End()")
	}
}

func TestTracer_SpanError(t *testing.T) {
	tracer := observe.NewTracer("trace-2", nil)
	span := tracer.StartSpan("compute", "", nil)
	span.EndWithError(errors.New("compute failed"))

	spans := tracer.Spans()
	if spans[0].Status != observe.SpanStatusError {
		t.Errorf("Status = %d, want SpanStatusError", spans[0].Status)
	}
	if spans[0].ErrorMessage != "compute failed" {
		t.Errorf("ErrorMessage = %q, want %q", spans[0].ErrorMessage, "compute failed")
	}
}

func TestTracer_Exporter(t *testing.T) {
	var exported []observe.Span
	exporter := &testExporter{spans: &exported}
	tracer := observe.NewTracer("trace-3", exporter)

	span := tracer.StartSpan("my-op", "", nil)
	span.End()

	if len(exported) != 1 {
		t.Errorf("exporter received %d spans, want 1", len(exported))
	}
}

// testExporter collects exported spans for testing.
type testExporter struct {
	spans *[]observe.Span
}

func (e *testExporter) ExportSpan(span observe.Span) {
	*e.spans = append(*e.spans, span)
}
