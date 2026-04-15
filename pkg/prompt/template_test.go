package prompt

import (
	"strings"
	"testing"
)

func TestTemplate_Render(t *testing.T) {
	tmpl := New("Hello, {{name}}! You are {{age}} years old.")
	result, err := tmpl.WithVar("name", "Alice").WithVar("age", "30").Render()
	if err != nil {
		t.Fatal(err)
	}
	expected := "Hello, Alice! You are 30 years old."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTemplate_WithVars(t *testing.T) {
	tmpl := New("{{greeting}}, {{name}}!")
	result, err := tmpl.WithVars(map[string]string{
		"greeting": "Hi",
		"name":     "Bob",
	}).Render()
	if err != nil {
		t.Fatal(err)
	}
	if result != "Hi, Bob!" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestTemplate_MissingVariable(t *testing.T) {
	tmpl := New("Hello, {{name}}!")
	_, err := tmpl.Render()
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention missing variable: %v", err)
	}
}

func TestTemplate_Immutability(t *testing.T) {
	tmpl := New("{{x}}")
	t1 := tmpl.WithVar("x", "a")
	t2 := tmpl.WithVar("x", "b")

	r1, _ := t1.Render()
	r2, _ := t2.Render()
	if r1 != "a" || r2 != "b" {
		t.Errorf("templates should be immutable: r1=%q r2=%q", r1, r2)
	}
}

func TestTemplate_MustRender(t *testing.T) {
	tmpl := New("{{x}}")
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustRender should panic on missing variable")
		}
	}()
	tmpl.MustRender()
}

func TestTemplate_AsPromptFunc(t *testing.T) {
	tmpl := New("Analyze this: {{input}}")
	fn := tmpl.AsPromptFunc()
	result := fn("some code")
	if result != "Analyze this: some code" {
		t.Errorf("unexpected: %q", result)
	}
}

func TestTemplate_NoPlaceholders(t *testing.T) {
	tmpl := New("Just plain text.")
	result, err := tmpl.Render()
	if err != nil {
		t.Fatal(err)
	}
	if result != "Just plain text." {
		t.Errorf("unexpected: %q", result)
	}
}

func TestTemplate_MultipleSameVar(t *testing.T) {
	tmpl := New("{{x}} and {{x}}")
	result, err := tmpl.WithVar("x", "hello").Render()
	if err != nil {
		t.Fatal(err)
	}
	if result != "hello and hello" {
		t.Errorf("unexpected: %q", result)
	}
}
