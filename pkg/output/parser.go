// Package output provides structured output parsing for LLM responses.
//
// It enables extracting typed Go structs from LLM text responses, with
// automatic JSON Schema generation and multi-strategy parsing.
//
// Key types:
//   - Parser[O]: interface for parsing raw LLM output into typed values
//   - JSONParser[O]: built-in implementation with three-tier extraction
//   - SchemaFor[O](): generates JSON Schema from Go struct via reflection
//   - NewStructuredAgent: creates an Agent[I, O] that produces typed output
package output

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Parser extracts a typed value from raw LLM text output.
type Parser[O any] interface {
	// Parse extracts and deserializes a value of type O from raw text.
	Parse(raw string) (O, error)

	// FormatInstruction returns text to append to the prompt that instructs
	// the LLM to produce output in the expected format.
	FormatInstruction() string
}

// JSONParser extracts JSON from LLM responses and deserializes into type O.
//
// It uses a three-tier extraction strategy:
//  1. Direct json.Unmarshal of the entire response
//  2. Extract from ```json ... ``` code blocks
//  3. Find the first { to the last } (or [ to ])
type JSONParser[O any] struct{}

// NewJSONParser creates a new JSONParser for type O.
func NewJSONParser[O any]() *JSONParser[O] {
	return &JSONParser[O]{}
}

var jsonCodeBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")

// Parse attempts to extract and unmarshal JSON from the raw LLM response.
func (p *JSONParser[O]) Parse(raw string) (O, error) {
	var zero O
	trimmed := strings.TrimSpace(raw)

	// Strategy 1: Direct unmarshal.
	var result O
	if err := json.Unmarshal([]byte(trimmed), &result); err == nil {
		return result, nil
	}

	// Strategy 2: Extract from ```json ... ``` code block.
	if matches := jsonCodeBlockRe.FindStringSubmatch(trimmed); len(matches) > 1 {
		if err := json.Unmarshal([]byte(strings.TrimSpace(matches[1])), &result); err == nil {
			return result, nil
		}
	}

	// Strategy 3: Find outermost JSON object or array.
	if extracted := extractJSON(trimmed); extracted != "" {
		if err := json.Unmarshal([]byte(extracted), &result); err == nil {
			return result, nil
		}
	}

	return zero, fmt.Errorf("output: failed to extract JSON from response:\n%s", truncate(raw, 200))
}

// FormatInstruction returns a prompt instruction for JSON output.
func (p *JSONParser[O]) FormatInstruction() string {
	schema := SchemaFor[O]()
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "Respond with valid JSON only. No additional text."
	}
	return fmt.Sprintf(
		"You MUST respond with valid JSON only, no additional text or markdown.\nThe JSON must conform to this schema:\n%s",
		string(schemaJSON),
	)
}

// extractJSON finds the outermost JSON object or array in a string.
func extractJSON(s string) string {
	// Try object first.
	if result := extractBracketed(s, '{', '}'); result != "" {
		return result
	}
	// Try array.
	return extractBracketed(s, '[', ']')
}

func extractBracketed(s string, open, close byte) string {
	start := strings.IndexByte(s, open)
	if start == -1 {
		return ""
	}
	// Find matching close bracket, respecting nesting.
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		if escaped {
			escaped = false
			continue
		}
		ch := s[i]
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
		if ch == open {
			depth++
		} else if ch == close {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
