package stream

import (
	"testing"
	"time"
)

func TestObserverFunc(t *testing.T) {
	var received Step
	fn := ObserverFunc(func(s Step) {
		received = s
	})

	step := Step{AgentName: "test", Type: StepStarted, Timestamp: time.Now()}
	fn.OnStep(step)

	if received.AgentName != "test" || received.Type != StepStarted {
		t.Errorf("unexpected step: %+v", received)
	}
}

func TestMultiObserver(t *testing.T) {
	c1 := &Collector{}
	c2 := &Collector{}
	multi := NewMultiObserver(c1, c2)

	step := Step{AgentName: "a", Type: StepCompleted}
	multi.OnStep(step)

	if len(c1.Steps) != 1 || len(c2.Steps) != 1 {
		t.Errorf("expected both collectors to receive step, got %d and %d", len(c1.Steps), len(c2.Steps))
	}
}

func TestCollector(t *testing.T) {
	c := &Collector{}
	c.OnStep(Step{AgentName: "a", Type: StepStarted})
	c.OnStep(Step{AgentName: "a", Type: StepCompleted})

	if len(c.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(c.Steps))
	}
	if c.Steps[0].Type != StepStarted || c.Steps[1].Type != StepCompleted {
		t.Error("unexpected step types")
	}
}
