package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/lucientong/waggle/pkg/llm"
)

// ---- helpers for ToolAgent tests ----

// toolCallJSON builds a JSON string mimicking the LLM's tool call response.
func toolCallJSON(thought string, calls []map[string]string) string {
	type tc struct {
		Tool string `json:"tool"`
		Args string `json:"args"`
	}
	resp := struct {
		Thought   string `json:"thought,omitempty"`
		ToolCalls []tc   `json:"tool_calls,omitempty"`
	}{Thought: thought}
	for _, c := range calls {
		resp.ToolCalls = append(resp.ToolCalls, tc{Tool: c["tool"], Args: c["args"]})
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// finalAnswerJSON builds the LLM's final_answer JSON response.
func finalAnswerJSON(answer string) string {
	data, _ := json.Marshal(map[string]string{"final_answer": answer})
	return string(data)
}

// newToolAgentMockServer returns a mock OpenAI server that replies with
// the given responses in sequence, one per Chat call.
func newToolAgentMockServer(t *testing.T, responses []string) *httptest.Server {
	t.Helper()
	var idx int64
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		i := atomic.AddInt64(&idx, 1) - 1
		if int(i) >= len(responses) {
			t.Errorf("unexpected extra call #%d", i)
			http.Error(w, "no more responses", http.StatusInternalServerError)
			return
		}
		resp := fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":%s}}]}`,
			jsonStringEscape(responses[i]))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp)) //nolint
	}))
}

