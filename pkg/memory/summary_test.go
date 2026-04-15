package memory

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockSummarizer returns a simple concatenation of message contents.
func mockSummarizer(_ context.Context, msgs []Message) (string, error) {
	var parts []string
	for _, m := range msgs {
		parts = append(parts, m.Content)
	}
	return "Summary: " + strings.Join(parts, ", "), nil
}

func failingSummarizer(_ context.Context, _ []Message) (string, error) {
	return "", fmt.Errorf("summarizer failed")
}

func TestSummaryStore_NoCompressionBelowThreshold(t *testing.T) {
	ctx := context.Background()
	store := NewSummaryStore(6, mockSummarizer)

	for i := 0; i < 5; i++ {
		store.Add(ctx, Message{Role: "user", Content: fmt.Sprintf("msg%d", i)})
	}

	msgs, _ := store.Messages(ctx)
	// No summary should be generated since we're below threshold.
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, "Summary") {
			t.Error("should not have summary yet")
		}
	}
}

func TestSummaryStore_CompressesAboveThreshold(t *testing.T) {
	ctx := context.Background()
	store := NewSummaryStore(4, mockSummarizer, WithKeepRecent(2))

	// Add 5 messages (exceeds threshold of 4).
	for i := 0; i < 5; i++ {
		store.Add(ctx, Message{Role: "user", Content: fmt.Sprintf("msg%d", i)})
	}

	msgs, _ := store.Messages(ctx)
	// Should have: 1 summary system message + 2 recent messages.
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (1 summary + 2 recent), got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || !strings.Contains(msgs[0].Content, "Summary") {
		t.Errorf("first message should be summary, got: %+v", msgs[0])
	}
	// Recent messages should be the last 2.
	if msgs[1].Content != "msg3" || msgs[2].Content != "msg4" {
		t.Errorf("unexpected recent messages: %+v %+v", msgs[1], msgs[2])
	}
}

func TestSummaryStore_SummarizerError(t *testing.T) {
	ctx := context.Background()
	store := NewSummaryStore(4, failingSummarizer, WithKeepRecent(2))

	for i := 0; i < 4; i++ {
		store.Add(ctx, Message{Role: "user", Content: fmt.Sprintf("msg%d", i)})
	}

	// This Add should trigger compression and fail.
	err := store.Add(ctx, Message{Role: "user", Content: "trigger"})
	if err == nil {
		t.Fatal("expected error from failing summarizer")
	}
	if !strings.Contains(err.Error(), "summarizer failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSummaryStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := NewSummaryStore(4, mockSummarizer, WithKeepRecent(2))

	// Trigger compression.
	for i := 0; i < 5; i++ {
		store.Add(ctx, Message{Role: "user", Content: fmt.Sprintf("msg%d", i)})
	}

	store.Clear(ctx)
	msgs, _ := store.Messages(ctx)
	if len(msgs) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(msgs))
	}
}

func TestSummaryStore_MinThreshold(t *testing.T) {
	store := NewSummaryStore(1, mockSummarizer) // should be clamped to 4
	if store.threshold != 4 {
		t.Errorf("expected threshold clamped to 4, got %d", store.threshold)
	}
}

func TestSummaryStore_KeepRecentClamp(t *testing.T) {
	store := NewSummaryStore(4, mockSummarizer, WithKeepRecent(10)) // exceeds threshold
	if store.keepRecent >= store.threshold {
		t.Errorf("keepRecent should be clamped below threshold, got %d", store.keepRecent)
	}
}
