package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Summarizer compresses a list of messages into a single summary string.
// Typically backed by an LLM call.
type Summarizer func(ctx context.Context, messages []Message) (string, error)

// SummaryStore compresses older messages into a summary when the history exceeds
// a threshold. The summary is stored as a system message at the beginning of the
// conversation, followed by the most recent messages.
//
// SummaryStore is safe for concurrent use.
type SummaryStore struct {
	mu         sync.RWMutex
	messages   []Message
	threshold  int        // trigger summarization when len(messages) exceeds this
	keepRecent int        // number of recent messages to keep after summarization
	summarizer Summarizer // function to generate summary
	summary    string     // accumulated summary text
}

// SummaryOption configures a SummaryStore.
type SummaryOption func(*SummaryStore)

// WithKeepRecent sets how many recent messages to preserve after summarization.
// Defaults to threshold / 2.
func WithKeepRecent(n int) SummaryOption {
	return func(s *SummaryStore) {
		if n > 0 {
			s.keepRecent = n
		}
	}
}

// NewSummaryStore creates a SummaryStore.
//
// threshold: number of messages that triggers summarization.
// summarizer: function that compresses messages into a summary string.
//
// When the message count exceeds threshold, older messages are summarized and
// replaced with a single system message containing the summary. The most recent
// messages (keepRecent, default threshold/2) are preserved.
func NewSummaryStore(threshold int, summarizer Summarizer, opts ...SummaryOption) *SummaryStore {
	if threshold < 4 {
		threshold = 4
	}
	s := &SummaryStore{
		threshold:  threshold,
		keepRecent: threshold / 2,
		summarizer: summarizer,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.keepRecent >= s.threshold {
		s.keepRecent = s.threshold / 2
	}
	return s
}

// Add appends a message and triggers summarization if the threshold is exceeded.
func (s *SummaryStore) Add(ctx context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messages = append(s.messages, msg)

	if len(s.messages) > s.threshold {
		if err := s.compress(ctx); err != nil {
			return fmt.Errorf("memory summary: %w", err)
		}
	}
	return nil
}

// Messages returns a copy of the current messages, with any accumulated summary
// prepended as a system message.
func (s *SummaryStore) Messages(_ context.Context) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Message
	if s.summary != "" {
		out = append(out, Message{
			Role:    "system",
			Content: "Previous conversation summary:\n" + s.summary,
		})
	}
	out = append(out, s.messages...)
	return out, nil
}

// Clear removes all messages and the accumulated summary.
func (s *SummaryStore) Clear(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = s.messages[:0]
	s.summary = ""
	return nil
}

// compress summarizes older messages and keeps only the most recent ones.
// Must be called with mu held. Note: the summarizer call happens while holding
// the lock — this is acceptable because summarization is infrequent and callers
// should not depend on high-frequency concurrent Add calls.
func (s *SummaryStore) compress(ctx context.Context) error {
	if len(s.messages) <= s.keepRecent {
		return nil
	}

	// Split into old (to summarize) and recent (to keep).
	cutoff := len(s.messages) - s.keepRecent
	old := s.messages[:cutoff]

	// Build input for summarizer: include prior summary if exists.
	toSummarize := old
	if s.summary != "" {
		toSummarize = append([]Message{{
			Role:    "system",
			Content: "Previous summary: " + s.summary,
		}}, old...)
	}

	newSummary, err := s.summarizer(ctx, toSummarize)
	if err != nil {
		return err
	}

	s.summary = strings.TrimSpace(newSummary)
	s.messages = append([]Message(nil), s.messages[cutoff:]...)
	return nil
}
