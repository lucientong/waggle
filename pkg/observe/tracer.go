package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// SpanStatus represents the completion status of a span.
type SpanStatus int

const (
	// SpanStatusOK indicates the span completed successfully.
	SpanStatusOK SpanStatus = iota
	// SpanStatusError indicates the span completed with an error.
	SpanStatusError
	// SpanStatusRunning indicates the span is still in progress.
	SpanStatusRunning
)

// Span represents a single traced operation. It is analogous to an
// OpenTelemetry span but avoids the external dependency. Spans can be
// exported to an OTel-compatible backend via the Exporter interface.
type Span struct {
	// TraceID is a unique identifier for the entire trace (all spans in one workflow run).
	TraceID string
	// SpanID is a unique identifier for this specific span.
	SpanID string
	// ParentSpanID is the SpanID of the parent span (empty for root spans).
	ParentSpanID string
	// Name is the human-readable span name (typically the agent name).
	Name string
	// StartTime is when the span began.
	StartTime time.Time
	// EndTime is when the span ended (zero if still running).
	EndTime time.Time
	// Status is the completion status.
	Status SpanStatus
	// ErrorMessage holds the error string if Status is SpanStatusError.
	ErrorMessage string
	// Attributes holds arbitrary key-value pairs associated with the span.
	Attributes map[string]any
}

// Duration returns the span's duration. Returns 0 if the span is still running.
func (s *Span) Duration() time.Duration {
	if s.EndTime.IsZero() {
		return 0
	}
	return s.EndTime.Sub(s.StartTime)
}

// IsRunning returns true if the span has not yet ended.
func (s *Span) IsRunning() bool {
	return s.EndTime.IsZero()
}

// SpanExporter receives completed spans for export to a backend (e.g., Jaeger, Zipkin, OTLP).
type SpanExporter interface {
	// ExportSpan is called when a span ends. Implementations should be non-blocking.
	ExportSpan(span Span)
}

// Tracer records execution spans for a single workflow run.
// It is safe for concurrent use.
type Tracer struct {
	traceID  string
	mu       sync.Mutex
	spans    []*Span
	exporter SpanExporter
	sampler  Sampler
	sampled  bool // cached sampling decision for this trace
}

// TracerOption configures a Tracer.
type TracerOption func(*Tracer)

// WithSampler sets a sampling strategy for the tracer.
// If the sampler decides not to sample this trace, no spans are recorded or exported.
// Default: AlwaysSample.
func WithSampler(s Sampler) TracerOption {
	return func(t *Tracer) {
		t.sampler = s
	}
}

// NewTracer creates a new Tracer for the given trace ID.
// If exporter is non-nil, completed spans are forwarded to it.
func NewTracer(traceID string, exporter SpanExporter, opts ...TracerOption) *Tracer {
	t := &Tracer{
		traceID:  traceID,
		exporter: exporter,
		sampler:  AlwaysSample{},
	}
	for _, opt := range opts {
		opt(t)
	}
	t.sampled = t.sampler.ShouldSample(traceID)
	return t
}

// IsSampled returns whether this trace is being recorded.
func (t *Tracer) IsSampled() bool {
	return t.sampled
}

// tracerKey is the context key for storing the active Tracer.
type tracerKey struct{}

// WithTracer returns a new context with the given tracer attached.
func WithTracer(ctx context.Context, t *Tracer) context.Context {
	return context.WithValue(ctx, tracerKey{}, t)
}

// TracerFromContext retrieves the Tracer from the context.
// Returns nil if no tracer is present.
func TracerFromContext(ctx context.Context) *Tracer {
	t, _ := ctx.Value(tracerKey{}).(*Tracer)
	return t
}

// StartSpan begins a new span for the named operation.
// If parentSpanID is empty, the span is a root span for this trace.
// Returns a spanHandle that must be ended via End or EndWithError.
// If the trace is not sampled, returns a no-op handle that does nothing.
func (t *Tracer) StartSpan(name, parentSpanID string, attrs map[string]any) *spanHandle {
	if !t.sampled {
		return &spanHandle{span: &Span{SpanID: "unsampled"}, tracer: t, noop: true}
	}

	span := &Span{
		TraceID:      t.traceID,
		SpanID:       generateID(),
		ParentSpanID: parentSpanID,
		Name:         name,
		StartTime:    time.Now(),
		Status:       SpanStatusRunning,
		Attributes:   attrs,
	}

	t.mu.Lock()
	t.spans = append(t.spans, span)
	t.mu.Unlock()

	return &spanHandle{span: span, tracer: t}
}

// Spans returns a snapshot of all spans recorded by this tracer.
func (t *Tracer) Spans() []Span {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := make([]Span, len(t.spans))
	for i, s := range t.spans {
		result[i] = *s
	}
	return result
}

// spanHandle provides the end-span API.
type spanHandle struct {
	span   *Span
	tracer *Tracer
	noop   bool
}

// SpanID returns the ID of this span.
func (h *spanHandle) SpanID() string {
	return h.span.SpanID
}

// SetAttribute sets a key-value attribute on the span.
func (h *spanHandle) SetAttribute(key string, value any) {
	if h.span.Attributes == nil {
		h.span.Attributes = make(map[string]any)
	}
	h.span.Attributes[key] = value
}

// End marks the span as successfully completed.
func (h *spanHandle) End() {
	if h.noop {
		return
	}
	h.span.EndTime = time.Now()
	h.span.Status = SpanStatusOK
	if h.tracer.exporter != nil {
		h.tracer.exporter.ExportSpan(*h.span)
	}
}

// EndWithError marks the span as failed with the given error.
func (h *spanHandle) EndWithError(err error) {
	if h.noop {
		return
	}
	h.span.EndTime = time.Now()
	h.span.Status = SpanStatusError
	if err != nil {
		h.span.ErrorMessage = err.Error()
	}
	if h.tracer.exporter != nil {
		h.tracer.exporter.ExportSpan(*h.span)
	}
}

// generateID generates a cryptographically random unique identifier for spans.
// Returns a 32-character hex string (128-bit), compatible with OpenTelemetry trace/span ID format.
func generateID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// Extremely unlikely; fall back to time-based ID.
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b)
}
