package memory

import (
	"context"
	"testing"
)

func TestWindowStore_BasicWindow(t *testing.T) {
	ctx := context.Background()
	store := NewWindowStore(3)

	for i := 0; i < 5; i++ {
		store.Add(ctx, Message{Role: "user", Content: string(rune('a' + i))})
	}

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// Should keep the last 3: c, d, e
	if msgs[0].Content != "c" || msgs[1].Content != "d" || msgs[2].Content != "e" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}

func TestWindowStore_PinSystemMessage(t *testing.T) {
	ctx := context.Background()
	store := NewWindowStore(3) // default: pin system message

	store.Add(ctx, Message{Role: "system", Content: "You are helpful."})
	store.Add(ctx, Message{Role: "user", Content: "a"})
	store.Add(ctx, Message{Role: "assistant", Content: "b"})
	store.Add(ctx, Message{Role: "user", Content: "c"})
	store.Add(ctx, Message{Role: "assistant", Content: "d"})

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// System message should be pinned at position 0.
	if msgs[0].Role != "system" || msgs[0].Content != "You are helpful." {
		t.Errorf("system message not pinned: %+v", msgs[0])
	}
	// The last 2 non-system messages should follow.
	if msgs[1].Content != "c" || msgs[2].Content != "d" {
		t.Errorf("unexpected tail messages: %+v %+v", msgs[1], msgs[2])
	}
}

func TestWindowStore_NoPinSystemMessage(t *testing.T) {
	ctx := context.Background()
	store := NewWindowStore(2, WithPinSystemMessage(false))

	store.Add(ctx, Message{Role: "system", Content: "sys"})
	store.Add(ctx, Message{Role: "user", Content: "a"})
	store.Add(ctx, Message{Role: "assistant", Content: "b"})

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// Without pinning, system message should be evicted.
	if msgs[0].Content != "a" || msgs[1].Content != "b" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}

func TestWindowStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := NewWindowStore(5)
	store.Add(ctx, Message{Role: "user", Content: "hello"})
	store.Clear(ctx)

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(msgs))
	}
}

func TestWindowStore_MinSize(t *testing.T) {
	store := NewWindowStore(0) // should be clamped to 1
	ctx := context.Background()
	store.Add(ctx, Message{Role: "user", Content: "a"})
	store.Add(ctx, Message{Role: "user", Content: "b"})

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message with minSize=1, got %d", len(msgs))
	}
	if msgs[0].Content != "b" {
		t.Errorf("expected last message, got %+v", msgs[0])
	}
}
