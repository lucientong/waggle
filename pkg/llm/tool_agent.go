package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lucientong/waggle/pkg/agent"
)

// ToolDefinition describes a callable tool that can be invoked by the LLM.
type ToolDefinition struct {
	// Name is the unique identifier for the tool (e.g., "search_web", "get_weather").
	Name string `json:"name"`
	// Description tells the LLM what the tool does and when to use it.
	Description string `json:"description"`
	// Parameters is a JSON Schema object describing the tool's input parameters.
	Parameters json.RawMessage `json:"parameters"`
}

// ToolCall represents a single tool invocation requested by the LLM.
type ToolCall struct {
	// ToolName is the name of the tool to call.
	ToolName string
	// Arguments is the raw JSON arguments string provided by the LLM.
	Arguments string
}

// ToolResult holds the output of a single tool execution.
type ToolResult struct {
	// ToolName identifies which tool produced this result.
	ToolName string
	// Content is the string result returned by the tool.
	Content string
	// Error is non-nil if the tool execution failed.
	Error error
}

// ToolFunc is a function that executes a tool given its JSON argument string.
// It returns the tool's result as a string or an error.
type ToolFunc func(ctx context.Context, argsJSON string) (string, error)

// ToolAgentResult holds the final output of a tool-calling agent execution.
type ToolAgentResult struct {
	// FinalResponse is the LLM's final text response after all tool calls are resolved.
	FinalResponse string
	// ToolCalls is the list of tool invocations that occurred during this run.
	ToolCalls []ToolCall
	// ToolResults is the list of results from those tool invocations.
	ToolResults []ToolResult
}

// toolCallResponse is used to parse the LLM's structured tool call response.
// The LLM is instructed to respond with this JSON when it wants to call a tool.
type toolCallResponse struct {
	// Thought is the LLM's reasoning before deciding to call a tool (optional).
	Thought string `json:"thought,omitempty"`
	// ToolCalls lists the tools the LLM wants to invoke.
	ToolCalls []struct {
		Tool string `json:"tool"`
		Args string `json:"args"`
	} `json:"tool_calls,omitempty"`
	// FinalAnswer is set when the LLM has gathered enough information to answer.
	FinalAnswer string `json:"final_answer,omitempty"`
}

// toolAgent implements an Agent[string, ToolAgentResult] with function-calling support.
// It runs a ReAct-style loop: call LLM -> execute tools -> call LLM again until done.
type toolAgent struct {
	name          string
	provider      Provider
	tools         map[string]ToolFunc
	toolDefs      []ToolDefinition
	systemPrompt  string
	maxIterations int
}

// Name returns the agent's name.
func (a *toolAgent) Name() string { return a.name }

