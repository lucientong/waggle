package rag

import (
	"testing"
)

func TestTokenSplitter_Basic(t *testing.T) {
	s := NewTokenSplitter(3, 0)
	chunks := s.Split("one two three four five six")
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "one two three" {
		t.Errorf("chunk 0: %q", chunks[0])
	}
	if chunks[1] != "four five six" {
		t.Errorf("chunk 1: %q", chunks[1])
	}
}

func TestTokenSplitter_WithOverlap(t *testing.T) {
	s := NewTokenSplitter(4, 2)
	chunks := s.Split("a b c d e f g h")
	// step = 4-2 = 2, so chunks start at 0, 2, 4, 6
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestTokenSplitter_ShortText(t *testing.T) {
	s := NewTokenSplitter(10, 0)
	chunks := s.Split("hello world")
	if len(chunks) != 1 || chunks[0] != "hello world" {
		t.Errorf("expected single chunk, got %v", chunks)
	}
}

func TestTokenSplitter_Empty(t *testing.T) {
	s := NewTokenSplitter(5, 0)
	chunks := s.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected no chunks, got %d", len(chunks))
	}
}

func TestParagraphSplitter_Basic(t *testing.T) {
	text := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
	s := NewParagraphSplitter(0)
	chunks := s.Split(text)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestParagraphSplitter_MergeShort(t *testing.T) {
	text := "This is the first paragraph with enough text.\n\nShort.\n\nThis is a longer paragraph that should stand on its own."
	s := NewParagraphSplitter(20) // min 20 chars
	chunks := s.Split(text)
	// First paragraph (46 chars) >= 20, so it stands alone.
	// "Short." (6) < 20, merged with the third paragraph.
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (short merged with next), got %d: %v", len(chunks), chunks)
	}
	if chunks[0] != "This is the first paragraph with enough text." {
		t.Errorf("chunk 0: %q", chunks[0])
	}
}

func TestParagraphSplitter_Empty(t *testing.T) {
	s := NewParagraphSplitter(0)
	chunks := s.Split("")
	if len(chunks) != 0 {
		t.Errorf("expected no chunks, got %d", len(chunks))
	}
}
