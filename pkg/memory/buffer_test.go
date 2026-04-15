package memory

import (
	"context"
	"testing"
)

func TestBufferStore_AddAndMessages(t *testing.T) {
	ctx := context.Background()
	store := NewBufferStore()

	if msgs, _ := store.Messages(ctx); len(msgs) != 0 {
		t.Fatalf("expected empty, got %d messages", len(msgs))
	}

	store.Add(ctx, Message{Role: "user", Content: "hello"})
	store.Add(ctx, Message{Role: "assistant", Content: "hi"})

	msgs, err := store.Messages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Errorf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi" {
		t.Errorf("unexpected second message: %+v", msgs[1])
	}
}

func TestBufferStore_MessagesReturnsCopy(t *testing.T) {
	ctx := context.Background()
	store := NewBufferStore()
	store.Add(ctx, Message{Role: "user", Content: "hello"})

	msgs, _ := store.Messages(ctx)
	msgs[0].Content = "mutated"

	msgs2, _ := store.Messages(ctx)
	if msgs2[0].Content != "hello" {
		t.Error("Messages should return a copy, but original was mutated")
	}
}

func TestBufferStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := NewBufferStore()
	store.Add(ctx, Message{Role: "user", Content: "hello"})
	store.Clear(ctx)

	msgs, _ := store.Messages(ctx)
	if len(msgs) != 0 {
		t.Fatalf("expected empty after clear, got %d", len(msgs))
	}
}
