// Package prompt provides lightweight prompt template and few-shot example utilities.
//
// Templates use {{variable}} placeholders for string interpolation, offering a
// simpler syntax than Go's text/template. FewShotBuilder helps construct prompts
// with input/output examples.
package prompt

import (
	"fmt"
	"strings"
)

// Template is a prompt template with {{variable}} placeholder support.
//
// Templates are immutable — each With* method returns a new Template, allowing
// safe reuse and composition.
type Template struct {
	raw  string
	vars map[string]string
}

// New creates a Template from a raw template string.
// Placeholders use the syntax {{key}}.
func New(tmpl string) *Template {
	return &Template{
		raw:  tmpl,
		vars: make(map[string]string),
	}
}

// WithVar returns a new Template with the given variable set.
func (t *Template) WithVar(key, value string) *Template {
	newVars := make(map[string]string, len(t.vars)+1)
	for k, v := range t.vars {
		newVars[k] = v
	}
	newVars[key] = value
	return &Template{raw: t.raw, vars: newVars}
}

// WithVars returns a new Template with all given variables set.
func (t *Template) WithVars(vars map[string]string) *Template {
	newVars := make(map[string]string, len(t.vars)+len(vars))
	for k, v := range t.vars {
		newVars[k] = v
	}
	for k, v := range vars {
		newVars[k] = v
	}
	return &Template{raw: t.raw, vars: newVars}
}

// Render replaces all {{key}} placeholders with their values and returns
// the resulting string. Returns an error if any placeholder has no value.
func (t *Template) Render() (string, error) {
	result := t.raw
	// Check for missing variables.
	var missing []string
	for {
		start := strings.Index(result, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}}")
		if end == -1 {
			break
		}
		key := strings.TrimSpace(result[start+2 : start+end])
		if val, ok := t.vars[key]; ok {
			result = result[:start] + val + result[start+end+2:]
		} else {
			missing = append(missing, key)
			// Skip past this placeholder to find others.
			result = result[:start] + "<<MISSING:" + key + ">>" + result[start+end+2:]
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("prompt template: missing variables: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// MustRender is like Render but panics on error.
func (t *Template) MustRender() string {
	s, err := t.Render()
	if err != nil {
		panic(err)
	}
	return s
}

// AsPromptFunc returns a function suitable for use as an LLMAgent's promptFn.
// The input value is converted to string via fmt.Sprint and used to fill
// the "input" variable.
func (t *Template) AsPromptFunc() func(any) string {
	return func(input any) string {
		rendered := t.WithVar("input", fmt.Sprint(input))
		s, err := rendered.Render()
		if err != nil {
			return fmt.Sprint(input) // fallback
		}
		return s
	}
}
