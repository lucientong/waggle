# Waggle

> A lightweight AI Agent orchestration engine for Go.

[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/lucientong/waggle)](https://goreportcard.com/report/github.com/lucientong/waggle)

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
└── examples/       # code_review / research / customer_support
```

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
