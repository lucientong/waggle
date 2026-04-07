package observe_test

import (
	"context"
	"testing"
	"time"

	"github.com/lucientong/waggle/pkg/observe"
)

func TestWithTracer_And_TracerFromContext(t *testing.T) {
	tracer := observe.NewTracer("trace-ctx", nil)
	ctx := observe.WithTracer(context.Background(), tracer)

	retrieved := observe.TracerFromContext(ctx)
	if retrieved == nil {
		t.Fatal("TracerFromContext returned nil")
	}
	if retrieved != tracer {
		t.Error("TracerFromContext returned different tracer")
	}
}

func TestTracerFromContext_NoTracer(t *testing.T) {
	retrieved := observe.TracerFromContext(context.Background())
	if retrieved != nil {
		t.Errorf("TracerFromContext expected nil for empty context, got %v", retrieved)
	}
}

func TestAgentMetrics_AvgDuration(t *testing.T) {
	m := observe.NewMetrics()

	// No runs: AvgDuration should be 0.
	ag := m.Agent("no-runs")
	if ag.AvgDuration() != 0 {
		t.Errorf("AvgDuration() = %v, want 0 for no runs", ag.AvgDuration())
	}

	// With runs: AvgDuration should be TotalDuration / TotalRuns.
	m.RecordStart("avg-agent", 100)
	m.RecordSuccess("avg-agent", 50*time.Millisecond, 200)
	m.RecordStart("avg-agent", 100)
	m.RecordSuccess("avg-agent", 150*time.Millisecond, 200)

	ag = m.Agent("avg-agent")
	expected := (50*time.Millisecond + 150*time.Millisecond) / 2
	if ag.AvgDuration() != expected {
		t.Errorf("AvgDuration() = %v, want %v", ag.AvgDuration(), expected)
	}
}

func TestSpan_Duration_Running(t *testing.T) {
	tracer := observe.NewTracer("trace-dur", nil)
	handle := tracer.StartSpan("running-span", "", nil)

	spans := tracer.Spans()
	if spans[0].Duration() != 0 {
		t.Errorf("Duration() = %v, want 0 for running span", spans[0].Duration())
	}
	if !spans[0].IsRunning() {
		t.Error("IsRunning() should be true for active span")
	}

	handle.End()

	spans = tracer.Spans()
	if spans[0].IsRunning() {
		t.Error("IsRunning() should be false after End()")
	}
}

func TestSpanHandle_EndWithError_NilError(t *testing.T) {
	tracer := observe.NewTracer("trace-nil-err", nil)
	handle := tracer.StartSpan("nil-err-span", "", nil)
	handle.EndWithError(nil) //nolint

	spans := tracer.Spans()
	if spans[0].Status != observe.SpanStatusError {
		t.Errorf("Status = %d, want SpanStatusError", spans[0].Status)
	}
	if spans[0].ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty for nil error", spans[0].ErrorMessage)
	}
}

func TestSpanHandle_SetAttribute_NilMap(t *testing.T) {
	tracer := observe.NewTracer("trace-attr", nil)
	// Start span with nil attrs.
	handle := tracer.StartSpan("attr-span", "", nil)
	handle.SetAttribute("key", "value")
	handle.End()

	spans := tracer.Spans()
	if spans[0].Attributes["key"] != "value" {
		t.Errorf("Attributes[key] = %v, want %q", spans[0].Attributes["key"], "value")
	}
}

func TestTracer_ExporterWithError(t *testing.T) {
	var exported []observe.Span
	exporter := &collectingExporter{spans: &exported}
	tracer := observe.NewTracer("trace-exp-err", exporter)

	handle := tracer.StartSpan("failing-op", "", nil)
	handle.EndWithError(context.DeadlineExceeded)

	if len(exported) != 1 {
		t.Fatalf("exporter received %d spans, want 1", len(exported))
	}
	if exported[0].Status != observe.SpanStatusError {
		t.Errorf("exported span Status = %d, want SpanStatusError", exported[0].Status)
	}
}

type collectingExporter struct {
	spans *[]observe.Span
}

func (e *collectingExporter) ExportSpan(span observe.Span) {
	*e.spans = append(*e.spans, span)
}
