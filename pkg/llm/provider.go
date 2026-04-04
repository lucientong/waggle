// Package llm provides LLM provider implementations and routing for Waggle.
//
// All providers implement the Provider interface, which exposes a simple Chat
// and ChatStream API. Implementations use net/http directly to avoid external
// SDK dependencies.
package llm

import "context"

// Role represents the role of a message in a conversation.
type Role string

const (
	// RoleSystem is the system instruction role.
	RoleSystem Role = "system"
	// RoleUser is the human user role.
	RoleUser Role = "user"
	// RoleAssistant is the LLM assistant role.
	RoleAssistant Role = "assistant"
)

// Message represents a single turn in a conversation.
type Message struct {
	// Role identifies the speaker: system, user, or assistant.
	Role Role `json:"role"`
	// Content is the text content of the message.
	Content string `json:"content"`
}

// ProviderInfo holds metadata about a provider, used by the router for
// intelligent provider selection.
type ProviderInfo struct {
	// Name is the provider identifier (e.g., "openai", "anthropic", "ollama").
	Name string
	// Model is the model name (e.g., "gpt-4o", "claude-3-5-sonnet-20241022").
	Model string
	// CostPer1KTokens is the estimated cost in USD per 1,000 tokens (input + output average).
	// Set to 0 for local models.
	CostPer1KTokens float64
	// AvgLatencyMs is the approximate average latency for a typical request in milliseconds.
	AvgLatencyMs int
	// MaxContextTokens is the maximum context window size in tokens.
	MaxContextTokens int
	// SupportsStreaming indicates whether the provider supports ChatStream.
	SupportsStreaming bool
	// Tags are arbitrary labels (e.g., "fast", "cheap", "powerful") used for routing hints.
	Tags []string
}

// Provider is the core interface for LLM backends.
//
// Implementations must handle authentication, retries, and rate limiting
// internally or leave those concerns to the Agent wrappers (WithRetry, WithTimeout).
type Provider interface {
	// Info returns metadata about this provider instance.
	Info() ProviderInfo

	// Chat sends a list of messages and returns the assistant's complete response.
	// This is a blocking call that waits for the full response.
	Chat(ctx context.Context, messages []Message) (string, error)

	// ChatStream sends a list of messages and returns a channel that emits
	// response tokens as they arrive. The channel is closed when the response is
	// complete or an error occurs. Callers should check ctx for cancellation.
	//
	// If the provider does not support streaming, it may return the full response
	// as a single token on the channel.
	ChatStream(ctx context.Context, messages []Message) (<-chan string, error)
}
