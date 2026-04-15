package memory

import (
	"context"
	"sync"
)

// BufferStore keeps all messages in memory. It is suitable for short
// conversations where the full history fits within the LLM context window.
//
// BufferStore is safe for concurrent use.
type BufferStore struct {
	mu       sync.RWMutex
	messages []Message
}

// NewBufferStore creates an empty BufferStore.
func NewBufferStore() *BufferStore {
	return &BufferStore{}
}

// Add appends a message to the history.
func (s *BufferStore) Add(_ context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

// Messages returns a copy of the full conversation history.
func (s *BufferStore) Messages(_ context.Context) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out, nil
}

// Clear removes all messages.
func (s *BufferStore) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = s.messages[:0]
	return nil
}
