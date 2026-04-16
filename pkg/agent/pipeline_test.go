package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// --- PipelineContext Tests ---

func TestPipelineContext_SetGet(t *testing.T) {
	p := NewPipelineContext()
	p.Set("key", "value")

	v, ok := p.Get("key")
	if !ok || v != "value" {
		t.Errorf("expected (value, true), got (%v, %v)", v, ok)
	}

	_, ok = p.Get("missing")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestPipelineGet_Typed(t *testing.T) {
	p := NewPipelineContext()
	p.Set("count", 42)

	val, ok := PipelineGet[int](p, "count")
	if !ok || val != 42 {
		t.Errorf("expected (42, true), got (%v, %v)", val, ok)
	}

	// Wrong type.
	sval, ok := PipelineGet[string](p, "count")
	if ok {
		t.Errorf("expected false for wrong type, got %v", sval)
	}

	// Missing key.
	_, ok = PipelineGet[int](p, "nope")
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestPipelineContext_InContext(t *testing.T) {
	pctx := NewPipelineContext()
	pctx.Set("data", "hello")

	ctx := WithPipelineCtx(context.Background(), pctx)

	retrieved := PipelineCtxFrom(ctx)
	if retrieved == nil {
		t.Fatal("PipelineCtxFrom returned nil")
	}
	v, ok := retrieved.Get("data")
	if !ok || v != "hello" {
		t.Errorf("unexpected: %v, %v", v, ok)
	}
}

func TestPipelineCtxFrom_NilWhenMissing(t *testing.T) {
	p := PipelineCtxFrom(context.Background())
	if p != nil {
		t.Error("expected nil when no PipelineContext in context")
	}
}

// --- Pipeline (ChainN) Tests ---

func TestPipeline_BasicChain(t *testing.T) {
	double := Erase(Func[int, int]("double", func(_ context.Context, n int) (int, error) {
		return n * 2, nil
	}))
	addOne := Erase(Func[int, int]("addOne", func(_ context.Context, n int) (int, error) {
		return n + 1, nil
	}))
	toString := Erase(Func[int, string]("toString", func(_ context.Context, n int) (string, error) {
		return fmt.Sprintf("%d", n), nil
	}))

	p := NewPipeline("math").Add(double).Add(addOne).Add(toString)

	result, err := p.Run(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if result != "11" { // 5*2=10, 10+1=11, "11"
		t.Errorf("expected '11', got %v", result)
	}
}

func TestPipeline_SixStages(t *testing.T) {
	// Proves we can exceed Chain5's limit.
	stages := make([]UntypedAgent, 6)
	for i := 0; i < 6; i++ {
		i := i
		stages[i] = Erase(Func[int, int](fmt.Sprintf("stage%d", i), func(_ context.Context, n int) (int, error) {
			return n + 1, nil
		}))
	}

	p := NewPipeline("six")
	for _, s := range stages {
		p.Add(s)
	}

	result, err := p.Run(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if result != 6 {
		t.Errorf("expected 6 after 6 +1 stages, got %v", result)
	}
}

func TestPipeline_Error(t *testing.T) {
	ok := Erase(Func[int, int]("ok", func(_ context.Context, n int) (int, error) {
		return n, nil
	}))
	fail := Erase(Func[int, int]("fail", func(_ context.Context, _ int) (int, error) {
		return 0, errors.New("boom")
	}))
	after := Erase(Func[int, int]("after", func(_ context.Context, n int) (int, error) {
		return n, nil
	}))

	p := NewPipeline("err-test").Add(ok).Add(fail).Add(after)

	_, err := p.Run(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsSubstring(err.Error(), "stage 1") || !containsSubstring(err.Error(), "fail") {
		t.Errorf("error should mention stage and agent name: %v", err)
	}
}

func TestPipeline_TypeMismatch(t *testing.T) {
	intAgent := Erase(Func[int, int]("int", func(_ context.Context, n int) (int, error) {
		return n, nil
	}))
	stringAgent := Erase(Func[string, string]("string", func(_ context.Context, s string) (string, error) {
		return s, nil
	}))

	p := NewPipeline("mismatch").Add(intAgent).Add(stringAgent)

	_, err := p.Run(context.Background(), 42)
	if err == nil {
		t.Fatal("expected type mismatch error")
	}
}

func TestPipeline_Empty(t *testing.T) {
	p := NewPipeline("empty")
	result, err := p.Run(context.Background(), "passthrough")
	if err != nil {
		t.Fatal(err)
	}
	if result != "passthrough" {
		t.Errorf("expected passthrough, got %v", result)
	}
}

func TestPipeline_WithPipelineContext(t *testing.T) {
	// Stage 1: store a value in PipelineContext.
	storeAgent := Erase(Func[string, string]("store", func(ctx context.Context, s string) (string, error) {
		if pctx := PipelineCtxFrom(ctx); pctx != nil {
			pctx.Set("original", s)
		}
		return s + "-processed", nil
	}))

	// Stage 2: read from PipelineContext.
	readAgent := Erase(Func[string, string]("read", func(ctx context.Context, s string) (string, error) {
		if pctx := PipelineCtxFrom(ctx); pctx != nil {
			orig, _ := PipelineGet[string](pctx, "original")
			return fmt.Sprintf("%s (original: %s)", s, orig), nil
		}
		return s, nil
	}))

	p := NewPipeline("ctx-test").Add(storeAgent).Add(readAgent)

	pctx := NewPipelineContext()
	ctx := WithPipelineCtx(context.Background(), pctx)

	result, err := p.Run(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	expected := "hello-processed (original: hello)"
	if result != expected {
		t.Errorf("expected %q, got %v", expected, result)
	}
}

func TestPipeline_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stage := Erase(Func[int, int]("stage", func(_ context.Context, n int) (int, error) {
		return n, nil
	}))

	p := NewPipeline("cancel").Add(stage)
	_, err := p.Run(ctx, 1)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestPipeline_Name(t *testing.T) {
	p := NewPipeline("my-pipeline")
	if p.Name() != "my-pipeline" {
		t.Errorf("expected my-pipeline, got %s", p.Name())
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
