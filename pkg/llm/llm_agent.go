package llm

import (
	"context"
	"fmt"

	"github.com/lucientong/waggle/pkg/agent"
	"github.com/lucientong/waggle/pkg/memory"
)

// PromptFunc transforms the input into a list of messages to send to the LLM.
// This allows callers to inject system prompts, conversation history, or
// input formatting logic.
type PromptFunc[I any] func(ctx context.Context, input I) ([]Message, error)

// llmAgentOptions holds optional configuration for an LLM agent.
type llmAgentOptions struct {
	memoryStore memory.Store
}

// LLMAgentOption configures an LLM agent.
type LLMAgentOption func(*llmAgentOptions)

// WithMemory attaches a memory store to the LLM agent. When set, the agent
// will prepend conversation history from the store before each call, and
// automatically record user inputs and assistant responses.
func WithMemory(store memory.Store) LLMAgentOption {
	return func(o *llmAgentOptions) {
		o.memoryStore = store
	}
}

// llmAgent wraps a Provider and a PromptFunc into an Agent[I, string].
type llmAgent[I any] struct {
	name     string
	provider Provider
	promptFn PromptFunc[I]
	opts     llmAgentOptions
}

// Name returns the agent's name.
func (a *llmAgent[I]) Name() string { return a.name }

// Run calls the prompt function to build messages, then invokes the LLM provider.
// If a memory store is configured, conversation history is prepended and the
// exchange is recorded.
func (a *llmAgent[I]) Run(ctx context.Context, input I) (string, error) {
	messages, err := a.promptFn(ctx, input)
	if err != nil {
		return "", fmt.Errorf("llm agent %q: build prompt: %w", a.name, err)
	}

	// If memory is configured, inject history and record the exchange.
	if a.opts.memoryStore != nil {
		messages, err = a.withMemory(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("llm agent %q: memory: %w", a.name, err)
		}
	}

	result, err := a.provider.Chat(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("llm agent %q: chat: %w", a.name, err)
	}

	// Record the assistant's response in memory.
	if a.opts.memoryStore != nil {
		if err := a.opts.memoryStore.Add(ctx, memory.Message{
			Role:    "assistant",
			Content: result,
		}); err != nil {
			return "", fmt.Errorf("llm agent %q: record response: %w", a.name, err)
		}
	}

	return result, nil
}

// withMemory prepends conversation history and records the user message.
func (a *llmAgent[I]) withMemory(ctx context.Context, messages []Message) ([]Message, error) {
	history, err := a.opts.memoryStore.Messages(ctx)
	if err != nil {
		return nil, err
	}

	// Record user messages from the current prompt into memory.
	for _, msg := range messages {
		if msg.Role == RoleUser {
			if err := a.opts.memoryStore.Add(ctx, memory.Message{
				Role:    string(msg.Role),
				Content: msg.Content,
			}); err != nil {
				return nil, err
			}
		}
	}

	// Build final message list: system messages from prompt first, then history,
	// then non-system messages from prompt.
	var systemMsgs []Message
	var nonSystemMsgs []Message
	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			nonSystemMsgs = append(nonSystemMsgs, msg)
		}
	}

	// Convert memory history to LLM messages.
	var historyMsgs []Message
	for _, m := range history {
		historyMsgs = append(historyMsgs, Message{
			Role:    Role(m.Role),
			Content: m.Content,
		})
	}

	result := make([]Message, 0, len(systemMsgs)+len(historyMsgs)+len(nonSystemMsgs))
	result = append(result, systemMsgs...)
	result = append(result, historyMsgs...)
	result = append(result, nonSystemMsgs...)
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
//
// With memory:
//
//	chatbot := llm.NewLLMAgent[string]("chatbot", provider, promptFn,
//	    llm.WithMemory(memory.NewWindowStore(20)),
//	)
func NewLLMAgent[I any](name string, provider Provider, promptFn PromptFunc[I], options ...LLMAgentOption) agent.Agent[I, string] {
	a := &llmAgent[I]{
		name:     name,
		provider: provider,
		promptFn: promptFn,
	}
	for _, opt := range options {
		opt(&a.opts)
	}
	return a
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
