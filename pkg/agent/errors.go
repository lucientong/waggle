package agent

import (
	"fmt"
	"time"
)

// ErrTypeMismatch is returned when an UntypedAgent receives an input whose
// concrete type does not match the expected type parameter I.
type ErrTypeMismatch struct {
	// AgentName is the name of the agent that encountered the mismatch.
	AgentName string
	// Got is the actual value that was passed in.
	Got any
}

// Error implements the error interface.
func (e *ErrTypeMismatch) Error() string {
	return fmt.Sprintf("agent %q: type mismatch: got %T", e.AgentName, e.Got)
}

// RetryExhaustedError is returned by the retry wrapper when all retry attempts
// have been exhausted. It wraps the last error returned by the underlying agent.
type RetryExhaustedError struct {
	// AgentName is the name of the agent that was retried.
	AgentName string
	// Attempts is the total number of attempts that were made (initial + retries).
	Attempts int
	// LastErr is the error returned by the final attempt.
	LastErr error
}

// Error implements the error interface.
func (e *RetryExhaustedError) Error() string {
	return fmt.Sprintf("agent %q: retry exhausted after %d attempt(s): %v",
		e.AgentName, e.Attempts, e.LastErr)
}

// Unwrap returns the underlying last error, enabling errors.Is and errors.As
// to inspect the error chain.
func (e *RetryExhaustedError) Unwrap() error {
	return e.LastErr
}

// TimeoutError is returned when an agent's execution exceeds its configured
// deadline. It wraps the underlying context error.
type TimeoutError struct {
	// AgentName is the name of the agent that timed out.
	AgentName string
	// Duration is the timeout that was exceeded.
	Duration time.Duration
	// Cause is the underlying context error (context.DeadlineExceeded).
	Cause error
}

// Error implements the error interface.
func (e *TimeoutError) Error() string {
	return fmt.Sprintf("agent %q: timed out after %s: %v",
		e.AgentName, e.Duration, e.Cause)
}

// Unwrap returns the underlying context error, enabling errors.Is(err, context.DeadlineExceeded).
func (e *TimeoutError) Unwrap() error {
	return e.Cause
}
