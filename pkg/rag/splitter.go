package rag

import "strings"

// Splitter divides text into smaller chunks for embedding and retrieval.
type Splitter interface {
	Split(text string) []string
}

// TokenSplitter splits text into chunks of approximately maxTokens tokens,
// with an optional overlap between chunks. Tokens are approximated by words.
type TokenSplitter struct {
	maxTokens int
	overlap   int
}

// NewTokenSplitter creates a splitter that produces chunks of up to maxTokens
// words with the given overlap.
func NewTokenSplitter(maxTokens, overlap int) *TokenSplitter {
	if maxTokens < 1 {
		maxTokens = 500
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxTokens {
		overlap = maxTokens / 4
	}
	return &TokenSplitter{maxTokens: maxTokens, overlap: overlap}
}

// Split divides text into word-count based chunks.
func (s *TokenSplitter) Split(text string) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	if len(words) <= s.maxTokens {
		return []string{text}
	}

	var chunks []string
	step := s.maxTokens - s.overlap
	if step < 1 {
		step = 1
	}

	for i := 0; i < len(words); i += step {
		end := i + s.maxTokens
		if end > len(words) {
			end = len(words)
		}
		chunk := strings.Join(words[i:end], " ")
		chunks = append(chunks, chunk)
		if end == len(words) {
			break
		}
	}
	return chunks
}

// ParagraphSplitter splits text by double newlines (paragraphs).
// Paragraphs shorter than minLength are merged with the next paragraph.
type ParagraphSplitter struct {
	minLength int
}

// NewParagraphSplitter creates a paragraph splitter.
// minLength: minimum character length for a chunk (shorter paragraphs are merged).
func NewParagraphSplitter(minLength int) *ParagraphSplitter {
	if minLength < 0 {
		minLength = 0
	}
	return &ParagraphSplitter{minLength: minLength}
}

// Split divides text by paragraph boundaries.
func (s *ParagraphSplitter) Split(text string) []string {
	// Split on double newlines.
	rawParagraphs := strings.Split(text, "\n\n")
	var paragraphs []string
	for _, p := range rawParagraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			paragraphs = append(paragraphs, trimmed)
		}
	}

	if len(paragraphs) == 0 {
		return nil
	}
	if s.minLength == 0 {
		return paragraphs
	}

	// Merge short paragraphs.
	var chunks []string
	current := paragraphs[0]
	for i := 1; i < len(paragraphs); i++ {
		if len(current) < s.minLength {
			current = current + "\n\n" + paragraphs[i]
		} else {
			chunks = append(chunks, current)
			current = paragraphs[i]
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}
