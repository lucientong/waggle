// Package observe provides observability primitives for Waggle workflows.
//
// It defines structured events, execution tracing, metrics collection,
// and a structured logging wrapper. All components are optional and designed
// to be composable with each other.
package observe

import "time"

// EventType identifies the kind of event that occurred during a workflow run.
type EventType string

const (
	// EventAgentStart is emitted when an agent begins execution.
	EventAgentStart EventType = "agent.start"
	// EventAgentEnd is emitted when an agent completes successfully.
	EventAgentEnd EventType = "agent.end"
	// EventAgentError is emitted when an agent returns an error.
	EventAgentError EventType = "agent.error"
	// EventDataFlow is emitted when data is passed from one agent to another.
	EventDataFlow EventType = "data.flow"
	// EventWorkflowStart is emitted when a workflow run begins.
	EventWorkflowStart EventType = "workflow.start"
	// EventWorkflowEnd is emitted when a workflow run completes.
	EventWorkflowEnd EventType = "workflow.end"
)

// Event represents a single observable occurrence during a workflow execution.
// Events are emitted through an event channel and consumed by observers such as
// the tracer, metrics collector, or the web visualization panel.
type Event struct {
	// Type identifies the kind of event.
	Type EventType `json:"type"`
	// AgentName is the name of the agent that produced this event.
	AgentName string `json:"agent_name,omitempty"`
	// WorkflowID is a unique identifier for the workflow run.
	WorkflowID string `json:"workflow_id,omitempty"`
	// Timestamp is when the event was emitted.
	Timestamp time.Time `json:"timestamp"`
	// Duration is the elapsed time (for EventAgentEnd and EventAgentError).
	Duration time.Duration `json:"duration,omitempty"`
	// Error is the error message (for EventAgentError).
	Error string `json:"error,omitempty"`
	// InputSize is the byte size of the input data (for EventAgentStart and EventDataFlow).
	InputSize int `json:"input_size,omitempty"`
	// OutputSize is the byte size of the output data (for EventAgentEnd and EventDataFlow).
	OutputSize int `json:"output_size,omitempty"`
	// Metadata holds any additional key-value pairs for extensibility.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewAgentStartEvent creates an EventAgentStart event.
func NewAgentStartEvent(workflowID, agentName string, inputSize int) Event {
	return Event{
		Type:       EventAgentStart,
		AgentName:  agentName,
		WorkflowID: workflowID,
		Timestamp:  time.Now(),
		InputSize:  inputSize,
	}
}

// NewAgentEndEvent creates an EventAgentEnd event.
func NewAgentEndEvent(workflowID, agentName string, duration time.Duration, outputSize int) Event {
	return Event{
		Type:       EventAgentEnd,
		AgentName:  agentName,
		WorkflowID: workflowID,
		Timestamp:  time.Now(),
		Duration:   duration,
		OutputSize: outputSize,
	}
}

// NewAgentErrorEvent creates an EventAgentError event.
func NewAgentErrorEvent(workflowID, agentName string, duration time.Duration, err error) Event {
	e := Event{
		Type:       EventAgentError,
		AgentName:  agentName,
		WorkflowID: workflowID,
		Timestamp:  time.Now(),
		Duration:   duration,
	}
	if err != nil {
		e.Error = err.Error()
	}
	return e
}

// NewDataFlowEvent creates an EventDataFlow event representing data transfer
// between two agents.
func NewDataFlowEvent(workflowID, fromAgent, toAgent string, dataSize int) Event {
	return Event{
		Type:       EventDataFlow,
		WorkflowID: workflowID,
		Timestamp:  time.Now(),
		InputSize:  dataSize,
		Metadata: map[string]any{
			"from": fromAgent,
			"to":   toAgent,
		},
	}
}
