package stream

import (
	"context"
	"fmt"
	"time"

	"github.com/lucientong/waggle/pkg/agent"
)

// observableChain2 wraps two agents and emits steps to an observer.
type observableChain2[A, B, C any] struct {
	name   string
	first  agent.Agent[A, B]
	second agent.Agent[B, C]
	obs    Observer
}

func (c *observableChain2[A, B, C]) Name() string { return c.name }

func (c *observableChain2[A, B, C]) Run(ctx context.Context, input A) (C, error) {
	var zero C

	// Step 1: first agent.
	c.obs.OnStep(Step{AgentName: c.first.Name(), Type: StepStarted, Index: 0, Timestamp: time.Now()})
	mid, err := c.first.Run(ctx, input)
	if err != nil {
		c.obs.OnStep(Step{AgentName: c.first.Name(), Type: StepError, Content: err.Error(), Index: 0, Timestamp: time.Now()})
		return zero, fmt.Errorf("observable chain %q stage 0 (%s): %w", c.name, c.first.Name(), err)
	}
	c.obs.OnStep(Step{AgentName: c.first.Name(), Type: StepCompleted, Content: fmt.Sprint(mid), Index: 0, Timestamp: time.Now()})

	// Check context between stages.
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	// Step 2: second agent.
	c.obs.OnStep(Step{AgentName: c.second.Name(), Type: StepStarted, Index: 1, Timestamp: time.Now()})
	result, err := c.second.Run(ctx, mid)
	if err != nil {
		c.obs.OnStep(Step{AgentName: c.second.Name(), Type: StepError, Content: err.Error(), Index: 1, Timestamp: time.Now()})
		return zero, fmt.Errorf("observable chain %q stage 1 (%s): %w", c.name, c.second.Name(), err)
	}
	c.obs.OnStep(Step{AgentName: c.second.Name(), Type: StepCompleted, Content: fmt.Sprint(result), Index: 1, Timestamp: time.Now()})

	return result, nil
}

// ObservableChain2 creates a two-stage observable chain that emits Step events
// to the observer as each agent starts and completes.
func ObservableChain2[A, B, C any](first agent.Agent[A, B], second agent.Agent[B, C], obs Observer) agent.Agent[A, C] {
	return &observableChain2[A, B, C]{
		name:   first.Name() + "→" + second.Name(),
		first:  first,
		second: second,
		obs:    obs,
	}
}

// observableChain3 wraps three agents.
type observableChain3[A, B, C, D any] struct {
	name  string
	inner agent.Agent[A, D]
	obs   Observer
}

func (c *observableChain3[A, B, C, D]) Name() string     { return c.name }
func (c *observableChain3[A, B, C, D]) Run(ctx context.Context, input A) (D, error) {
	return c.inner.Run(ctx, input)
}

// ObservableChain3 creates a three-stage observable chain.
func ObservableChain3[A, B, C, D any](
	first agent.Agent[A, B],
	second agent.Agent[B, C],
	third agent.Agent[C, D],
	obs Observer,
) agent.Agent[A, D] {
	// Compose as Chain2(Chain2(first, second), third) with observation.
	inner := ObservableChain2(
		ObservableChain2(first, second, obs),
		third,
		obs,
	)
	return inner
}
