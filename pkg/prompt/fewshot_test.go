package prompt

import (
	"strings"
	"testing"
)

func TestFewShotBuilder_Build(t *testing.T) {
	prompt := NewFewShot("Translate English to French.").
		AddExample("Hello", "Bonjour").
		AddExample("Goodbye", "Au revoir").
		Build()

	if !strings.Contains(prompt, "Translate English to French.") {
		t.Error("should contain instruction")
	}
	if !strings.Contains(prompt, "Input: Hello") {
		t.Error("should contain first example input")
	}
	if !strings.Contains(prompt, "Output: Bonjour") {
		t.Error("should contain first example output")
	}
	if !strings.Contains(prompt, "Input: Goodbye") {
		t.Error("should contain second example input")
	}
}

func TestFewShotBuilder_CustomLabels(t *testing.T) {
	prompt := NewFewShot("Classify sentiment.").
		WithLabels("Text", "Sentiment").
		AddExample("I love it", "positive").
		Build()

	if !strings.Contains(prompt, "Text: I love it") {
		t.Error("should use custom input label")
	}
	if !strings.Contains(prompt, "Sentiment: positive") {
		t.Error("should use custom output label")
	}
}

func TestFewShotBuilder_BuildWithInput(t *testing.T) {
	prompt := NewFewShot("Translate.").
		AddExample("Hello", "Bonjour").
		BuildWithInput("Thank you")

	if !strings.Contains(prompt, "Input: Thank you") {
		t.Error("should contain user input")
	}
	if !strings.HasSuffix(prompt, "Output:") {
		t.Error("should end with Output: label")
	}
}

func TestFewShotBuilder_NoExamples(t *testing.T) {
	prompt := NewFewShot("Just an instruction.").Build()
	if prompt != "Just an instruction." {
		t.Errorf("unexpected: %q", prompt)
	}
}
