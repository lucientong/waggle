# Waggle

> A lightweight AI Agent orchestration engine for Go.

[![Go Reference](https://pkg.go.dev/badge/github.com/lucientong/waggle.svg)](https://pkg.go.dev/github.com/lucientong/waggle)
[![CI](https://github.com/lucientong/waggle/actions/workflows/ci.yml/badge.svg)](https://github.com/lucientong/waggle/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/lucientong/waggle)](https://goreportcard.com/report/github.com/lucientong/waggle)
[![codecov](https://codecov.io/gh/lucientong/waggle/branch/master/graph/badge.svg)](https://codecov.io/gh/lucientong/waggle)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

**Waggle** is named after the *waggle dance* — the way honeybees communicate the direction and distance of food sources to their hive. In Waggle, every Agent is a bee, goroutines are its wings, and channels are the colony's communication dance.

## Why Waggle?

Existing AI Agent orchestration frameworks (LangChain, CrewAI, AutoGen, LangGraph) are built in Python and share five core problems that Waggle solves:

| Problem | Python Frameworks | Waggle |
|---|---|---|
| **Performance** | GIL prevents true concurrency | Goroutine-per-agent, thousands of concurrent agents |
| **Type safety** | `dict/str/Any` everywhere, runtime errors | `Agent[I, O any]` generics, compile-time type checking |
| **Orchestration** | Linear or simple DAG only | 6 built-in patterns: Chain, Parallel, Race, Vote, Router, Loop |
| **Deployment** | Heavy Python env, Docker required | Embed as Go library, single binary, zero external deps |
| **Observability** | Black-box execution, poor debugging | Built-in DAG visualizer, OpenTelemetry traces, structured logs |

## Installation

```bash
go get github.com/lucientong/waggle
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "strings"

    "github.com/lucientong/waggle/pkg/agent"
)

func main() {
    ctx := context.Background()

    // Create agents from plain functions
    upper := agent.Func[string, string]("upper", func(ctx context.Context, s string) (string, error) {
        return strings.ToUpper(s), nil
    })

    exclaim := agent.Func[string, string]("exclaim", func(ctx context.Context, s string) (string, error) {
        return s + "!", nil
    })

    // Chain them together: upper -> exclaim
    pipeline := agent.Chain2(upper, exclaim)

    result, err := pipeline.Run(ctx, "hello waggle")
    if err != nil {
        panic(err)
    }
    fmt.Println(result) // Output: HELLO WAGGLE!
}
```

## Core Concepts

### Agent

The minimal execution unit with compile-time type safety:

```go
type Agent[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
}
```

### FuncAgent

Create an Agent from any function:

```go
fetcher := agent.Func[string, []byte]("fetcher", func(ctx context.Context, url string) ([]byte, error) {
    // fetch URL content
})

summarizer := agent.Func[[]byte, string]("summarizer", func(ctx context.Context, data []byte) (string, error) {
    // summarize content with LLM
})
```

### Chain — Serial Pipeline

Connect agents where each output feeds the next input:

```go
// Chain2: Agent[A,B] + Agent[B,C] => Agent[A,C]
pipeline := agent.Chain2(fetcher, summarizer)

// Chain3: three agents
pipeline := agent.Chain3(fetcher, summarizer, reviewer)

// Chain4, Chain5 also available
```

### Decorators — Wrappers

Enhance any agent with cross-cutting concerns:

```go
// Retry with exponential backoff + jitter
reliable := agent.WithRetry(myAgent,
    agent.WithMaxAttempts(3),
    agent.WithBaseDelay(100*time.Millisecond),
)

// Timeout
bounded := agent.WithTimeout(myAgent, 5*time.Second)

// Cache (memoize identical inputs)
cached := agent.WithCache(myAgent, func(input string) string {
    return input // use input as cache key
})

// Compose decorators
pipeline := agent.Chain2(
    agent.WithRetry(agent.WithTimeout(fetcher, 3*time.Second), agent.WithMaxAttempts(3)),
    agent.WithCache(summarizer, func(data []byte) string { return string(data) }),
)
```

### Waggle — Orchestrator (Phase 2)

```go
w := waggle.New()
w.Chain(fetcher, summarizer, reviewer)
result, err := w.Run(ctx, "https://example.com")
```

### Orchestration Patterns (Phase 3)

```go
// Parallel: fan-out to all, collect all results
results := waggle.Parallel(agent1, agent2, agent3)

// Race: first to finish wins
fastest := waggle.Race(primaryAgent, backupAgent)

// Vote: majority consensus
decision := waggle.Vote(judge1, judge2, judge3)

// Router: conditional branching
routed := waggle.Router(classifyFn, map[string]Agent{
    "code":    codeReviewer,
    "docs":    docReviewer,
    "default": generalReviewer,
})

// Loop: repeat until condition met
refined := waggle.Loop(improveAgent, func(result string) bool {
    return qualityScore(result) >= 0.9
})
```

### LLM Integration

Waggle provides built-in LLM providers and a smart router for multi-model orchestration:

```go
import "github.com/lucientong/waggle/pkg/llm"

// OpenAI
openai := llm.NewOpenAI("sk-...",
    llm.WithOpenAIModel("gpt-4o"),
)

// Anthropic Claude
claude := llm.NewAnthropic("sk-ant-...",
    llm.WithAnthropicModel("claude-3-5-sonnet-20241022"),
)

// Ollama (local models)
local := llm.NewOllama(
    llm.WithOllamaModel("llama3"),
)
```

#### OpenAI-Compatible Providers

Since many providers offer OpenAI-compatible APIs, you can connect to them by simply overriding the `baseURL`:

```go
// Google Gemini (OpenAI-compatible endpoint)
gemini := llm.NewOpenAI("YOUR_GEMINI_API_KEY",
    llm.WithOpenAIBaseURL("https://generativelanguage.googleapis.com/v1beta/openai"),
    llm.WithOpenAIModel("gemini-2.0-flash"),
)

// Azure OpenAI
azure := llm.NewOpenAI("YOUR_AZURE_API_KEY",
    llm.WithOpenAIBaseURL("https://YOUR_RESOURCE.openai.azure.com/openai/deployments/YOUR_DEPLOYMENT"),
    llm.WithOpenAIModel("gpt-4o"),
)

// DeepSeek
deepseek := llm.NewOpenAI("YOUR_DEEPSEEK_API_KEY",
    llm.WithOpenAIBaseURL("https://api.deepseek.com/v1"),
    llm.WithOpenAIModel("deepseek-chat"),
)

// Groq
groq := llm.NewOpenAI("YOUR_GROQ_API_KEY",
    llm.WithOpenAIBaseURL("https://api.groq.com/openai/v1"),
    llm.WithOpenAIModel("llama-3.3-70b-versatile"),
)

// Mistral
mistral := llm.NewOpenAI("YOUR_MISTRAL_API_KEY",
    llm.WithOpenAIBaseURL("https://api.mistral.ai/v1"),
    llm.WithOpenAIModel("mistral-large-latest"),
)
```

#### LLM Agent

Turn any LLM provider into a type-safe Agent:

```go
summarizer := llm.NewLLMAgent[string]("summarizer", openai,
    func(ctx context.Context, text string) ([]llm.Message, error) {
        return []llm.Message{
            {Role: llm.RoleSystem, Content: "You are a concise summarizer."},
            {Role: llm.RoleUser, Content: "Summarize: " + text},
        }, nil
    },
)

// Or use the SimplePrompt helper:
translator := llm.NewLLMAgent("translator", claude,
    llm.SimplePrompt[string]("Translate to English.", func(s string) string { return s }),
)

result, _ := summarizer.Run(ctx, "Long article text...")
```

#### Tool Agent (Function Calling)

Build ReAct-loop agents that can invoke tools:

```go
agent := llm.NewToolAgent("assistant", openai,
    "You are a helpful assistant.",
    []llm.ToolDefinition{
        {
            Name:        "search",
            Description: "Search the web for information",
            Parameters:  `{"type":"object","properties":{"query":{"type":"string"}}}`,
        },
    },
    func(ctx context.Context, name string, args string) (string, error) {
        // Execute tool and return result
        return searchWeb(args), nil
    },
)

result, _ := agent.Run(ctx, "What is the weather in Tokyo?")
```

#### Smart Router

Route requests across multiple providers with built-in strategies:

```go
router := llm.NewRouter(
    llm.StrategyLowestCost,  // or: StrategyLowestLatency, StrategyRoundRobin, StrategyFailover
    openai, claude, local,
)

// Use the router as a regular provider — it selects the best backend automatically
result, _ := router.Chat(ctx, messages)
```

## Project Structure

```
waggle/
├── pkg/
│   ├── agent/      # Agent interface, FuncAgent, Chain, wrappers (Retry/Timeout/Cache)
│   ├── waggle/     # Core orchestrator, DAG, executor, patterns
│   ├── llm/        # LLM providers (OpenAI, Anthropic, Ollama) + LLM/Tool agents
│   ├── observe/    # Events, tracing, metrics, structured logging
│   └── web/        # Embedded web visualization panel
├── cmd/waggle/     # CLI: run / serve / validate / dot
├── web/            # Frontend source (D3.js DAG visualization)
└── examples/       # code_review / research / customer_support / proactive_agent
```

## Examples

| Example | Patterns Used | Description |
|---------|--------------|-------------|
| [Code Review](examples/code_review/) | Chain, WithRetry, WithCache | Multi-stage code review pipeline |
| [Customer Support](examples/customer_support/) | Router, Loop, WithRetry | Intelligent ticket routing & response refinement |
| [Research Assistant](examples/research/) | Parallel, Race, Chain | Concurrent multi-source research synthesis |
| [Proactive Agent](examples/proactive_agent/) | Router, Chain, WithTimeout | Layered timer-driven proactive messaging |

## Development Phases

- [x] **Phase 1** — Agent interface, FuncAgent, Chain, Retry/Timeout/Cache wrappers
- [x] **Phase 2** — DAG orchestrator, topological executor
- [x] **Phase 3** — Parallel / Race / Vote / Router / Loop patterns
- [x] **Phase 4** — LLM providers (OpenAI, Anthropic, Ollama) + LLM/Tool agents
- [x] **Phase 5** — Observability (events, tracing, metrics, slog)
- [x] **Phase 6** — Embedded web DAG visualization panel
- [x] **Phase 7** — CLI (`waggle run / serve / validate / dot`)
- [x] **Phase 8** — Real-world examples (code review, research, customer support)

## Requirements

- Go 1.26+

## License

Apache 2.0 — see [LICENSE](LICENSE)
