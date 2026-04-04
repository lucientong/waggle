package observe

import (
	"sync"
	"time"
)

// AgentMetrics holds aggregated performance metrics for a single agent.
type AgentMetrics struct {
	// AgentName identifies the agent.
	AgentName string
	// TotalRuns is the total number of times the agent was invoked.
	TotalRuns int64
	// SuccessRuns is the count of invocations that completed without error.
	SuccessRuns int64
	// ErrorRuns is the count of invocations that returned an error.
	ErrorRuns int64
	// TotalDuration is the cumulative execution time across all runs.
	TotalDuration time.Duration
	// MinDuration is the fastest observed execution time.
	MinDuration time.Duration
	// MaxDuration is the slowest observed execution time.
	MaxDuration time.Duration
	// TotalInputBytes is the total byte size of all inputs processed.
	TotalInputBytes int64
	// TotalOutputBytes is the total byte size of all outputs produced.
	TotalOutputBytes int64
}

// AvgDuration returns the average execution duration across all runs.
// Returns 0 if no runs have been recorded.
func (m *AgentMetrics) AvgDuration() time.Duration {
	if m.TotalRuns == 0 {
		return 0
	}
	return m.TotalDuration / time.Duration(m.TotalRuns)
}

// ErrorRate returns the fraction of runs that resulted in errors (0.0 to 1.0).
func (m *AgentMetrics) ErrorRate() float64 {
	if m.TotalRuns == 0 {
		return 0
	}
	return float64(m.ErrorRuns) / float64(m.TotalRuns)
}

// Metrics collects and aggregates per-agent performance metrics.
// It can be wired to an event channel to automatically receive events.
// It is safe for concurrent use.
type Metrics struct {
	mu   sync.RWMutex
	data map[string]*AgentMetrics
}

// NewMetrics creates an empty metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		data: make(map[string]*AgentMetrics),
	}
}

// RecordStart records the start of an agent execution with the given input size.
func (m *Metrics) RecordStart(agentName string, inputSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag := m.getOrCreate(agentName)
	ag.TotalRuns++
	ag.TotalInputBytes += int64(inputSize)
}

// RecordSuccess records a successful agent execution.
func (m *Metrics) RecordSuccess(agentName string, duration time.Duration, outputSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag := m.getOrCreate(agentName)
	ag.SuccessRuns++
	ag.TotalDuration += duration
	ag.TotalOutputBytes += int64(outputSize)

	if ag.MinDuration == 0 || duration < ag.MinDuration {
		ag.MinDuration = duration
	}
	if duration > ag.MaxDuration {
		ag.MaxDuration = duration
	}
}

// RecordError records a failed agent execution.
func (m *Metrics) RecordError(agentName string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ag := m.getOrCreate(agentName)
	ag.ErrorRuns++
	ag.TotalDuration += duration
}

// ConsumeEvents processes events from an event channel and updates metrics.
// This method blocks until the channel is closed. Run it in a separate goroutine.
func (m *Metrics) ConsumeEvents(events <-chan Event) {
	startTimes := make(map[string]time.Time)
	inputSizes := make(map[string]int)

	for event := range events {
		switch event.Type {
		case EventAgentStart:
			startTimes[event.AgentName] = event.Timestamp
			inputSizes[event.AgentName] = event.InputSize
			m.RecordStart(event.AgentName, event.InputSize)

		case EventAgentEnd:
			duration := event.Duration
			if duration == 0 {
				if start, ok := startTimes[event.AgentName]; ok {
					duration = time.Since(start)
				}
			}
			m.RecordSuccess(event.AgentName, duration, event.OutputSize)
			delete(startTimes, event.AgentName)
			delete(inputSizes, event.AgentName)

		case EventAgentError:
			duration := event.Duration
			if duration == 0 {
				if start, ok := startTimes[event.AgentName]; ok {
					duration = time.Since(start)
				}
			}
			m.RecordError(event.AgentName, duration)
			delete(startTimes, event.AgentName)
			delete(inputSizes, event.AgentName)
		}
	}
}

// Agent returns a snapshot of the metrics for the named agent.
// Returns a zero-value AgentMetrics if the agent has no recorded data.
func (m *Metrics) Agent(name string) AgentMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ag, ok := m.data[name]; ok {
		return *ag
	}
	return AgentMetrics{AgentName: name}
}

// All returns snapshots of all recorded agent metrics.
func (m *Metrics) All() []AgentMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentMetrics, 0, len(m.data))
	for _, ag := range m.data {
		result = append(result, *ag)
	}
	return result
}

// getOrCreate retrieves the metrics entry for an agent, creating it if absent.
// Caller must hold m.mu.Lock().
func (m *Metrics) getOrCreate(name string) *AgentMetrics {
	if ag, ok := m.data[name]; ok {
		return ag
	}
	ag := &AgentMetrics{AgentName: name}
	m.data[name] = ag
	return ag
}
