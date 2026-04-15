package output

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lucientong/waggle/pkg/llm"
)

// mockProvider returns a fixed response for testing.
type mockProvider struct {
	responses []string
	callCount int
}

func (m *mockProvider) Info() llm.ProviderInfo {
	return llm.ProviderInfo{Name: "mock", Model: "test"}
}

func (m *mockProvider) Chat(_ context.Context, _ []llm.Message) (string, error) {
	if m.callCount >= len(m.responses) {
		return "", fmt.Errorf("no more responses")
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

func (m *mockProvider) ChatStream(_ context.Context, _ []llm.Message) (<-chan string, error) {
	return nil, fmt.Errorf("not implemented")
}

type ReviewResult struct {
	Score  int    `json:"score"`
	Review string `json:"review"`
}

func TestStructuredAgent_Success(t *testing.T) {
	provider := &mockProvider{
		responses: []string{`{"score": 8, "review": "Good code"}`},
	}

	reviewer := NewStructuredAgent[string, ReviewResult](
		"reviewer", provider,
		func(code string) string { return "Review: " + code },
	)

	result, err := reviewer.Run(context.Background(), "func main() {}")
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 8 || result.Review != "Good code" {
		t.Errorf("unexpected: %+v", result)
	}
}

func TestStructuredAgent_RetryOnParseFailure(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			"I think the score is 7", // invalid JSON
			`{"score": 7, "review": "Decent"}`,
		},
	}

	reviewer := NewStructuredAgent[string, ReviewResult](
		"reviewer", provider,
		func(code string) string { return "Review: " + code },
		WithMaxRetries(1),
	)

	result, err := reviewer.Run(context.Background(), "code")
	if err != nil {
		t.Fatal(err)
	}
	if result.Score != 7 {
		t.Errorf("expected score 7, got %d", result.Score)
	}
	if provider.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", provider.callCount)
	}
}

func TestStructuredAgent_AllRetriesFail(t *testing.T) {
	provider := &mockProvider{
		responses: []string{
			"not json 1",
			"not json 2",
		},
	}

	reviewer := NewStructuredAgent[string, ReviewResult](
		"reviewer", provider,
		func(code string) string { return "Review: " + code },
		WithMaxRetries(1),
	)

	_, err := reviewer.Run(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "parse failed after 2 attempts") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStructuredAgent_Name(t *testing.T) {
	provider := &mockProvider{responses: []string{`{}`}}
	a := NewStructuredAgent[string, struct{}]("test-agent", provider, func(s string) string { return s })
	if a.Name() != "test-agent" {
		t.Errorf("expected name test-agent, got %s", a.Name())
	}
}
