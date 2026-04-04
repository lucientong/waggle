package agent

import (
	"context"
	"time"
)

// timeoutAgent wraps an Agent[I, O] with a per-call deadline.
type timeoutAgent[I, O any] struct {
	inner    Agent[I, O]
	duration time.Duration
}

// Name returns the name of the underlying agent, prefixed with "timeout:".
func (t *timeoutAgent[I, O]) Name() string {
	return "timeout:" + t.inner.Name()
}

// Run executes the underlying agent with a deadline derived from the configured duration.
//
// If the parent ctx already has a shorter deadline, that deadline takes precedence
// (context.WithTimeout respects the minimum of the two deadlines).
//
// On timeout, Run returns a TimeoutError wrapping context.DeadlineExceeded.
func (t *timeoutAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	ctx, cancel := context.WithTimeout(ctx, t.duration)
	defer cancel()

	result, err := t.inner.Run(ctx, input)
	if err != nil {
		// Unwrap context deadline exceeded into a typed TimeoutError for better diagnostics.
		if ctx.Err() == context.DeadlineExceeded {
			var zero O
			return zero, &TimeoutError{
				AgentName: t.inner.Name(),
				Duration:  t.duration,
				Cause:     ctx.Err(),
			}
		}
		return result, err
	}
	return result, nil
}

// WithTimeout wraps an agent so that each call is bounded by the given duration.
//
// If the call exceeds the duration, Run returns a *TimeoutError.
// The wrapped agent must respect context cancellation for the timeout to be effective.
//
// Example:
//
//	bounded := agent.WithTimeout(myAgent, 5*time.Second)
func WithTimeout[I, O any](a Agent[I, O], duration time.Duration) Agent[I, O] {
	return &timeoutAgent[I, O]{inner: a, duration: duration}
}