// jsonStringEscape returns s as a JSON-escaped string (with surrounding quotes).
func jsonStringEscape(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

// ---- Tests ----

func TestToolAgent_DirectFinalAnswer(t *testing.T) {
	// LLM responds with a final_answer immediately (no tool calls).
	srv := newToolAgentMockServer(t, []string{
		finalAnswerJSON("The capital of France is Paris."),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	ta := llm.NewToolAgent("qa", provider, nil, nil)

	result, err := ta.Run(context.Background(), "What is the capital of France?")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result.FinalResponse != "The capital of France is Paris." {
		t.Errorf("FinalResponse = %q, want %q", result.FinalResponse, "The capital of France is Paris.")
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %d, want 0", len(result.ToolCalls))
	}
}

func TestToolAgent_PlainTextResponse(t *testing.T) {
	// LLM responds with plain text (not JSON) — should be treated as final answer.
	srv := newToolAgentMockServer(t, []string{
		"I don't need any tools for this. The answer is 42.",
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	ta := llm.NewToolAgent("simple", provider, nil, nil)

	result, err := ta.Run(context.Background(), "What is 6 * 7?")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result.FinalResponse != "I don't need any tools for this. The answer is 42." {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
}

func TestToolAgent_ToolCallThenFinalAnswer(t *testing.T) {
	// Turn 1: LLM requests a tool call.
	// Turn 2: LLM provides final answer after seeing tool result.
	srv := newToolAgentMockServer(t, []string{
		toolCallJSON("I need to look up the weather.", []map[string]string{
			{"tool": "get_weather", "args": `{"city":"Tokyo"}`},
		}),
		finalAnswerJSON("The weather in Tokyo is sunny, 25°C."),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	defs := []llm.ToolDefinition{
		{Name: "get_weather", Description: "Get current weather", Parameters: json.RawMessage(`{}`)},
	}
	tools := map[string]llm.ToolFunc{
		"get_weather": func(_ context.Context, args string) (string, error) {
			return "Sunny, 25°C", nil
		},
	}

	ta := llm.NewToolAgent("weather-bot", provider, defs, tools)

	result, err := ta.Run(context.Background(), "What's the weather in Tokyo?")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result.FinalResponse != "The weather in Tokyo is sunny, 25°C." {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("ToolCalls count = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ToolName != "get_weather" {
		t.Errorf("ToolCalls[0].ToolName = %q, want %q", result.ToolCalls[0].ToolName, "get_weather")
	}
	if len(result.ToolResults) != 1 || result.ToolResults[0].Content != "Sunny, 25°C" {
		t.Errorf("ToolResults[0].Content = %q, want %q", result.ToolResults[0].Content, "Sunny, 25°C")
	}
	if result.ToolResults[0].Error != nil {
		t.Errorf("ToolResults[0].Error = %v, want nil", result.ToolResults[0].Error)
	}
}

func TestToolAgent_ToolNotFound(t *testing.T) {
	// LLM requests a tool that doesn't exist, then provides final answer.
	srv := newToolAgentMockServer(t, []string{
		toolCallJSON("", []map[string]string{
			{"tool": "nonexistent_tool", "args": "{}"},
		}),
		finalAnswerJSON("Sorry, I couldn't use that tool."),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	ta := llm.NewToolAgent("test-not-found", provider, nil, nil)

	result, err := ta.Run(context.Background(), "Do something")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(result.ToolResults) != 1 {
		t.Fatalf("ToolResults count = %d, want 1", len(result.ToolResults))
	}
	if result.ToolResults[0].Error == nil {
		t.Error("expected error in ToolResults for unknown tool")
	}
}

func TestToolAgent_ToolError(t *testing.T) {
	// The tool function returns an error; the agent should still continue.
	srv := newToolAgentMockServer(t, []string{
		toolCallJSON("", []map[string]string{
			{"tool": "broken_tool", "args": "{}"},
		}),
		finalAnswerJSON("The tool failed, but I'll provide a fallback."),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	defs := []llm.ToolDefinition{
		{Name: "broken_tool", Description: "Always fails", Parameters: json.RawMessage(`{}`)},
	}
	tools := map[string]llm.ToolFunc{
		"broken_tool": func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("database connection timeout")
		},
	}

	ta := llm.NewToolAgent("err-bot", provider, defs, tools)

	result, err := ta.Run(context.Background(), "Do something")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result.ToolResults[0].Error == nil {
		t.Error("expected error in ToolResults")
	}
	if result.FinalResponse != "The tool failed, but I'll provide a fallback." {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
}

func TestToolAgent_MaxIterationsExhausted(t *testing.T) {
	// LLM always requests tool calls, never gives a final answer.
	alwaysCallTool := toolCallJSON("thinking...", []map[string]string{
		{"tool": "loop_tool", "args": "{}"},
	})
	srv := newToolAgentMockServer(t, []string{
		alwaysCallTool, alwaysCallTool, alwaysCallTool,
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	defs := []llm.ToolDefinition{
		{Name: "loop_tool", Description: "Just loops", Parameters: json.RawMessage(`{}`)},
	}
	tools := map[string]llm.ToolFunc{
		"loop_tool": func(_ context.Context, _ string) (string, error) {
			return "ok", nil
		},
	}

	ta := llm.NewToolAgent("loop-bot", provider, defs, tools,
		llm.WithToolAgentMaxIterations(3),
	)

	_, err := ta.Run(context.Background(), "keep looping")
	if err == nil {
		t.Fatal("Run() expected max iterations error, got nil")
	}
}

func TestToolAgent_ContextCancelled(t *testing.T) {
	srv := newToolAgentMockServer(t, []string{
		finalAnswerJSON("should not reach here"),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	ta := llm.NewToolAgent("cancel-bot", provider, nil, nil)

	_, err := ta.Run(ctx, "test")
	if err == nil {
		t.Fatal("Run() expected context cancelled error, got nil")
	}
}

func TestToolAgent_Name(t *testing.T) {
	ta := llm.NewToolAgent("my-agent", nil, nil, nil)
	if ta.Name() != "my-agent" {
		t.Errorf("Name() = %q, want %q", ta.Name(), "my-agent")
	}
}

func TestToolAgent_Options(t *testing.T) {
	// Verify WithToolAgentSystemPrompt and WithToolAgentMaxIterations compile and apply.
	srv := newToolAgentMockServer(t, []string{
		finalAnswerJSON("ok"),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	ta := llm.NewToolAgent("opts-bot", provider, nil, nil,
		llm.WithToolAgentSystemPrompt("You are a custom assistant."),
		llm.WithToolAgentMaxIterations(5),
	)

	result, err := ta.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if result.FinalResponse != "ok" {
		t.Errorf("FinalResponse = %q, want %q", result.FinalResponse, "ok")
	}
}

func TestToolAgent_MultipleToolCalls(t *testing.T) {
	// LLM requests two tools in one turn, then provides final answer.
	srv := newToolAgentMockServer(t, []string{
		toolCallJSON("I need both tools.", []map[string]string{
			{"tool": "search", "args": `{"q":"go generics"}`},
			{"tool": "calc", "args": `{"expr":"2+2"}`},
		}),
		finalAnswerJSON("Go generics are great and 2+2=4."),
	})
	defer srv.Close()

	provider := llm.NewOpenAI("key",
		llm.WithOpenAIBaseURL(srv.URL),
		llm.WithOpenAIHTTPClient(srv.Client()),
	)

	defs := []llm.ToolDefinition{
		{Name: "search", Description: "Search the web", Parameters: json.RawMessage(`{}`)},
		{Name: "calc", Description: "Calculator", Parameters: json.RawMessage(`{}`)},
	}
	tools := map[string]llm.ToolFunc{
		"search": func(_ context.Context, _ string) (string, error) {
			return "Go generics since 1.18", nil
		},
		"calc": func(_ context.Context, _ string) (string, error) {
			return "4", nil
		},
	}

	ta := llm.NewToolAgent("multi-bot", provider, defs, tools)

	result, err := ta.Run(context.Background(), "search and calculate")
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Errorf("ToolCalls count = %d, want 2", len(result.ToolCalls))
	}
	if len(result.ToolResults) != 2 {
		t.Errorf("ToolResults count = %d, want 2", len(result.ToolResults))
	}
	if result.FinalResponse != "Go generics are great and 2+2=4." {
		t.Errorf("FinalResponse = %q", result.FinalResponse)
	}
}
