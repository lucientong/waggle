package waggle

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lucientong/waggle/pkg/agent"
)

// ---- Parallel ---------------------------------------------------------------

// ParallelResults holds the outputs of a Parallel execution.
// Each entry corresponds to an agent in the same order as they were passed to Parallel.
type ParallelResults[O any] struct {
	Results []O
	Errors  []error
}

// parallelAgent runs multiple agents concurrently with the same input and
// collects all results. It implements Agent[I, ParallelResults[O]].
type parallelAgent[I, O any] struct {
	name   string
	agents []agent.Agent[I, O]
}

func (p *parallelAgent[I, O]) Name() string { return p.name }

// Run executes all agents concurrently with the same input.
// All agents run regardless of individual failures; errors are collected into
// ParallelResults.Errors. The Run itself never returns an error — callers must
// inspect ParallelResults.Errors to detect partial failures.
func (p *parallelAgent[I, O]) Run(ctx context.Context, input I) (ParallelResults[O], error) {
	n := len(p.agents)
	results := make([]O, n)
	errs := make([]error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i, a := range p.agents {
		i, a := i, a
		go func() {
			defer wg.Done()
			results[i], errs[i] = a.Run(ctx, input)
		}()
	}
	wg.Wait()

	return ParallelResults[O]{Results: results, Errors: errs}, nil
}

// Parallel creates an agent that fans out to all provided agents concurrently
// and collects all results into a ParallelResults[O].
//
// All agents must have the same input type I and output type O.
// Results are ordered identically to the agents slice.
//
// Example:
//
//	pa := waggle.Parallel("analyzers", analyzer1, analyzer2, analyzer3)
//	results, _ := pa.Run(ctx, codeContent)
//	// results.Results[0] = analyzer1 output, etc.
func Parallel[I, O any](name string, agents ...agent.Agent[I, O]) agent.Agent[I, ParallelResults[O]] {
	return &parallelAgent[I, O]{name: name, agents: agents}
}

// ---- Race -------------------------------------------------------------------

// raceAgent runs multiple agents concurrently and returns the first successful result.
type raceAgent[I, O any] struct {
	name   string
	agents []agent.Agent[I, O]
}

func (r *raceAgent[I, O]) Name() string { return r.name }

// Run starts all agents concurrently. The first agent to return a non-error result
// wins; its output is returned and all other agents are cancelled via context.
// If all agents fail, an error aggregating all failures is returned.
func (r *raceAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	type outcome struct {
		value O
		err   error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan outcome, len(r.agents))
	for _, a := range r.agents {
		a := a
		go func() {
			v, err := a.Run(ctx, input)
			ch <- outcome{value: v, err: err}
		}()
	}

	var errs []error
	for i := 0; i < len(r.agents); i++ {
		select {
		case <-ctx.Done():
			var zero O
			return zero, ctx.Err()
		case out := <-ch:
			if out.err == nil {
				cancel() // stop all remaining agents
				return out.value, nil
			}
			errs = append(errs, out.err)
		}
	}

	var zero O
	return zero, fmt.Errorf("race %q: all agents failed: %w", r.name, errors.Join(errs...))
}

// Race creates an agent that runs all provided agents concurrently and returns
// the result of the first one to succeed. The others are cancelled.
//
// Useful for latency hedging (run a primary and a backup; take whichever responds first).
//
// Example:
//
//	ra := waggle.Race("llm-race", openaiAgent, anthropicAgent, ollamaAgent)
//	result, _ := ra.Run(ctx, prompt) // fastest LLM wins
func Race[I, O any](name string, agents ...agent.Agent[I, O]) agent.Agent[I, O] {
	return &raceAgent[I, O]{name: name, agents: agents}
}

// ---- Vote -------------------------------------------------------------------

// VoteFunc is a function that selects a winner from a slice of candidate outputs.
// It is used by the Vote pattern to implement custom consensus logic.
type VoteFunc[O any] func(candidates []O) (O, error)

// MajorityVote is a VoteFunc that selects the value that appears most frequently.
// Uses fmt.Sprintf for comparison, so O must have a meaningful string representation.
// Returns an error if no majority is found (all candidates differ).
func MajorityVote[O any]() VoteFunc[O] {
	return func(candidates []O) (O, error) {
		counts := make(map[string]int)
		index := make(map[string]O)
		for _, c := range candidates {
			key := fmt.Sprintf("%v", c)
			counts[key]++
			index[key] = c
		}
		best := ""
		bestCount := 0
		for k, n := range counts {
			if n > bestCount {
				bestCount = n
				best = k
			}
		}
		if bestCount > len(candidates)/2 {
			return index[best], nil
		}
		var zero O
		return zero, fmt.Errorf("no majority vote winner among %d candidates", len(candidates))
	}
}

// voteAgent runs multiple agents concurrently and applies a vote function to select
// the consensus result.
type voteAgent[I, O any] struct {
	name     string
	agents   []agent.Agent[I, O]
	voteFunc VoteFunc[O]
}

func (v *voteAgent[I, O]) Name() string { return v.name }

// Run executes all agents concurrently, then applies voteFunc to the successful outputs.
// Agents that return errors are excluded from the vote. If not enough agents succeed
// for a consensus, an error is returned.
func (v *voteAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	type outcome struct {
		value O
		err   error
	}

	ch := make(chan outcome, len(v.agents))
	for _, a := range v.agents {
		a := a
		go func() {
			val, err := a.Run(ctx, input)
			ch <- outcome{value: val, err: err}
		}()
	}

	candidates := make([]O, 0, len(v.agents))
	for i := 0; i < len(v.agents); i++ {
		select {
		case <-ctx.Done():
			var zero O
			return zero, ctx.Err()
		case out := <-ch:
			if out.err == nil {
				candidates = append(candidates, out.value)
			}
		}
	}

	if len(candidates) == 0 {
		var zero O
		return zero, fmt.Errorf("vote %q: all agents failed", v.name)
	}

	return v.voteFunc(candidates)
}

// Vote creates an agent that runs all provided agents concurrently and applies
// a vote function to select the consensus result.
//
// Use MajorityVote() for simple majority consensus.
//
// Example:
//
//	va := waggle.Vote("quality-vote", MajorityVote[string](), judge1, judge2, judge3)
//	result, _ := va.Run(ctx, article) // 2/3 judges must agree
func Vote[I, O any](name string, voteFunc VoteFunc[O], agents ...agent.Agent[I, O]) agent.Agent[I, O] {
	return &voteAgent[I, O]{name: name, agents: agents, voteFunc: voteFunc}
}

// ---- Router -----------------------------------------------------------------

// RouterFunc examines the input and returns the key of the branch to route to.
type RouterFunc[I any] func(ctx context.Context, input I) (string, error)

// routerAgent routes input to one of several named branches based on a routing function.
type routerAgent[I, O any] struct {
	name     string
	routeFn  RouterFunc[I]
	branches map[string]agent.Agent[I, O]
	fallback agent.Agent[I, O] // optional, used when routeFn returns an unknown key
}

func (r *routerAgent[I, O]) Name() string { return r.name }

// Run calls routeFn to determine the target branch, then delegates to that branch.
// If the branch key is not found and a fallback is set, the fallback is used.
// If neither the branch nor fallback is found, an error is returned.
func (r *routerAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	key, err := r.routeFn(ctx, input)
	if err != nil {
		var zero O
		return zero, fmt.Errorf("router %q: routing function failed: %w", r.name, err)
	}

	branch, ok := r.branches[key]
	if !ok {
		if r.fallback != nil {
			branch = r.fallback
		} else {
			var zero O
			return zero, fmt.Errorf("router %q: unknown branch key %q", r.name, key)
		}
	}

	return branch.Run(ctx, input)
}

// RouterOption configures a routerAgent.
type RouterOption[I, O any] func(*routerAgent[I, O])

// WithFallback sets a fallback agent used when the routing function returns an
// unrecognised branch key.
func WithFallback[I, O any](a agent.Agent[I, O]) RouterOption[I, O] {
	return func(r *routerAgent[I, O]) {
		r.fallback = a
	}
}

// Router creates an agent that inspects each input and routes it to one of the
// named branches. The routing decision is made by routeFn, which returns a branch key.
//
// Example:
//
//	ra := waggle.Router("triage", classifyFn,
//	    map[string]agent.Agent[string, string]{
//	        "bug":     bugHandler,
//	        "feature": featureHandler,
//	    },
//	    waggle.WithFallback[string, string](defaultHandler),
//	)
func Router[I, O any](
	name string,
	routeFn RouterFunc[I],
	branches map[string]agent.Agent[I, O],
	opts ...RouterOption[I, O],
) agent.Agent[I, O] {
	r := &routerAgent[I, O]{
		name:     name,
		routeFn:  routeFn,
		branches: branches,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ---- Loop -------------------------------------------------------------------

// LoopCondition examines the latest output and returns true if the loop should
// continue iterating, or false if the loop should terminate.
type LoopCondition[O any] func(output O) bool

// loopAgent repeatedly runs an agent until a condition is met or max iterations
// is reached.
type loopAgent[I, O any] struct {
	name          string
	inner         agent.Agent[O, O] // loop body: same input/output type for chaining
	init          agent.Agent[I, O] // first pass: converts I to O
	condition     LoopCondition[O]
	maxIterations int
}

func (l *loopAgent[I, O]) Name() string { return l.name }

// Run executes the init agent once to produce the first O, then repeatedly
// runs the inner agent on its own output until condition returns false or
// maxIterations is reached.
func (l *loopAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	// Initial transformation: I -> O
	current, err := l.init.Run(ctx, input)
	if err != nil {
		return current, fmt.Errorf("loop %q init: %w", l.name, err)
	}

	for i := 0; i < l.maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return current, err
		}
		if !l.condition(current) {
			return current, nil
		}

		next, err := l.inner.Run(ctx, current)
		if err != nil {
			return current, fmt.Errorf("loop %q iteration %d: %w", l.name, i+1, err)
		}
		current = next
	}

	return current, fmt.Errorf("loop %q: max iterations (%d) reached", l.name, l.maxIterations)
}

// DefaultMaxLoopIterations is the default cap on loop iterations if not specified.
const DefaultMaxLoopIterations = 10

// LoopOption configures a loopAgent.
type LoopOption[I, O any] func(*loopAgent[I, O])

// WithMaxIterations sets the maximum number of loop iterations.
// The loop returns an error if the condition is still true after this many iterations.
func WithMaxIterations[I, O any](n int) LoopOption[I, O] {
	return func(l *loopAgent[I, O]) {
		if n > 0 {
			l.maxIterations = n
		}
	}
}

// Loop creates an agent that first runs initAgent to produce an initial output,
// then repeatedly runs bodyAgent on its own output until condition(output) returns false.
//
// initAgent converts the input type I to the loop's working type O.
// bodyAgent refines O -> O on each iteration.
// condition returns true while more iterations are needed (i.e., the loop continues).
//
// Example:
//
//	// Refine text until quality score >= 0.9
//	la := waggle.Loop("refine",
//	    draftAgent,   // string -> string (init: produces first draft)
//	    improveAgent, // string -> string (body: improves draft)
//	    func(draft string) bool { return qualityScore(draft) < 0.9 },
//	    waggle.WithMaxIterations[string, string](5),
//	)
func Loop[I, O any](
	name string,
	initAgent agent.Agent[I, O],
	bodyAgent agent.Agent[O, O],
	condition LoopCondition[O],
	opts ...LoopOption[I, O],
) agent.Agent[I, O] {
	l := &loopAgent[I, O]{
		name:          name,
		inner:         bodyAgent,
		init:          initAgent,
		condition:     condition,
		maxIterations: DefaultMaxLoopIterations,
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}