// Run executes the tool-calling ReAct loop.
//
// The loop:
//  1. Sends messages + tool definitions to the LLM.
//  2. If the LLM requests tool calls, executes them and appends results to the conversation.
//  3. Repeats until the LLM provides a FinalAnswer or max iterations is reached.
func (a *toolAgent) Run(ctx context.Context, input string) (ToolAgentResult, error) {
	toolsJSON, err := json.Marshal(a.toolDefs)
	if err != nil {
		return ToolAgentResult{}, fmt.Errorf("tool agent %q: marshal tool defs: %w", a.name, err)
	}

	systemMsg := a.systemPrompt
	if len(a.toolDefs) > 0 {
		systemMsg += fmt.Sprintf(`

Available tools (JSON):
%s

When you need to use a tool, respond ONLY with valid JSON in this format:
{"thought": "...", "tool_calls": [{"tool": "tool_name", "args": "{\"param\": \"value\"}"}]}

When you have the final answer, respond ONLY with:
{"final_answer": "your answer here"}`, string(toolsJSON))
	}

	messages := []Message{
		{Role: RoleSystem, Content: systemMsg},
		{Role: RoleUser, Content: input},
	}

	result := ToolAgentResult{}

	for i := 0; i < a.maxIterations; i++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		response, err := a.provider.Chat(ctx, messages)
		if err != nil {
			return result, fmt.Errorf("tool agent %q: llm call: %w", a.name, err)
		}

		// Try to parse as structured tool call response.
		var parsed toolCallResponse
		if err := json.Unmarshal([]byte(response), &parsed); err != nil {
			// Not JSON — treat as plain final answer.
			result.FinalResponse = response
			return result, nil
		}

		// LLM provided a final answer.
		if parsed.FinalAnswer != "" {
			result.FinalResponse = parsed.FinalAnswer
			return result, nil
		}

		// LLM requested tool calls.
		if len(parsed.ToolCalls) == 0 {
			result.FinalResponse = response
			return result, nil
		}

		// Append the LLM's tool call request to conversation.
		messages = append(messages, Message{Role: RoleAssistant, Content: response})

		// Execute each requested tool.
		var toolResultContent string
		for _, tc := range parsed.ToolCalls {
			call := ToolCall{ToolName: tc.Tool, Arguments: tc.Args}
			result.ToolCalls = append(result.ToolCalls, call)

			fn, ok := a.tools[tc.Tool]
			if !ok {
				tr := ToolResult{ToolName: tc.Tool, Error: fmt.Errorf("tool %q not found", tc.Tool)}
				result.ToolResults = append(result.ToolResults, tr)
				toolResultContent += fmt.Sprintf("Tool %q: error: tool not found\n", tc.Tool)
				continue
			}

			output, err := fn(ctx, tc.Args)
			tr := ToolResult{ToolName: tc.Tool, Content: output, Error: err}
			result.ToolResults = append(result.ToolResults, tr)

			if err != nil {
				toolResultContent += fmt.Sprintf("Tool %q: error: %v\n", tc.Tool, err)
			} else {
				toolResultContent += fmt.Sprintf("Tool %q result: %s\n", tc.Tool, output)
			}
		}

		// Append tool results as a user message for the next iteration.
		messages = append(messages, Message{Role: RoleUser, Content: toolResultContent})
	}

	return result, fmt.Errorf("tool agent %q: max iterations (%d) reached without final answer",
		a.name, a.maxIterations)
}

// ToolAgentOption configures a tool agent.
type ToolAgentOption func(*toolAgent)

// WithToolAgentSystemPrompt sets the system prompt for the tool agent.
func WithToolAgentSystemPrompt(prompt string) ToolAgentOption {
	return func(a *toolAgent) {
		a.systemPrompt = prompt
	}
}

// WithToolAgentMaxIterations sets the maximum number of tool call iterations.
func WithToolAgentMaxIterations(n int) ToolAgentOption {
	return func(a *toolAgent) {
		if n > 0 {
			a.maxIterations = n
		}
	}
}

// NewToolAgent creates an Agent[string, ToolAgentResult] that supports function calling.
//
// tools is a map of tool name -> ToolDefinition + ToolFunc pair.
//
// Example:
//
//	ta := llm.NewToolAgent("assistant", provider,
//	    map[string]llm.ToolDefinition{
//	        "get_weather": {Name: "get_weather", Description: "Get weather for a city", Parameters: schemaJSON},
//	    },
//	    map[string]llm.ToolFunc{
//	        "get_weather": func(ctx context.Context, args string) (string, error) {
//	            // parse args JSON and call weather API
//	            return "Sunny, 22°C", nil
//	        },
//	    },
//	)
func NewToolAgent(
	name string,
	provider Provider,
	toolDefs []ToolDefinition,
	tools map[string]ToolFunc,
	opts ...ToolAgentOption,
) agent.Agent[string, ToolAgentResult] {
	a := &toolAgent{
		name:          name,
		provider:      provider,
		tools:         tools,
		toolDefs:      toolDefs,
		systemPrompt:  "You are a helpful assistant with access to tools.",
		maxIterations: 10,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
