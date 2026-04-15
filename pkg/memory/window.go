package memory

import (
	"context"
	"sync"
)

// WindowStore keeps the most recent N messages. If the first message has
// role "system", it is always retained at position 0 regardless of the window size.
//
// WindowStore is safe for concurrent use.
type WindowStore struct {
	mu       sync.RWMutex
	messages []Message
	maxSize  int
	pinFirst bool // true if messages[0] is a system message that should be pinned
}

// WindowOption configures a WindowStore.
type WindowOption func(*WindowStore)

// WithPinSystemMessage ensures that a leading system message is never evicted
// by the sliding window. Enabled by default.
func WithPinSystemMessage(pin bool) WindowOption {
	return func(w *WindowStore) {
		w.pinFirst = pin
	}
}

// NewWindowStore creates a WindowStore that retains at most maxSize messages.
// By default, a leading system message is pinned.
func NewWindowStore(maxSize int, opts ...WindowOption) *WindowStore {
	if maxSize < 1 {
		maxSize = 1
	}
	w := &WindowStore{
		maxSize:  maxSize,
		pinFirst: true,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Add appends a message and trims the window if necessary.
func (s *WindowStore) Add(_ context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	s.trim()
	return nil
}

// Messages returns a copy of the current windowed history.
func (s *WindowStore) Messages(_ context.Context) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Message, len(s.messages))
	copy(out, s.messages)
	return out, nil
}

// Clear removes all messages.
func (s *WindowStore) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = s.messages[:0]
	s.pinFirst = true // reset pinning state
	return nil
}

// trim removes old messages to stay within maxSize, respecting the pinned system message.
// Must be called with mu held.
func (s *WindowStore) trim() {
	if len(s.messages) <= s.maxSize {
		return
	}

	// Check if first message is a system message that should be pinned.
	hasPin := s.pinFirst && len(s.messages) > 0 && s.messages[0].Role == "system"

	if hasPin {
		// Keep pinned system message + last (maxSize-1) messages.
		keep := s.maxSize - 1
		if keep < 0 {
			keep = 0
		}
		tail := s.messages[len(s.messages)-keep:]
		trimmed := make([]Message, 0, s.maxSize)
		trimmed = append(trimmed, s.messages[0])
		trimmed = append(trimmed, tail...)
		s.messages = trimmed
	} else {
		// Keep only the last maxSize messages.
		s.messages = s.messages[len(s.messages)-s.maxSize:]
	}
}
