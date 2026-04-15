// Package stream provides intermediate step observation for agent pipelines.
//
// When agents execute in a Chain or DAG, the Observer interface allows callers
// to receive real-time notifications about each agent's progress — starts,
// completions, errors, and streaming token chunks.
//
// This is particularly useful for:
//   - Displaying progress in web UIs (via SSE)
//   - Logging intermediate reasoning steps
//   - Building streaming chat interfaces
package stream

import "time"

// StepType identifies the kind of execution step.
type StepType string

const (
	// StepStarted indicates an agent has begun execution.
	StepStarted StepType = "started"
	// StepChunk carries a streaming token from an LLM.
	StepChunk StepType = "chunk"
	// StepCompleted indicates an agent finished successfully.
	StepCompleted StepType = "completed"
	// StepError indicates an agent encountered an error.
	StepError StepType = "error"
)

// Step represents a single observation point in a pipeline execution.
type Step struct {
	// AgentName is the name of the agent producing this step.
	AgentName string `json:"agent_name"`
	// Type identifies the step kind.
	Type StepType `json:"type"`
	// Content holds the token chunk (for StepChunk), final result (for StepCompleted),
	// or error message (for StepError). Empty for StepStarted.
	Content string `json:"content,omitempty"`
	// Index is the position of this agent in a Chain (0-based).
	Index int `json:"index"`
	// Timestamp is when this step was recorded.
	Timestamp time.Time `json:"timestamp"`
}

// Observer receives step notifications from observable pipelines.
type Observer interface {
	OnStep(step Step)
}

// ObserverFunc is a function adapter for Observer.
type ObserverFunc func(Step)

// OnStep calls the underlying function.
func (f ObserverFunc) OnStep(s Step) { f(s) }

// MultiObserver fans out steps to multiple observers.
type MultiObserver struct {
	observers []Observer
}

// NewMultiObserver creates an observer that forwards to all given observers.
func NewMultiObserver(observers ...Observer) *MultiObserver {
	return &MultiObserver{observers: observers}
}

// OnStep forwards the step to all wrapped observers.
func (m *MultiObserver) OnStep(s Step) {
	for _, obs := range m.observers {
		obs.OnStep(s)
	}
}

// Collector is an Observer that accumulates all steps in a slice.
// Useful for testing.
type Collector struct {
	Steps []Step
}

// OnStep appends the step to the collector.
func (c *Collector) OnStep(s Step) {
	c.Steps = append(c.Steps, s)
}
