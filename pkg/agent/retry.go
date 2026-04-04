package agent

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

// retryConfig holds the configuration for the retry wrapper.
// All fields have sensible defaults applied by defaultRetryConfig.
type retryConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	jitter      bool
}

// defaultRetryConfig returns a retryConfig with conservative defaults:
// 3 attempts, 100ms base delay, 30s max delay, jitter enabled.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxAttempts: 3,
		baseDelay:   100 * time.Millisecond,
		maxDelay:    30 * time.Second,
		jitter:      true,
	}
}

// RetryOption is a functional option for configuring the retry wrapper.
type RetryOption func(*retryConfig)

// WithMaxAttempts sets the maximum number of total attempts (initial + retries).
// Must be >= 1. Values less than 1 are silently ignored.
func WithMaxAttempts(n int) RetryOption {
	return func(c *retryConfig) {
		if n >= 1 {
			c.maxAttempts = n
		}
	}
}

// WithBaseDelay sets the initial delay before the first retry.
// The delay doubles with each subsequent attempt (exponential backoff).
func WithBaseDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d > 0 {
			c.baseDelay = d
		}
	}
}

// WithMaxDelay sets the upper bound for the retry delay.
// The exponential backoff is capped at this value.
func WithMaxDelay(d time.Duration) RetryOption {
	return func(c *retryConfig) {
		if d > 0 {
			c.maxDelay = d
		}
	}
}

// WithJitter controls whether random jitter is added to the delay.
// Jitter helps avoid thundering-herd problems when many agents retry simultaneously.
// Defaults to true.
func WithJitter(enabled bool) RetryOption {
	return func(c *retryConfig) {
		c.jitter = enabled
	}
}

// retryAgent wraps an Agent[I, O] with retry logic.
type retryAgent[I, O any] struct {
	inner  Agent[I, O]
	config retryConfig
}

// Name returns the name of the underlying agent, prefixed with "retry:".
func (r *retryAgent[I, O]) Name() string {
	return "retry:" + r.inner.Name()
}

// Run executes the underlying agent, retrying on failure using exponential backoff.
//
// The retry loop checks for context cancellation before each sleep, ensuring that
// a cancelled context causes immediate termination rather than waiting out a delay.
//
// Delay formula: min(baseDelay * 2^attempt, maxDelay) [+ jitter]
func (r *retryAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	var (
		lastErr error
		zero    O
	)

	for attempt := 0; attempt < r.config.maxAttempts; attempt++ {
		// Check for context cancellation before each attempt.
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := r.inner.Run(ctx, input)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// If this was the last attempt, do not sleep.
		if attempt == r.config.maxAttempts-1 {
			break
		}

		// Calculate exponential backoff: baseDelay * 2^attempt
		delay := r.config.baseDelay * (1 << uint(attempt))
		if delay > r.config.maxDelay {
			delay = r.config.maxDelay
		}

		// Add jitter: multiply by a random factor in [0.5, 1.5)
		if r.config.jitter {
			factor := 0.5 + rand.Float64()
			delay = time.Duration(float64(delay) * factor)
		}

		slog.Warn("agent retry",
			"agent", r.inner.Name(),
			"attempt", attempt+1,
			"max_attempts", r.config.maxAttempts,
			"delay", delay,
			"error", lastErr,
		)

		// Wait for delay or context cancellation.
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
		}
	}

	return zero, &RetryExhaustedError{
		AgentName: r.inner.Name(),
		Attempts:  r.config.maxAttempts,
		LastErr:   lastErr,
	}
}

// WithRetry wraps an agent with retry logic using exponential backoff with optional jitter.
//
// Example:
//
//	reliable := agent.WithRetry(myAgent,
//	    agent.WithMaxAttempts(5),
//	    agent.WithBaseDelay(200*time.Millisecond),
//	    agent.WithMaxDelay(10*time.Second),
//	)
func WithRetry[I, O any](a Agent[I, O], opts ...RetryOption) Agent[I, O] {
	cfg := defaultRetryConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &retryAgent[I, O]{inner: a, config: cfg}
}
