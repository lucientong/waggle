package agent

import (
	"context"
	"fmt"
)

// chainAgent2 represents a two-agent serial pipeline.
// It implements Agent[A, C] by running first (A->B) then second (B->C).
type chainAgent2[A, B, C any] struct {
	first  Agent[A, B]
	second Agent[B, C]
}

// Name returns a composite name showing the pipeline structure.
func (c *chainAgent2[A, B, C]) Name() string {
	return fmt.Sprintf("chain(%s->%s)", c.first.Name(), c.second.Name())
}

// Run executes the first agent and passes its output to the second agent.
// If either agent returns an error, execution short-circuits and the error is returned.
// Context cancellation is respected between stages.
func (c *chainAgent2[A, B, C]) Run(ctx context.Context, input A) (C, error) {
	var zero C

	// Check context before first stage.
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	mid, err := c.first.Run(ctx, input)
	if err != nil {
		return zero, fmt.Errorf("chain stage %q: %w", c.first.Name(), err)
	}

	// Check context between stages.
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	result, err := c.second.Run(ctx, mid)
	if err != nil {
		return zero, fmt.Errorf("chain stage %q: %w", c.second.Name(), err)
	}

	return result, nil
}

// Chain2 composes two agents into a serial pipeline.
//
// The output type of first (B) must match the input type of second (B).
// This is enforced at compile time by the type parameters.
//
//	Agent[A, B] + Agent[B, C] => Agent[A, C]
//
// Example:
//
//	fetcher   := agent.Func[string, []byte]("fetcher", fetchURL)
//	parser    := agent.Func[[]byte, Document]("parser", parseHTML)
//	pipeline  := agent.Chain2(fetcher, parser)
//	doc, err  := pipeline.Run(ctx, "https://example.com")
func Chain2[A, B, C any](first Agent[A, B], second Agent[B, C]) Agent[A, C] {
	return &chainAgent2[A, B, C]{first: first, second: second}
}

// Chain3 composes three agents into a serial pipeline.
//
//	Agent[A,B] + Agent[B,C] + Agent[C,D] => Agent[A,D]
func Chain3[A, B, C, D any](a Agent[A, B], b Agent[B, C], c Agent[C, D]) Agent[A, D] {
	return Chain2(Chain2(a, b), c)
}

// Chain4 composes four agents into a serial pipeline.
//
//	Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] => Agent[A,E]
func Chain4[A, B, C, D, E any](a Agent[A, B], b Agent[B, C], c Agent[C, D], d Agent[D, E]) Agent[A, E] {
	return Chain2(Chain3(a, b, c), d)
}

// Chain5 composes five agents into a serial pipeline.
//
//	Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] + Agent[E,F] => Agent[A,F]
func Chain5[A, B, C, D, E, F any](a Agent[A, B], b Agent[B, C], c Agent[C, D], d Agent[D, E], e Agent[E, F]) Agent[A, F] {
	return Chain2(Chain4(a, b, c, d), e)
}
