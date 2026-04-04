package agent

import "context"

// funcAgent is the concrete implementation of Agent[I, O] backed by a plain function.
// It is unexported; use Func to construct one.
type funcAgent[I, O any] struct {
	name string
	fn   func(ctx context.Context, input I) (O, error)
}

// Name returns the agent's name.
func (f *funcAgent[I, O]) Name() string {
	return f.name
}

// Run calls the underlying function with the provided context and input.
func (f *funcAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	return f.fn(ctx, input)
}

// Func creates an Agent[I, O] from a plain function.
//
// This is the primary way to create lightweight agents without defining a struct.
//
// Example:
//
//	upper := agent.Func[string, string]("upper", func(ctx context.Context, s string) (string, error) {
//	    return strings.ToUpper(s), nil
//	})
//
//	result, err := upper.Run(ctx, "hello") // returns "HELLO", nil
func Func[I, O any](name string, fn func(ctx context.Context, input I) (O, error)) Agent[I, O] {
	return &funcAgent[I, O]{
		name: name,
		fn:   fn,
	}
}
