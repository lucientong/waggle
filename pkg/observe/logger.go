package observe

import (
	"context"
	"log/slog"
	"time"
)

// Logger is a structured logger wrapper for Waggle that adds workflow context
// (workflow ID, agent name) to all log entries. It delegates to a slog.Logger.
type Logger struct {
	inner *slog.Logger
}

// NewLogger creates a Logger wrapping the given slog.Logger.
// If logger is nil, the default slog logger is used.
func NewLogger(logger *slog.Logger) *Logger {
	if logger == nil {
		logger = slog.Default()
	}
	return &Logger{inner: logger}
}

// loggerContextKey is the key for storing the Logger in a context.
type loggerContextKey struct{}

// WithLogger returns a new context with the given logger attached.
func WithLogger(ctx context.Context, l *Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, l)
}

// LoggerFromContext retrieves the Logger from the context.
// Returns a default logger if none is present.
func LoggerFromContext(ctx context.Context) *Logger {
	if l, ok := ctx.Value(loggerContextKey{}).(*Logger); ok {
		return l
	}
	return NewLogger(nil)
}

// AgentStart logs the beginning of an agent execution at Debug level.
func (l *Logger) AgentStart(agentName, workflowID string, inputSize int) {
	l.inner.Debug("agent start",
		"agent", agentName,
		"workflow_id", workflowID,
		"input_size_bytes", inputSize,
	)
}

// AgentEnd logs the successful completion of an agent execution at Debug level.
func (l *Logger) AgentEnd(agentName, workflowID string, duration time.Duration, outputSize int) {
	l.inner.Debug("agent end",
		"agent", agentName,
		"workflow_id", workflowID,
		"duration_ms", duration.Milliseconds(),
		"output_size_bytes", outputSize,
	)
}

// AgentError logs a failed agent execution at Error level.
func (l *Logger) AgentError(agentName, workflowID string, duration time.Duration, err error) {
	l.inner.Error("agent error",
		"agent", agentName,
		"workflow_id", workflowID,
		"duration_ms", duration.Milliseconds(),
		"error", err,
	)
}

// AgentRetry logs a retry attempt at Warn level.
func (l *Logger) AgentRetry(agentName string, attempt, maxAttempts int, delay time.Duration, err error) {
	l.inner.Warn("agent retry",
		"agent", agentName,
		"attempt", attempt,
		"max_attempts", maxAttempts,
		"delay_ms", delay.Milliseconds(),
		"error", err,
	)
}

// WorkflowStart logs the start of a workflow run at Info level.
func (l *Logger) WorkflowStart(workflowID string, nodeCount int) {
	l.inner.Info("workflow start",
		"workflow_id", workflowID,
		"node_count", nodeCount,
	)
}

// WorkflowEnd logs the completion of a workflow run at Info level.
func (l *Logger) WorkflowEnd(workflowID string, duration time.Duration, err error) {
	if err != nil {
		l.inner.Error("workflow end",
			"workflow_id", workflowID,
			"duration_ms", duration.Milliseconds(),
			"error", err,
		)
		return
	}
	l.inner.Info("workflow end",
		"workflow_id", workflowID,
		"duration_ms", duration.Milliseconds(),
	)
}

// With returns a new Logger with additional key-value pairs added to all log entries.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{inner: l.inner.With(args...)}
}

// ConsumeEvents processes events from an event channel and logs them.
// Run in a goroutine; blocks until the channel is closed.
func (l *Logger) ConsumeEvents(events <-chan Event) {
	for event := range events {
		switch event.Type {
		case EventAgentStart:
			l.AgentStart(event.AgentName, event.WorkflowID, event.InputSize)
		case EventAgentEnd:
			l.AgentEnd(event.AgentName, event.WorkflowID, event.Duration, event.OutputSize)
		case EventAgentError:
			errVal := error(nil)
			if event.Error != "" {
				errVal = &stringError{msg: event.Error}
			}
			l.AgentError(event.AgentName, event.WorkflowID, event.Duration, errVal)
		}
	}
}

// stringError is a minimal error implementation for reconstructed error messages.
type stringError struct{ msg string }

func (e *stringError) Error() string { return e.msg }
