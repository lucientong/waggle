package llm

import (
	"context"
	"fmt"

	"github.com/lucientong/waggle/pkg/agent"
)

// PromptFunc transforms the input into a list of messages to send to the LLM.
// This allows callers to inject system prompts, conversation history, or
// input formatting logic.
type PromptFunc[I any] func(ctx context.Context, input I) ([]Message, error)

// llmAgent wraps a Provider and a PromptFunc into an Agent[I, string].
type llmAgent[I any] struct {
	name     string
	provider Provider
	promptFn PromptFunc[I]
}

// Name returns the agent's name.
func (a *llmAgent[I]) Name() string { return a.name }

// Run calls the prompt function to build messages, then invokes the LLM provider.
func (a *llmAgent[I]) Run(ctx context.Context, input I) (string, error) {
	messages, err := a.promptFn(ctx, input)
	if err != nil {
		return "", fmt.Errorf("llm agent %q: build prompt: %w", a.name, err)
	}
	result, err := a.provider.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("llm agent %q: chat: %w", a.name, err)
	}
	return result, nil
}

// NewLLMAgent creates an Agent[I, string] backed by an LLM provider.
//
// The promptFn is called for each Run invocation to build the message list.
// The provider handles the actual LLM API call.
//
// Example:
//
//	summarizer := llm.NewLLMAgent[string]("summarizer", openaiProvider,
//	    func(ctx context.Context, text string) ([]llm.Message, error) {
//	        return []llm.Message{
//	            {Role: llm.RoleSystem, Content: "You are a concise summarizer."},
//	            {Role: llm.RoleUser, Content: "Summarize: " + text},
//	        }, nil
//	    },
//	)
func NewLLMAgent[I any](name string, provider Provider, promptFn PromptFunc[I]) agent.Agent[I, string] {
	return &llmAgent[I]{
		name:     name,
		provider: provider,
		promptFn: promptFn,
	}
}

// SimplePrompt is a convenience PromptFunc that wraps the input as a user message
// with a fixed system prompt.
//
// Example:
//
//	summarizer := llm.NewLLMAgent("summarizer", provider,
//	    llm.SimplePrompt[string]("You are a helpful assistant.", func(s string) string { return s }),
//	)
func SimplePrompt[I any](systemPrompt string, format func(I) string) PromptFunc[I] {
	return func(_ context.Context, input I) ([]Message, error) {
		msgs := []Message{}
		if systemPrompt != "" {
			msgs = append(msgs, Message{Role: RoleSystem, Content: systemPrompt})
		}
		msgs = append(msgs, Message{Role: RoleUser, Content: format(input)})
		return msgs, nil
	}
}
