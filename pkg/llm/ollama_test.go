package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lucientong/waggle/pkg/llm"
)

// newMockOllamaServer returns a mock Ollama server that returns streaming NDJSON chunks.
func newMockOllamaServer(t *testing.T, chunks []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}

		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			w.Write(data)         //nolint
			w.Write([]byte("\n")) //nolint
			flusher.Flush()
		}
	}))
}

func TestOllama_Chat_Success(t *testing.T) {
	chunks := []map[string]any{
		{"message": map[string]string{"role": "assistant", "content": "Hello "}, "done": false},
		{"message": map[string]string{"role": "assistant", "content": "world!"}, "done": false},
		{"message": map[string]string{"role": "assistant", "content": ""}, "done": true},
	}
	srv := newMockOllamaServer(t, chunks)
	defer srv.Close()

	provider := llm.NewOllama(
		llm.WithOllamaBaseURL(srv.URL),
		llm.WithOllamaHTTPClient(srv.Client()),
		llm.WithOllamaModel("test-model"),
	)

	result, err := provider.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "Hi"},
	})
	if err != nil {
		t.Fatalf("Chat() unexpected error: %v", err)
	}
	if result != "Hello world!" {
		t.Errorf("Chat() = %q, want %q", result, "Hello world!")
	}
}

func TestOllama_ChatStream_Success(t *testing.T) {
	chunks := []map[string]any{
		{"message": map[string]string{"role": "assistant", "content": "token1"}, "done": false},
		{"message": map[string]string{"role": "assistant", "content": "token2"}, "done": false},
		{"message": map[string]string{"role": "assistant", "content": ""}, "done": true},
	}
	srv := newMockOllamaServer(t, chunks)
	defer srv.Close()

	provider := llm.NewOllama(
		llm.WithOllamaBaseURL(srv.URL),
		llm.WithOllamaHTTPClient(srv.Client()),
	)

	ch, err := provider.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "stream test"},
	})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	var tokens []string
	for tok := range ch {
		tokens = append(tokens, tok)
	}

	if len(tokens) != 2 {
		t.Errorf("received %d tokens, want 2", len(tokens))
	}
	if tokens[0] != "token1" || tokens[1] != "token2" {
		t.Errorf("tokens = %v, want [token1 token2]", tokens)
	}
}

func TestOllama_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	provider := llm.NewOllama(
		llm.WithOllamaBaseURL(srv.URL),
		llm.WithOllamaHTTPClient(srv.Client()),
	)

	_, err := provider.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	if err == nil {
		t.Fatal("Chat() expected error for 404, got nil")
	}
}

func TestOllama_StreamError(t *testing.T) {
	// Server returns a chunk with an error field.
	chunks := []map[string]any{
		{"error": "model loading failed", "done": false},
	}
	srv := newMockOllamaServer(t, chunks)
	defer srv.Close()

	provider := llm.NewOllama(
		llm.WithOllamaBaseURL(srv.URL),
		llm.WithOllamaHTTPClient(srv.Client()),
	)

	ch, err := provider.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	})
	if err != nil {
		t.Fatalf("ChatStream() unexpected error: %v", err)
	}

	// Channel should close without delivering tokens.
	var tokens []string
	for tok := range ch {
		tokens = append(tokens, tok)
	}
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens on error, got %d", len(tokens))
	}
}

func TestOllama_ContextCancelled(t *testing.T) {
	// Long-running server that will be cancelled.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	provider := llm.NewOllama(
		llm.WithOllamaBaseURL(srv.URL),
		llm.WithOllamaHTTPClient(srv.Client()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: "hello"},
	})
	if err == nil {
		t.Fatal("Chat() expected context cancelled error, got nil")
	}
}

func TestOllama_Info(t *testing.T) {
	provider := llm.NewOllama(llm.WithOllamaModel("mistral"))
	info := provider.Info()

	if info.Name != "ollama" {
		t.Errorf("Info().Name = %q, want %q", info.Name, "ollama")
	}
	if info.Model != "mistral" {
		t.Errorf("Info().Model = %q, want %q", info.Model, "mistral")
	}
	if info.CostPer1KTokens != 0 {
		t.Errorf("CostPer1KTokens = %f, want 0 (local)", info.CostPer1KTokens)
	}
	if !info.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
}

func TestOllama_DefaultModel(t *testing.T) {
	provider := llm.NewOllama()
	info := provider.Info()
	if info.Model != "llama3.2" {
		t.Errorf("default model = %q, want %q", info.Model, "llama3.2")
	}
}
