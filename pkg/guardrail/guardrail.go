// Package guardrail provides input/output validation for AI agent pipelines.
//
// A Guard validates data flowing through a pipeline and can block or transform
// it. Guards wrap Agent[I, O] as decorators, consistent with waggle's
// composition-over-DSL philosophy.
//
// Built-in guards:
//   - MaxLength: rejects inputs/outputs exceeding a character limit
//   - RegexMatch: validates against a regex pattern
//   - JSONValid: ensures output is valid JSON
//   - ContentFilter: blocks outputs matching forbidden patterns (e.g., PII)
//   - Custom: build your own with ValidatorFunc
package guardrail

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/lucientong/waggle/pkg/agent"
)

// Validator checks a string value and returns an error if it fails validation.
type Validator interface {
	// Validate checks the value. Returns nil if valid, error if not.
	Validate(value string) error

	// Name returns a human-readable name for this validator.
	Name() string
}

// ValidatorFunc adapts a function into a Validator.
type ValidatorFunc struct {
	name string
	fn   func(string) error
}

// NewValidator creates a Validator from a function.
func NewValidator(name string, fn func(string) error) *ValidatorFunc {
	return &ValidatorFunc{name: name, fn: fn}
}

func (v *ValidatorFunc) Validate(value string) error { return v.fn(value) }
func (v *ValidatorFunc) Name() string                { return v.name }

// --- Built-in Validators ---

// MaxLength rejects strings exceeding maxChars characters.
func MaxLength(maxChars int) Validator {
	return NewValidator(
		fmt.Sprintf("max_length(%d)", maxChars),
		func(s string) error {
			if utf8.RuneCountInString(s) > maxChars {
				return fmt.Errorf("exceeds maximum length of %d characters (got %d)", maxChars, utf8.RuneCountInString(s))
			}
			return nil
		},
	)
}

// MinLength rejects strings shorter than minChars characters.
func MinLength(minChars int) Validator {
	return NewValidator(
		fmt.Sprintf("min_length(%d)", minChars),
		func(s string) error {
			if utf8.RuneCountInString(s) < minChars {
				return fmt.Errorf("below minimum length of %d characters (got %d)", minChars, utf8.RuneCountInString(s))
			}
			return nil
		},
	)
}

// RegexMatch validates that the string matches the given regex pattern.
func RegexMatch(pattern string) Validator {
	re := regexp.MustCompile(pattern)
	return NewValidator(
		fmt.Sprintf("regex(%s)", pattern),
		func(s string) error {
			if !re.MatchString(s) {
				return fmt.Errorf("does not match pattern %q", pattern)
			}
			return nil
		},
	)
}

// RegexReject rejects strings that match the given regex pattern.
func RegexReject(pattern string, description string) Validator {
	re := regexp.MustCompile(pattern)
	return NewValidator(
		fmt.Sprintf("reject(%s)", description),
		func(s string) error {
			if re.MatchString(s) {
				return fmt.Errorf("matched forbidden pattern: %s", description)
			}
			return nil
		},
	)
}

// JSONValid ensures the string is valid JSON (starts with { or [).
func JSONValid() Validator {
	return NewValidator("json_valid", func(s string) error {
		trimmed := strings.TrimSpace(s)
		if len(trimmed) == 0 {
			return fmt.Errorf("empty string is not valid JSON")
		}
		if trimmed[0] != '{' && trimmed[0] != '[' {
			return fmt.Errorf("not valid JSON (starts with %q)", string(trimmed[0]))
		}
		// Quick bracket balance check.
		depth := 0
		inString := false
		escaped := false
		for _, ch := range trimmed {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' && inString {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = !inString
				continue
			}
			if inString {
				continue
			}
			if ch == '{' || ch == '[' {
				depth++
			} else if ch == '}' || ch == ']' {
				depth--
			}
		}
		if depth != 0 {
			return fmt.Errorf("unbalanced JSON brackets")
		}
		return nil
	})
}

// ContentFilter rejects strings containing any of the forbidden substrings.
// Matching is case-insensitive.
func ContentFilter(forbidden []string) Validator {
	lower := make([]string, len(forbidden))
	for i, f := range forbidden {
		lower[i] = strings.ToLower(f)
	}
	return NewValidator("content_filter", func(s string) error {
		sl := strings.ToLower(s)
		for _, f := range lower {
			if strings.Contains(sl, f) {
				return fmt.Errorf("contains forbidden content")
			}
		}
		return nil
	})
}

