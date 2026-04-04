# Waggle Architecture

> A deep dive into the architecture, design principles, and internals of the Waggle AI Agent Orchestration Engine.

---

## Table of Contents

- [1. Overview](#1-overview)
- [2. Design Philosophy](#2-design-philosophy)
- [3. Project Layout](#3-project-layout)
- [4. Core Layer — `pkg/agent`](#4-core-layer--pkgagent)
  - [4.1 Agent Interface](#41-agent-interface)
  - [4.2 Type Erasure Bridge](#42-type-erasure-bridge)
  - [4.3 Chain — Serial Pipeline](#43-chain--serial-pipeline)
  - [4.4 Decorators](#44-decorators)
  - [4.5 Error Types](#45-error-types)
- [5. Orchestration Layer — `pkg/waggle`](#5-orchestration-layer--pkgwaggle)
  - [5.1 DAG Data Structure](#51-dag-data-structure)
  - [5.2 Waggle Orchestrator](#52-waggle-orchestrator)
  - [5.3 Concurrent DAG Executor](#53-concurrent-dag-executor)
  - [5.4 Orchestration Patterns](#54-orchestration-patterns)
- [6. LLM Integration Layer — `pkg/llm`](#6-llm-integration-layer--pkgllm)
  - [6.1 Provider Interface](#61-provider-interface)
  - [6.2 Provider Implementations](#62-provider-implementations)
  - [6.3 Intelligent Router](#63-intelligent-router)
  - [6.4 LLM Agent](#64-llm-agent)
  - [6.5 Tool Agent (ReAct Loop)](#65-tool-agent-react-loop)
- [7. Observability Layer — `pkg/observe`](#7-observability-layer--pkgobserve)
  - [7.1 Event System](#71-event-system)
  - [7.2 Distributed Tracing](#72-distributed-tracing)
  - [7.3 Metrics Collection](#73-metrics-collection)
  - [7.4 Structured Logging](#74-structured-logging)
- [8. Web Visualization — `pkg/web` + `web/`](#8-web-visualization--pkgweb--web)
  - [8.1 Embedded HTTP Server](#81-embedded-http-server)
  - [8.2 REST API](#82-rest-api)
  - [8.3 SSE Real-time Events](#83-sse-real-time-events)
  - [8.4 Frontend (D3.js)](#84-frontend-d3js)
- [9. CLI — `cmd/waggle`](#9-cli--cmdwaggle)
  - [9.1 YAML Workflow Definition](#91-yaml-workflow-definition)
  - [9.2 Commands](#92-commands)
- [10. Data Flow Architecture](#10-data-flow-architecture)
- [11. Concurrency Model](#11-concurrency-model)
- [12. Dependency Strategy](#12-dependency-strategy)

---

## 1. Overview

**Waggle** is a lightweight, embeddable AI Agent orchestration engine for Go 1.26+. Named after the honeybee *waggle dance* — the way bees communicate navigation information to their hive — Waggle treats every Agent as a bee, goroutines as wings, and channels as the colony's communication dance.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Application                              │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  pkg/agent (Core)                        │   │
│   │  Agent[I,O] · Func · Chain2-5 · Retry · Timeout · Cache │   │
│   └────────────────────────────┬────────────────────────────┘   │
│                                │                                 │
│   ┌────────────────────────────┴────────────────────────────┐   │
│   │              pkg/waggle (Orchestration)                   │   │
│   │  DAG · Executor · Parallel · Race · Vote · Router · Loop │   │
│   └────────────────────────────┬────────────────────────────┘   │
│                                │                                 │
│   ┌─────────────────┬──────────┴───────┬────────────────────┐   │
│   │   pkg/llm       │  pkg/observe     │   pkg/web          │   │
│   │ OpenAI/Claude   │ Events/Traces    │ Dashboard/SSE      │   │
│   │ Ollama/Router   │ Metrics/Logger   │ D3.js Visualizer   │   │
│   │ LLMAgent/Tool   │                  │                    │   │
│   └─────────────────┴──────────────────┴────────────────────┘   │
│                                                                  │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │             cmd/waggle (CLI)                             │   │
│   │  run · serve · validate · dot · version                  │   │
│   └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. Design Philosophy

### 2.1 Compile-Time Type Safety

The `Agent[I, O any]` interface uses Go generics to enforce type matching at compile time. When you chain agents, the Go compiler verifies that the output type of one agent matches the input type of the next:

```go
fetcher   := agent.Func[string, []byte]("fetcher", fetchURL)
parser    := agent.Func[[]byte, Document]("parser", parseHTML)
pipeline  := agent.Chain2(fetcher, parser) // Compiler enforces []byte matches
```

A `Chain2[string, int, Document]` would fail at compile time — not at runtime.

### 2.2 Zero External Dependencies (Core)

The core packages (`pkg/agent`, `pkg/waggle`, `pkg/observe`, `pkg/web`) depend only on the Go standard library: `context`, `sync`, `net/http`, `embed`, `log/slog`, `encoding/json`. The only external dependency is `gopkg.in/yaml.v3`, used exclusively by the CLI for YAML workflow parsing.

### 2.3 Goroutine-per-Agent Concurrency

Unlike Python frameworks constrained by the GIL, Waggle leverages Go's native concurrency. Each Agent in a DAG runs in its own goroutine, communicating through typed channels. This enables true parallel execution across CPU cores.

### 2.4 Functional Options Pattern

All configurable components use the functional options pattern for clean, extensible APIs:

```go
reliable := agent.WithRetry(myAgent,
    agent.WithMaxAttempts(5),
    agent.WithBaseDelay(200*time.Millisecond),
)
```

### 2.5 Decorator Composability

Decorators (`WithRetry`, `WithTimeout`, `WithCache`) wrap any `Agent[I,O]` and return a new `Agent[I,O]`, enabling arbitrary nesting:

```go
pipeline := agent.WithRetry(
    agent.WithTimeout(
        agent.WithCache(myAgent, keyFunc),
        5*time.Second,
    ),
    agent.WithMaxAttempts(3),
)
```

---

## 3. Project Layout

```
waggle/
├── pkg/
│   ├── agent/          # Layer 0: Core Agent interface + primitives
│   │   ├── agent.go        # Agent[I,O], UntypedAgent, Erase()
│   │   ├── func_agent.go   # Func() — create Agent from function
│   │   ├── chain.go        # Chain2 through Chain5
│   │   ├── retry.go        # WithRetry (exponential backoff + jitter)
│   │   ├── timeout.go      # WithTimeout (deadline enforcement)
│   │   ├── cache.go        # WithCache (memoization)
│   │   └── errors.go       # ErrTypeMismatch, RetryExhaustedError, TimeoutError
│   │
│   ├── waggle/         # Layer 1: DAG orchestration engine
│   │   ├── dag.go          # DAG data structure + algorithms
│   │   ├── waggle.go       # Waggle orchestrator
│   │   ├── executor.go     # Concurrent DAG executor
│   │   └── patterns.go     # Parallel, Race, Vote, Router, Loop
│   │
│   ├── llm/            # Layer 2: LLM provider integration
│   │   ├── provider.go     # Provider interface + types
│   │   ├── openai.go       # OpenAI Chat Completions
│   │   ├── anthropic.go    # Anthropic Messages API
│   │   ├── ollama.go       # Ollama local inference
│   │   ├── router.go       # Multi-provider router
│   │   ├── llm_agent.go    # LLMAgent builder
│   │   └── tool_agent.go   # ToolAgent (ReAct function calling)
│   │
│   ├── observe/        # Layer 2: Observability
│   │   ├── event.go        # Event types + factory functions
│   │   ├── tracer.go       # Span-based tracing (OTel-compatible)
│   │   ├── metrics.go      # Aggregated metrics collector
│   │   └── logger.go       # Structured logging (slog wrapper)
│   │
│   └── web/            # Layer 2: Visualization
│       ├── server.go       # HTTP server (go:embed)
│       ├── api.go          # REST API handlers
│       ├── sse.go          # Server-Sent Events hub
│       └── static/         # Embedded frontend
│
├── web/                # Frontend source (mirrored to pkg/web/static/)
│   ├── index.html          # Single-page app
│   ├── app.js              # D3.js force-directed graph
│   └── style.css           # Dark theme CSS
│
├── cmd/waggle/         # CLI binary
│   ├── main.go             # Entry point + command dispatch
│   └── commands/
│       ├── workflow.go     # YAML workflow parser + validator
│       ├── run.go          # run / validate / dot commands
│       └── serve.go        # serve / version commands
│
└── examples/           # Real-world examples
    ├── code_review/        # Chain4 + Cache + Retry pipeline
    ├── research/           # Parallel + Race + Chain2
    └── customer_support/   # Router + Loop + Chain2
```

---

## 4. Core Layer — `pkg/agent`

### 4.1 Agent Interface

The `Agent[I, O any]` interface is the fundamental building block:

```go
type Agent[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
}
```

**Design decisions:**

- **Two type parameters** (`I` for input, `O` for output) provide full pipeline type safety.
- **`context.Context`** as the first parameter enables cancellation, timeout, and value propagation.
- **`Name()`** is used for logging, tracing, error messages, and DAG node identification.

The simplest way to create an Agent is via `Func()`:

```go
func Func[I, O any](name string, fn func(ctx context.Context, input I) (O, error)) Agent[I, O]
```

### 4.2 Type Erasure Bridge

For dynamic scenarios (YAML workflows, DAG execution), types are not known at compile time. Waggle provides a type erasure mechanism:

```
Agent[I, O]  ──Erase()──>  UntypedAgent
     ▲                          │
     │                          │ RunUntyped(ctx, any) (any, error)
  compile-time                  │
  type safety              runtime type
                           assertion (I)
```

```go
type UntypedAgent interface {
    Name() string
    RunUntyped(ctx context.Context, input any) (any, error)
}

func Erase[I, O any](a Agent[I, O]) UntypedAgent
```

`Erase()` wraps a typed Agent. At runtime, `RunUntyped` performs a type assertion on the input. If the assertion fails, it returns `*ErrTypeMismatch` with the agent name and the received type.

### 4.3 Chain — Serial Pipeline

Chain functions compose agents into type-safe serial pipelines:

```
Chain2: Agent[A,B] + Agent[B,C] => Agent[A,C]
Chain3: Agent[A,B] + Agent[B,C] + Agent[C,D] => Agent[A,D]
Chain4: Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] => Agent[A,E]
Chain5: Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] + Agent[E,F] => Agent[A,F]
```

**Implementation:** Chain3-5 are built recursively from Chain2:
```go
func Chain3[A, B, C, D any](a Agent[A, B], b Agent[B, C], c Agent[C, D]) Agent[A, D] {
    return Chain2(Chain2(a, b), c)
}
```

**Error handling:** If any stage returns an error, execution short-circuits immediately. The error is wrapped with the failing stage's name: `chain stage "fetch": connection refused`.

**Context checking:** Between each stage, `ctx.Err()` is checked to enable fast cancellation without entering the next stage.

### 4.4 Decorators

#### WithRetry — Exponential Backoff

```
Attempt 1 → fail → sleep(baseDelay × 2⁰ × jitter)
Attempt 2 → fail → sleep(baseDelay × 2¹ × jitter)
Attempt 3 → fail → RetryExhaustedError{Attempts: 3, LastErr: ...}
```

- **Backoff formula:** `min(baseDelay × 2^attempt, maxDelay)`
- **Jitter:** Random factor in `[0.5, 1.5)` to prevent thundering herd
- **Context-aware:** Checks `ctx.Err()` before each retry and during sleep (`select` with `ctx.Done()`)
- **Defaults:** 3 attempts, 100ms base delay, 30s max delay, jitter enabled

#### WithTimeout — Deadline Enforcement

Wraps each `Run()` call with `context.WithTimeout`. If the parent context already has a shorter deadline, the shorter one takes precedence. Returns `*TimeoutError` (which `Unwrap()`s to `context.DeadlineExceeded`).

#### WithCache — Memoization

Uses `sync.Map` for lock-free concurrent reads. The `keyFunc` maps input to a string cache key. **Both results and errors are cached** — to avoid caching errors, compose `WithRetry` before `WithCache`:

```go
cached := agent.WithCache(
    agent.WithRetry(flaky, agent.WithMaxAttempts(3)),
    keyFunc,
)
```

### 4.5 Error Types

| Error | When | Unwrap |
|-------|------|--------|
| `*ErrTypeMismatch` | `UntypedAgent` receives wrong type | — |
| `*RetryExhaustedError` | All retry attempts failed | `LastErr` |
| `*TimeoutError` | Execution exceeded deadline | `context.DeadlineExceeded` |

All error types implement `Error() string` and (where applicable) `Unwrap() error`, supporting `errors.Is()` and `errors.As()`.

---

## 5. Orchestration Layer — `pkg/waggle`

### 5.1 DAG Data Structure

The DAG is the backbone of workflow execution:

```go
type DAG struct {
    nodes     map[string]*node        // id → node
    adjacency map[string][]string     // id → successor ids (outgoing edges)
    reverse   map[string][]string     // id → predecessor ids (incoming edges)
    edges     []edge                  // all edges
}
```

**Key algorithms:**

| Operation | Algorithm | Complexity |
|-----------|-----------|------------|
| `addEdge(from, to)` | Iterative DFS cycle detection | O(V + E) per edge |
| `TopologicalSort()` | Kahn's algorithm (BFS) | O(V + E) |
| `Layers()` | BFS level-order traversal | O(V + E) |
| `CriticalPath()` | DP on topological order | O(V + E) |

**Cycle detection:** Every `addEdge()` call performs a real-time check: "Does a path exist from `to` back to `from`?" If yes, the edge is rejected with `ErrCycleDetected`. This prevents cycles incrementally rather than deferring to sort time.

**Critical path analysis:** Uses dynamic programming on the topological order. For each node, `dist[id] = max(dist[pred] + weight[id])`. The critical path determines the theoretical minimum execution time.

**Layers:** Groups nodes by BFS level. Layer 0 = source nodes. All nodes in the same layer can execute in parallel:

```
Layer 0: [A, B]      ← no predecessors, start immediately
Layer 1: [C, D]      ← depends on Layer 0
Layer 2: [E]          ← depends on Layer 1
```

### 5.2 Waggle Orchestrator

The `Waggle` struct is the high-level API for building and running workflows:

```go
w := waggle.New()
w.Register(agent.Erase(fetcher), agent.Erase(parser))
w.Connect("fetcher", "parser")
result, err := w.RunFrom(ctx, "fetcher", "https://example.com")
```

`Register()` adds UntypedAgents as DAG nodes. `Connect()` declares data-flow edges with built-in cycle detection. `RunFrom()` executes the pipeline starting from a given node using topological ordering.

`DAGInfo()` returns a read-only `DAGSnapshot` for visualization, containing node IDs, names, predecessors, and successors.

### 5.3 Concurrent DAG Executor

The executor implements a goroutine-per-node execution model:

```
Source Nodes        Middle Nodes         Sink Node
┌───────┐         ┌───────┐           ┌───────┐
│Agent A│──ch──>  │Agent C│──ch──>    │Agent E│──> result
└───────┘    ╲    └───────┘           └───────┘
              ╲                          ╱
┌───────┐      ╲  ┌───────┐           ╱
│Agent B│──ch──> ─│Agent D│──ch──>  ─╱
└───────┘         └───────┘
```

**Execution flow:**

1. **Compute topological order** via Kahn's algorithm
2. **Allocate trigger channels** — one per node (buffered, size 1)
3. **Source nodes** get an immediate trigger signal
4. **Launch goroutines** — one per node, each waits on its trigger channel
5. When a goroutine completes:
   - Store output in `map[string]any` (mutex-protected)
   - Increment ready count for each successor
   - If successor's ready count == predecessor count, signal its trigger
6. **Fan-in:** Nodes with multiple predecessors receive `[]any` (aggregated outputs)
7. **Fan-out:** One node's output is forwarded to all successors
8. **Error propagation:** First error cancels the entire context; all goroutines exit

### 5.4 Orchestration Patterns

All patterns return `Agent[I, O]`, so they compose with chains, decorators, and each other.

#### Parallel — Fan-out, Collect All

```
Input ─┬─> Agent1 ─┐
       ├─> Agent2 ─┤──> ParallelResults[O]{Results, Errors}
       └─> Agent3 ─┘
```

All agents run concurrently with the same input. Results are ordered by agent index. Run itself never returns an error — partial failures are captured in `ParallelResults.Errors`.

#### Race — First Wins

```
Input ─┬─> Agent1 ──╮
       ├─> Agent2 ──┤──> first successful result
       └─> Agent3 ──╯    (others cancelled via ctx)
```

Useful for latency hedging: run the same query against multiple LLMs and take whichever responds first.

#### Vote — Consensus

```
Input ─┬─> Judge1 ─┐
       ├─> Judge2 ─┤──> VoteFunc(candidates) ──> winner
       └─> Judge3 ─┘
```

`MajorityVote[O]()` uses `fmt.Sprintf("%v", v)` for comparison. Requires >50% agreement.

#### Router — Conditional Branching

```
Input ──> routeFn(input) ──> "billing"  ──> billingAgent
                             "technical" ──> techAgent
                             "unknown"   ──> fallbackAgent
```

`WithFallback` provides a default branch for unrecognized keys.

#### Loop — Iterative Refinement

```
Input ──> initAgent ──> Output
              ▲            │
              │            ▼
              │       condition(output)?
              │        yes │    no
              │            │    └──> return output
              └────────────┘
         bodyAgent(output)
```

`initAgent` converts `I → O` (first pass). `bodyAgent` refines `O → O` on each iteration. Loop terminates when `condition` returns `false` or `maxIterations` is reached.

---

## 6. LLM Integration Layer — `pkg/llm`

### 6.1 Provider Interface

```go
type Provider interface {
    Info() ProviderInfo
    Chat(ctx context.Context, messages []Message) (string, error)
    ChatStream(ctx context.Context, messages []Message) (<-chan string, error)
}
```

**Design decision:** All implementations use `net/http` directly — no external SDKs. This keeps the dependency tree minimal and gives full control over request construction, error handling, and streaming.

### 6.2 Provider Implementations

| Provider | API | Model Default | Streaming | Cost | Context |
|----------|-----|---------------|-----------|------|---------|
| **OpenAI** | `/v1/chat/completions` | `gpt-4o` | SSE (`data: [DONE]`) | $0.005/1K | 128K |
| **Anthropic** | `/v1/messages` | `claude-3-5-sonnet` | SSE (`content_block_delta`) | $0.003/1K | 200K |
| **Ollama** | `/api/chat` | `llama3.2` | NDJSON (line-delimited) | Free | 8K |

**Anthropic specifics:** System messages are extracted and sent as the top-level `system` field (Anthropic API requirement), not in the messages array.

**Ollama Chat implementation:** `Chat()` is implemented by calling `ChatStream()` and collecting all tokens — avoiding code duplication.

### 6.3 Intelligent Router

The router wraps multiple Providers behind a single `Provider` interface with four strategies:

```
         ┌─> OpenAI    (cost: $0.005, latency: 800ms)
Request ─┤─> Anthropic (cost: $0.003, latency: 600ms)
         └─> Ollama    (cost: $0.000, latency: 2000ms)
```

| Strategy | Selection Logic |
|----------|----------------|
| `StrategyLowestCost` | Sort by `CostPer1KTokens`, use cheapest first |
| `StrategyLowestLatency` | Sort by `AvgLatencyMs`, use fastest first |
| `StrategyRoundRobin` | Rotate through providers sequentially |
| `StrategyFailover` | Try in order, fall back on error (default) |

`ChatStream` always uses failover strategy and skips providers where `SupportsStreaming` is false.

### 6.4 LLM Agent

`NewLLMAgent[I]` bridges the Provider interface with the Agent interface:

```
Input I ──> PromptFunc(ctx, input) ──> []Message ──> Provider.Chat() ──> string
```

`SimplePrompt[I]` is a convenience builder for the common case of a fixed system prompt + formatted user message.

### 6.5 Tool Agent (ReAct Loop)

The ToolAgent implements a ReAct-style reasoning loop for function calling:

```
                    ┌────────────────────────────┐
                    ▼                            │
User Input ──> LLM Call ──> Parse Response       │
                    │           │                │
                    │     ┌─────┴──────┐         │
                    │     │            │         │
                    │ tool_calls?  final_answer? │
                    │     │            │         │
                    │     ▼            ▼         │
                    │ Execute      Return        │
                    │ Tools        Result        │
                    │     │                      │
                    │     └──────────────────────┘
                    │     (append results to
                    │      conversation history)
```

**Protocol:** Tools are injected into the system prompt as a JSON schema. The LLM is instructed to respond with structured JSON:

- Tool call: `{"thought": "...", "tool_calls": [{"tool": "name", "args": "{...}"}]}`
- Final answer: `{"final_answer": "response text"}`
- Non-JSON responses are treated as direct final answers.

This design works with **any LLM** — it doesn't require native function-calling API support.

---

## 7. Observability Layer — `pkg/observe`

### 7.1 Event System

Events are the foundation of all observability. Six event types cover the full lifecycle:

```
workflow.start ──> agent.start ──> agent.end ──> data.flow ──> ... ──> workflow.end
                                   agent.error
```

Each `Event` carries: `Type`, `AgentName`, `WorkflowID`, `Timestamp`, `Duration`, `Error`, `InputSize`, `OutputSize`, and extensible `Metadata`.

Events flow through channels, enabling decoupled consumption:

```
Executor ──> chan Event ──┬──> Metrics.ConsumeEvents()
                          ├──> Logger.ConsumeEvents()
                          ├──> SSE Hub (web panel)
                          └──> Custom consumer
```

### 7.2 Distributed Tracing

The `Tracer` records `Span` objects compatible with OpenTelemetry concepts:

```go
type Span struct {
    TraceID, SpanID, ParentSpanID string
    Name                          string
    StartTime, EndTime            time.Time
    Status                        SpanStatus  // OK, Error, Running
    ErrorMessage                  string
    Attributes                    map[string]any
}
```

Spans are context-propagated via `WithTracer(ctx, tracer)` / `TracerFromContext(ctx)`. The `SpanExporter` interface allows exporting to Jaeger, Zipkin, or OTLP backends.

### 7.3 Metrics Collection

`Metrics` aggregates per-agent performance data with concurrent-safe access (`sync.RWMutex`):

```go
type AgentMetrics struct {
    AgentName        string
    TotalRuns        int64
    SuccessRuns      int64
    ErrorRuns        int64
    TotalDuration    time.Duration
    MinDuration      time.Duration
    MaxDuration      time.Duration
    TotalInputBytes  int64
    TotalOutputBytes int64
}
```

Derived metrics: `AvgDuration()` and `ErrorRate()`. Auto-updated via `ConsumeEvents()`.

### 7.4 Structured Logging

The `Logger` wraps `*slog.Logger` with workflow-aware convenience methods:

- `AgentStart`, `AgentEnd`, `AgentError`, `AgentRetry`
- `WorkflowStart`, `WorkflowEnd`
- Context injection: `WithLogger(ctx, l)` / `LoggerFromContext(ctx)`
- `ConsumeEvents()` for automatic event-to-log conversion

---

## 8. Web Visualization — `pkg/web` + `web/`

### 8.1 Embedded HTTP Server

The server uses `//go:embed static` to bundle the frontend into the binary:

```go
//go:embed static
var staticFiles embed.FS
```

Routes:
- `/` → Embedded `index.html`
- `/api/dag` → DAG structure (JSON)
- `/api/metrics` → Agent metrics (JSON)
- `/api/events` → SSE event stream
- `/health` → Health check

### 8.2 REST API

**GET /api/dag:**
```json
{
  "nodes": [{"id": "fetch", "name": "fetch", "status": "waiting", "predecessors": [], "successors": ["parse"]}],
  "edges": [{"from": "fetch", "to": "parse"}]
}
```

**GET /api/metrics:**
```json
{
  "agents": [{
    "agent_name": "fetch",
    "total_runs": 42,
    "error_rate": 0.05,
    "avg_duration_ms": 120
  }]
}
```

### 8.3 SSE Real-time Events

The SSE hub uses a register/remove/broadcast pattern:

```
                     ┌──> Client 1 (chan string)
Events ──> sseHub ──┤──> Client 2 (chan string)
                     └──> Client 3 (chan string)
```

- **Non-blocking broadcast:** Slow clients get messages dropped (no backpressure)
- **Connection lifecycle:** Initial `{"type":"connected"}` ping on connect
- **Hub started in `NewServer()`** so both `Start()` and `httptest.NewServer(Handler())` work correctly

### 8.4 Frontend (D3.js)

The single-page app renders a force-directed graph:

```
┌─────────────────────────────────────────────┐
│ 🐝 Waggle        Agent Orchestration Engine │
├─────────────────────────┬───────────────────┤
│                         │  Details          │
│   [fetch] ──> [parse]   │  Agent: fetch     │
│      │                  │  Status: success  │
│      v                  │  Runs: 42         │
│   [review]              │  Avg: 120ms       │
│                         │                   │
│   DAG Visualization     │  Event Log        │
│   (D3.js force-directed │  10:30 agent.start│
│    with drag/zoom)      │  10:30 agent.end  │
└─────────────────────────┴───────────────────┘
```

**Features:**
- **Force simulation** with collision avoidance, charge repulsion, and link constraints
- **Status-based coloring:** waiting (gray), running (blue with pulse animation), success (green), error (red)
- **Real-time updates** via SSE — node status changes animate instantly
- **Interactive:** Click nodes for detail panel, drag nodes to rearrange, scroll to zoom
- **Auto-refresh:** Metrics polled every 5 seconds

---

## 9. CLI — `cmd/waggle`

### 9.1 YAML Workflow Definition

```yaml
name: code-review
description: Automated code review pipeline
agents:
  - name: fetcher
    type: func
    description: Fetch PR content
  - name: reviewer
    type: llm
    model: gpt-4o
    provider: openai
    prompt: "Review the following code:"
    retry:
      max_attempts: 3
      base_delay_ms: 200
    timeout_secs: 30
flow:
  - from: fetcher
    to: reviewer
```

**Validation rules:**
- `name` is required
- All agents must have unique names
- All flow edges must reference registered agents
- Both `from` and `to` must be non-empty

### 9.2 Commands

| Command | Description |
|---------|-------------|
| `waggle run <workflow.yaml>` | Parse → build orchestrator → find source → execute |
| `waggle validate <file>...` | Parse and validate without execution |
| `waggle dot <workflow.yaml>` | Export Graphviz DOT format |
| `waggle serve [--addr :8080]` | Start web visualization panel |
| `waggle version` | Print version info |

`waggle run` uses `signal.NotifyContext(SIGINT, SIGTERM)` for graceful shutdown.

---

## 10. Data Flow Architecture

A complete data flow through the system:

```
User Code           Core              Orchestration        Observability      Visualization
─────────         ──────             ─────────────        ─────────────      ─────────────
                                                          
Create agents                                             
    │                                                     
    ▼                                                     
agent.Func()  ──> Agent[I,O]                              
    │                                                     
    ▼                                                     
agent.Erase() ──> UntypedAgent ──> waggle.Register()      
    │                                                     
    ▼                                                     
waggle.Connect() ────────────────> DAG.addEdge()          
                                   (cycle check)          
    │                                                     
    ▼                                                     
waggle.RunFrom() ──────────────> TopologicalSort()        
                                       │                   
                                       ▼                   
                                 goroutine per node ──> Event emission
                                       │                      │
                                       ▼                      ▼
                                 channel triggers      Metrics.Consume()
                                       │               Logger.Consume()
                                       ▼                      │
                                 output collection            ▼
                                       │               SSE Hub broadcast
                                       ▼                      │
                                 return result                ▼
                                                       D3.js live update
```

---

## 11. Concurrency Model

### Thread Safety Guarantees

| Component | Mechanism | Guarantee |
|-----------|-----------|-----------|
| `Executor` outputs | `sync.Mutex` | Safe concurrent writes to output map |
| `Executor` readyCount | `sync.Mutex` | Safe increment + threshold check |
| `Agent.WithCache` | `sync.Map` | Lock-free concurrent reads, safe writes |
| `observe.Metrics` | `sync.RWMutex` | Multiple readers, exclusive writer |
| `observe.Tracer` | `sync.Mutex` | Safe span recording |
| `sseHub` | Channel-based (single goroutine event loop) | No locks needed |

### Goroutine Lifecycle

```
Executor.Run()
    │
    ├── goroutine: node "A" (source, immediately triggered)
    ├── goroutine: node "B" (source, immediately triggered)
    ├── goroutine: node "C" (waits for A & B via trigger channel)
    └── goroutine: node "D" (waits for C)
    
    wg.Wait() blocks until all goroutines exit
    
    Error path: any error → cancel() → all goroutines check ctx.Err() → exit
```

---

## 12. Dependency Strategy

```
Core packages (pkg/agent, pkg/waggle, pkg/observe, pkg/web):
    └── Go standard library only
        ├── context, sync, time
        ├── net/http, encoding/json
        ├── log/slog
        ├── embed (for web static files)
        └── fmt, errors, strings, ...

CLI (cmd/waggle/commands):
    └── gopkg.in/yaml.v3 (YAML parsing — the only external dependency)
```

This means the **core engine can be embedded in any Go application with zero transitive dependencies**. The YAML dependency is isolated to the CLI, which is optional.

---

*Waggle v0.1.0 — Apache 2.0 License*
