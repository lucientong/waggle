// Package memory provides conversation memory management for AI agents.
//
// A Store holds a sequence of messages that represent a conversation history.
// Three implementations are provided:
//
//   - BufferStore: keeps all messages in memory (good for short conversations)
//   - WindowStore: keeps the last N messages with optional system message pinning
//   - SummaryStore: compresses old messages into a summary using an LLM
//
// All implementations are safe for concurrent use.
package memory

import "context"

// Message represents a single turn in a conversation history.
// Role is a plain string (not llm.Role) to avoid import cycles between packages.
type Message struct {
	// Role identifies the speaker: "system", "user", "assistant", or "tool".
	Role string `json:"role"`
	// Content is the text content of the message.
	Content string `json:"content"`
}

// Store is the core interface for conversation memory.
//
// Implementations must be safe for concurrent use from multiple goroutines.
type Store interface {
	// Add appends a message to the conversation history.
	Add(ctx context.Context, msg Message) error

	// Messages returns the current conversation history.
	// The returned slice must not be modified by the caller.
	Messages(ctx context.Context) ([]Message, error)

	// Clear removes all messages from the store.
	Clear(ctx context.Context) error
}
