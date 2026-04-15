package prompt

import (
	"fmt"
	"strings"
)

// Example represents an input/output pair for few-shot prompting.
type Example struct {
	Input  string
	Output string
}

// FewShotBuilder constructs a prompt with instruction text and input/output examples.
type FewShotBuilder struct {
	instruction string
	examples    []Example
	inputLabel  string
	outputLabel string
}

// NewFewShot creates a FewShotBuilder with the given instruction.
func NewFewShot(instruction string) *FewShotBuilder {
	return &FewShotBuilder{
		instruction: instruction,
		inputLabel:  "Input",
		outputLabel: "Output",
	}
}

// AddExample adds an input/output example pair.
func (b *FewShotBuilder) AddExample(input, output string) *FewShotBuilder {
	b.examples = append(b.examples, Example{Input: input, Output: output})
	return b
}

// WithLabels sets custom labels for input/output sections.
// Default: "Input" and "Output".
func (b *FewShotBuilder) WithLabels(inputLabel, outputLabel string) *FewShotBuilder {
	b.inputLabel = inputLabel
	b.outputLabel = outputLabel
	return b
}

// Build generates the complete few-shot prompt string.
//
// Format:
//
//	<instruction>
//
//	Input: <example1 input>
//	Output: <example1 output>
//
//	Input: <example2 input>
//	Output: <example2 output>
func Build(b *FewShotBuilder) string {
	return b.Build()
}

// Build generates the complete few-shot prompt string.
func (b *FewShotBuilder) Build() string {
	var sb strings.Builder
	sb.WriteString(b.instruction)

	for i, ex := range b.examples {
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("%s: %s\n%s: %s", b.inputLabel, ex.Input, b.outputLabel, ex.Output))
		_ = i // suppress unused
	}

	return sb.String()
}

// BuildWithInput generates the few-shot prompt with an additional input
// section at the end (without the output), ready for LLM completion.
func (b *FewShotBuilder) BuildWithInput(input string) string {
	prompt := b.Build()
	return fmt.Sprintf("%s\n\n%s: %s\n%s:", prompt, b.inputLabel, input, b.outputLabel)
}
