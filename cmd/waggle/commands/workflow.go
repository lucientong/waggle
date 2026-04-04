// Package commands implements the Waggle CLI subcommands.
package commands

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// WorkflowDefinition is the top-level structure for a Waggle YAML workflow file.
//
// Example YAML:
//
//	name: code-review
//	description: Automated code review pipeline
//	agents:
//	  - name: fetcher
//	    type: func
//	    description: Fetch PR content
//	  - name: reviewer
//	    type: llm
//	    model: gpt-4o
//	    prompt: "Review the following code for bugs and style issues:"
//	flow:
//	  - from: fetcher
//	    to: reviewer
type WorkflowDefinition struct {
	// Name is the workflow identifier.
	Name string `yaml:"name"`
	// Description is a human-readable summary.
	Description string `yaml:"description"`
	// Agents is the list of agent definitions.
	Agents []AgentDefinition `yaml:"agents"`
	// Flow declares the data-flow edges between agents.
	Flow []FlowEdge `yaml:"flow"`
}

// AgentDefinition describes a single agent in the workflow.
type AgentDefinition struct {
	// Name is the unique agent identifier within this workflow.
	Name string `yaml:"name"`
	// Type is the agent implementation type: "func", "llm", or "tool".
	Type string `yaml:"type"`
	// Description is a human-readable summary of the agent's purpose.
	Description string `yaml:"description"`
	// Model is the LLM model name (for type "llm").
	Model string `yaml:"model"`
	// Provider is the LLM provider name: "openai", "anthropic", or "ollama" (for type "llm").
	Provider string `yaml:"provider"`
	// Prompt is the system prompt for the LLM agent (for type "llm").
	Prompt string `yaml:"prompt"`
	// Retry configures retry behaviour for this agent.
	Retry *RetryDefinition `yaml:"retry"`
	// Timeout is the per-call timeout in seconds.
	TimeoutSecs float64 `yaml:"timeout_secs"`
}

// RetryDefinition configures retry behaviour for an agent.
type RetryDefinition struct {
	// MaxAttempts is the total number of attempts (initial + retries).
	MaxAttempts int `yaml:"max_attempts"`
	// BaseDelayMs is the initial retry delay in milliseconds.
	BaseDelayMs int `yaml:"base_delay_ms"`
	// MaxDelayMs is the maximum retry delay in milliseconds.
	MaxDelayMs int `yaml:"max_delay_ms"`
}

// FlowEdge declares a data-flow dependency between two agents.
type FlowEdge struct {
	// From is the source agent name.
	From string `yaml:"from"`
	// To is the destination agent name.
	To string `yaml:"to"`
}

// LoadWorkflow reads and parses a YAML workflow file.
func LoadWorkflow(path string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load workflow %q: %w", path, err)
	}
	return ParseWorkflow(data)
}

// ParseWorkflow parses YAML bytes into a WorkflowDefinition.
func ParseWorkflow(data []byte) (*WorkflowDefinition, error) {
	var wf WorkflowDefinition
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow YAML: %w", err)
	}
	if err := validateWorkflow(&wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

// validateWorkflow performs structural validation on a parsed WorkflowDefinition.
func validateWorkflow(wf *WorkflowDefinition) error {
	if wf.Name == "" {
		return fmt.Errorf("workflow: 'name' is required")
	}

	// Collect all agent names for reference checking.
	agentNames := make(map[string]bool, len(wf.Agents))
	for _, a := range wf.Agents {
		if a.Name == "" {
			return fmt.Errorf("workflow: all agents must have a 'name'")
		}
		if agentNames[a.Name] {
			return fmt.Errorf("workflow: duplicate agent name %q", a.Name)
		}
		agentNames[a.Name] = true
	}

	// Validate flow edges reference known agents.
	for i, edge := range wf.Flow {
		if edge.From == "" || edge.To == "" {
			return fmt.Errorf("workflow: flow edge %d must have both 'from' and 'to'", i)
		}
		if !agentNames[edge.From] {
			return fmt.Errorf("workflow: flow edge %d references unknown agent %q", i, edge.From)
		}
		if !agentNames[edge.To] {
			return fmt.Errorf("workflow: flow edge %d references unknown agent %q", i, edge.To)
		}
	}

	return nil
}