// Common PII patterns for convenience.
var (
	// PIIEmail detects email addresses.
	PIIEmail = RegexReject(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`, "email address")
	// PIIPhone detects phone numbers (common formats).
	PIIPhone = RegexReject(`(\+?\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}`, "phone number")
	// PIISSNLike detects SSN-like patterns (XXX-XX-XXXX).
	PIISSNLike = RegexReject(`\b\d{3}-\d{2}-\d{4}\b`, "SSN-like number")
)

// --- Guard Decorators ---

// GuardViolationError is returned when a guard validator fails.
type GuardViolationError struct {
	AgentName     string
	ValidatorName string
	Phase         string // "input" or "output"
	Err           error
}

func (e *GuardViolationError) Error() string {
	return fmt.Sprintf("guard violation on %s %s (%s): %s", e.Phase, e.AgentName, e.ValidatorName, e.Err)
}

func (e *GuardViolationError) Unwrap() error { return e.Err }

// WithInputGuard wraps an Agent[string, O] with input validation.
// If any validator fails, the agent is not called and an error is returned.
func WithInputGuard[O any](a agent.Agent[string, O], validators ...Validator) agent.Agent[string, O] {
	return &inputGuardAgent[O]{inner: a, validators: validators}
}

type inputGuardAgent[O any] struct {
	inner      agent.Agent[string, O]
	validators []Validator
}

func (g *inputGuardAgent[O]) Name() string { return g.inner.Name() }

func (g *inputGuardAgent[O]) Run(ctx context.Context, input string) (O, error) {
	var zero O
	for _, v := range g.validators {
		if err := v.Validate(input); err != nil {
			return zero, &GuardViolationError{
				AgentName:     g.inner.Name(),
				ValidatorName: v.Name(),
				Phase:         "input",
				Err:           err,
			}
		}
	}
	return g.inner.Run(ctx, input)
}

// WithOutputGuard wraps an Agent[I, string] with output validation.
// If any validator fails on the output, an error is returned.
func WithOutputGuard[I any](a agent.Agent[I, string], validators ...Validator) agent.Agent[I, string] {
	return &outputGuardAgent[I]{inner: a, validators: validators}
}

type outputGuardAgent[I any] struct {
	inner      agent.Agent[I, string]
	validators []Validator
}

func (g *outputGuardAgent[I]) Name() string { return g.inner.Name() }

func (g *outputGuardAgent[I]) Run(ctx context.Context, input I) (string, error) {
	output, err := g.inner.Run(ctx, input)
	if err != nil {
		return "", err
	}
	for _, v := range g.validators {
		if err := v.Validate(output); err != nil {
			return "", &GuardViolationError{
				AgentName:     g.inner.Name(),
				ValidatorName: v.Name(),
				Phase:         "output",
				Err:           err,
			}
		}
	}
	return output, nil
}

// WithInputExtractGuard wraps any Agent[I, O] with input validation by extracting
// a string from I via extractFn. This allows guardrails on non-string input types.
//
// Example:
//
//	guarded := guardrail.WithInputExtractGuard(postAgent,
//	    func(review *AggregatedReview) string { return review.Summary },
//	    guardrail.MaxLength(10000),
//	    guardrail.PIIEmail,
//	)
func WithInputExtractGuard[I, O any](a agent.Agent[I, O], extractFn func(I) string, validators ...Validator) agent.Agent[I, O] {
	return &inputExtractGuardAgent[I, O]{inner: a, extractFn: extractFn, validators: validators}
}

type inputExtractGuardAgent[I, O any] struct {
	inner      agent.Agent[I, O]
	extractFn  func(I) string
	validators []Validator
}

func (g *inputExtractGuardAgent[I, O]) Name() string { return g.inner.Name() }

func (g *inputExtractGuardAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	extracted := g.extractFn(input)
	for _, v := range g.validators {
		if err := v.Validate(extracted); err != nil {
			return zero, &GuardViolationError{
				AgentName:     g.inner.Name(),
				ValidatorName: v.Name(),
				Phase:         "input",
				Err:           err,
			}
		}
	}
	return g.inner.Run(ctx, input)
}

// WithOutputExtractGuard wraps any Agent[I, O] with output validation by extracting
// a string from O via extractFn. This allows guardrails on non-string output types.
//
// Example:
//
//	guarded := guardrail.WithOutputExtractGuard(reviewAgent,
//	    func(review *AggregatedReview) string { return review.Summary },
//	    guardrail.MaxLength(5000),
//	)
func WithOutputExtractGuard[I, O any](a agent.Agent[I, O], extractFn func(O) string, validators ...Validator) agent.Agent[I, O] {
	return &outputExtractGuardAgent[I, O]{inner: a, extractFn: extractFn, validators: validators}
}

type outputExtractGuardAgent[I, O any] struct {
	inner      agent.Agent[I, O]
	extractFn  func(O) string
	validators []Validator
}

func (g *outputExtractGuardAgent[I, O]) Name() string { return g.inner.Name() }

func (g *outputExtractGuardAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	var zero O
	output, err := g.inner.Run(ctx, input)
	if err != nil {
		return zero, err
	}
	extracted := g.extractFn(output)
	for _, v := range g.validators {
		if err := v.Validate(extracted); err != nil {
			return zero, &GuardViolationError{
				AgentName:     g.inner.Name(),
				ValidatorName: v.Name(),
				Phase:         "output",
				Err:           err,
			}
		}
	}
	return output, nil
}
