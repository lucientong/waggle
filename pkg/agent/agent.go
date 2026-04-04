// Package agent defines the core Agent interface and foundational implementations
// for the Waggle orchestration engine.
//
// An Agent is the minimal execution unit with compile-time type safety enforced
// via Go generics. Each Agent has a name and a Run method that transforms an
// input of type I into an output of type O.
package agent

import "context"

// Agent is the minimal execution unit in Waggle.
//
// The type parameters I and O represent the input and output types respectively,
// providing compile-time type safety. Mismatched types between connected agents
// are caught at compile time, not at runtime.
//
// Example:
//
//	fetcher := agent.Func[string, []byte]("fetcher", fetchURL)
//	parser  := agent.Func[[]byte, Document]("parser", parseHTML)
//
//	// Chain2 enforces that fetcher's output type ([]byte) matches parser's input type.
//	pipeline := agent.Chain2(fetcher, parser)
type Agent[I, O any] interface {
	// Name returns the agent's identifier, used in logs, traces, and error messages.
	Name() string

	// Run executes the agent with the given context and input.
	// It returns the output or an error. Implementations must respect ctx cancellation.
	Run(ctx context.Context, input I) (O, error)
}

// UntypedAgent is a type-erased variant of Agent used in dynamic scenarios such
// as building DAGs from YAML configuration where types are not known at compile time.
//
// Prefer the typed Agent[I, O] interface whenever possible.
type UntypedAgent interface {
	// Name returns the agent's identifier.
	Name() string

	// RunUntyped executes the agent with an untyped input.
	// The caller is responsible for passing the correct concrete type.
	// Returns (output any, error).
	RunUntyped(ctx context.Context, input any) (any, error)
}

// Erase wraps a typed Agent[I, O] into an UntypedAgent.
// At runtime, RunUntyped performs a type assertion on the input to type I.
// If the assertion fails, it returns an ErrTypeMismatch error.
//
// This is intended for use in dynamic orchestration scenarios (e.g., YAML-defined
// workflows) where compile-time types are unavailable.
func Erase[I, O any](a Agent[I, O]) UntypedAgent {
	return &erasedAgent[I, O]{inner: a}
}

// erasedAgent wraps a typed Agent[I, O] to implement UntypedAgent.
type erasedAgent[I, O any] struct {
	inner Agent[I, O]
}

// Name returns the name of the underlying typed agent.
func (e *erasedAgent[I, O]) Name() string {
	return e.inner.Name()
}

// RunUntyped asserts the input to type I and delegates to the typed agent.
func (e *erasedAgent[I, O]) RunUntyped(ctx context.Context, input any) (any, error) {
	typed, ok := input.(I)
	if !ok {
		return nil, &ErrTypeMismatch{
			AgentName: e.inner.Name(),
			Got:       input,
		}
	}
	return e.inner.Run(ctx, typed)
}
