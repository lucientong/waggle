package output

import (
	"context"
	"fmt"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/llm"
)

// Option configures a structured agent.
type Option func(*structuredOpts)

type structuredOpts struct {
	maxRetries int // retry parsing on failure (re-prompts LLM)
}

// WithMaxRetries sets the maximum number of parsing retries.
// On each retry, the LLM is re-prompted with the parse error feedback.
func WithMaxRetries(n int) Option {
	return func(o *structuredOpts) {
		o.maxRetries = n
	}
}

// structuredAgent wraps an LLM provider to produce typed output.
type structuredAgent[I, O any] struct {
	name     string
	provider llm.Provider
	promptFn func(I) string
	parser   Parser[O]
	opts     structuredOpts
}

func (a *structuredAgent[I, O]) Name() string { return a.name }

func (a *structuredAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	userPrompt := a.promptFn(input)
	formatInst := a.parser.FormatInstruction()

	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: formatInst},
		{Role: llm.RoleUser, Content: userPrompt},
	}

	attempts := 1 + a.opts.maxRetries
	var lastErr error

	for i := 0; i < attempts; i++ {
		raw, err := a.provider.Chat(ctx, messages)
		if err != nil {
			return zero, fmt.Errorf("structured agent %q: chat: %w", a.name, err)
		}

		result, parseErr := a.parser.Parse(raw)
		if parseErr == nil {
			return result, nil
		}

		lastErr = parseErr

		// On retry, append the failed response and error feedback.
		if i < attempts-1 {
			messages = append(messages,
				llm.Message{Role: llm.RoleAssistant, Content: raw},
				llm.Message{Role: llm.RoleUser, Content: fmt.Sprintf(
					"Your response could not be parsed: %s\nPlease try again with valid JSON only.",
					parseErr.Error(),
				)},
			)
		}
	}

	return zero, fmt.Errorf("structured agent %q: parse failed after %d attempts: %w",
		a.name, attempts, lastErr)
}

// NewStructuredAgent creates an Agent[I, O] that prompts an LLM and parses
// the response into a typed Go value.
//
// The promptFn converts input I into a user prompt string. The LLM is
// automatically instructed to respond in JSON format matching type O's schema.
//
// Example:
//
//	type Review struct {
//	    Score    int      `json:"score" jsonschema:"description=Quality score 1-10"`
//	    Issues   []string `json:"issues"`
//	    Summary  string   `json:"summary"`
//	}
//
//	reviewer := output.NewStructuredAgent[string, Review](
//	    "reviewer", provider,
//	    func(code string) string { return "Review this code:\n" + code },
//	    output.WithMaxRetries(2),
//	)
func NewStructuredAgent[I, O any](name string, provider llm.Provider, promptFn func(I) string, options ...Option) agent.Agent[I, O] {
	a := &structuredAgent[I, O]{
		name:     name,
		provider: provider,
		promptFn: promptFn,
		parser:   NewJSONParser[O](),
	}
	for _, opt := range options {
		opt(&a.opts)
	}
	return a
}
