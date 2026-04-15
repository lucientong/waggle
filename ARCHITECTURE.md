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
- [13. Memory Layer — `pkg/memory`](#13-memory-layer--pkgmemory)
- [14. Structured Output — `pkg/output`](#14-structured-output--pkgoutput)
- [15. Prompt Templates — `pkg/prompt`](#15-prompt-templates--pkgprompt)
- [16. Observable Pipelines — `pkg/stream`](#16-observable-pipelines--pkgstream)
- [17. RAG Pipeline — `pkg/rag`](#17-rag-pipeline--pkgrag)
- [18. Multi-Agent Conversations — `pkg/conv`](#18-multi-agent-conversations--pkgconv)

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
│   ┌─────────────────┬──────────┬──────────┬──────────────────┐   │
│   │   pkg/memory    │ pkg/output│ pkg/prompt│   pkg/stream    │   │
│   │ Buffer/Window   │ JSONParser│ Template │ Observer/Chain   │   │
│   │ Summary/Store   │ SchemaFor │ FewShot  │ Step/Collector   │   │
│   └─────────────────┴──────────┴──────────┴──────────────────┘   │
│   ┌──────────────────────────┬───────────────────────────────┐   │
│   │       pkg/rag            │         pkg/conv              │   │
│   │ Embedder/VectorStore     │ Channel/Participant           │   │
│   │ Splitter/Pipeline        │ Moderator/Envelope            │   │
│   └──────────────────────────┴───────────────────────────────┘   │
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
| `memory.BufferStore` | `sync.RWMutex` | Safe concurrent read/write |
| `memory.WindowStore` | `sync.RWMutex` | Safe concurrent read/write |
| `memory.SummaryStore` | `sync.RWMutex` | Safe concurrent read/write (summarization under lock) |
| `rag.InMemoryStore` | `sync.RWMutex` | Safe concurrent add/search |
| `conv.Channel` | `sync.Mutex` | Safe concurrent send/receive |

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
Core packages (pkg/agent, pkg/waggle, pkg/observe, pkg/web, pkg/memory, pkg/output, pkg/prompt, pkg/stream, pkg/rag, pkg/conv):
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

## 13. Memory Layer — `pkg/memory`

### Purpose and Design Rationale

The memory package provides conversational memory for LLM agents, enabling multi-turn interactions where past messages influence future responses. Memory is decoupled from the LLM package to avoid import cycles and to allow multiple memory strategies to be composed independently.

The `Message` type deliberately uses `string` for `Role` (not `llm.Role`) to break the dependency on the `pkg/llm` package, keeping the memory layer self-contained and importable by any package.

### Key Interfaces and Types

```go
// Store is the core memory interface for conversational history.
type Store interface {
    // Add appends a message to the conversation history.
    Add(ctx context.Context, msg Message) error
    // Messages returns the current conversation history.
    Messages(ctx context.Context) ([]Message, error)
    // Clear removes all messages from the store.
    Clear(ctx context.Context) error
}

// Message represents a single conversational message.
type Message struct {
    Role    string // "system", "user", "assistant" — string to avoid llm import
    Content string
}
```

#### BufferStore — Unbounded History

`BufferStore` is the simplest implementation: an append-only slice of messages protected by `sync.RWMutex`. Suitable for short conversations or when the caller manages truncation externally.

```go
type BufferStore struct {
    mu       sync.RWMutex
    messages []Message
}
```

#### WindowStore — Sliding Window

`WindowStore` keeps the most recent `n` messages while always preserving pinned system messages at the front. When the window limit is exceeded, the oldest non-system messages are dropped.

```go
type WindowStore struct {
    mu       sync.RWMutex
    messages []Message
    maxSize  int
}
```

#### SummaryStore — Compression via Summarization

`SummaryStore` monitors the message count and triggers a `Summarizer` function when the threshold is exceeded. The summarizer compresses older messages into a single summary message, keeping the conversation within context limits.

```go
// Summarizer compresses a slice of messages into a summary message.
type Summarizer func(ctx context.Context, messages []Message) (Message, error)

type SummaryStore struct {
    mu         sync.RWMutex
    messages   []Message
    threshold  int
    summarizer Summarizer
}
```

### Integration with Existing Packages

Memory integrates with the LLM layer via the `llm.WithMemory(store)` option on `LLMAgent`. When memory is configured, the agent automatically loads conversation history before each call and appends the user input and assistant response after each call.

```go
agent := llm.NewLLMAgent[string]("chatbot", provider,
    llm.WithSystemPrompt("You are a helpful assistant."),
    llm.WithMemory(memory.NewWindowStore(20)),
)
```

### Thread Safety

All store implementations use `sync.RWMutex` to guarantee safe concurrent access. `Add` and `Clear` acquire a write lock; `Messages` acquires a read lock. `SummaryStore` performs summarization under the write lock to prevent concurrent reads from observing a partially compressed history.

---

## 14. Structured Output — `pkg/output`

### Purpose and Design Rationale

The output package enables LLM agents to return typed Go structs instead of raw strings. This bridges the gap between unstructured LLM text and the type-safe `Agent[I, O]` pipeline. The package uses a three-tier extraction strategy to maximize compatibility across LLMs that may format JSON differently.

### Key Interfaces and Types

```go
// Parser[O] extracts a typed value from raw LLM output.
type Parser[O any] interface {
    // Parse attempts to extract a value of type O from the raw string.
    Parse(raw string) (O, error)
    // FormatInstruction returns a string to append to the prompt,
    // instructing the LLM on the expected output format.
    FormatInstruction() string
}
```

#### JSONParser — Three-Tier Extraction

`JSONParser[O]` attempts to parse LLM output as JSON using a three-tier strategy:

1. **Direct parse:** Try `json.Unmarshal` on the entire response.
2. **Code block extraction:** Look for `` ```json ... ``` `` fenced blocks and parse the content.
3. **Bracket matching:** Find the outermost `{...}` or `[...]` and parse that substring.

This gracefully handles LLMs that wrap JSON in markdown, add preamble text, or include trailing commentary.

```go
type JSONParser[O any] struct{}

func (p JSONParser[O]) Parse(raw string) (O, error)
func (p JSONParser[O]) FormatInstruction() string
```

#### SchemaFor — Reflection-Based JSON Schema

`SchemaFor[O]()` generates a JSON Schema string from a Go struct's type information and struct tags. This schema is included in the prompt instruction so the LLM knows exactly what fields and types to produce.

```go
func SchemaFor[O any]() string
```

#### NewStructuredAgent — Agent with Parse + Retry

`NewStructuredAgent` wraps an LLM agent and a `Parser[O]` into an `Agent[I, O]`. If parsing fails, it retries the LLM call (up to a configurable limit) with an augmented prompt that includes the parse error, giving the LLM a chance to correct its output.

```go
func NewStructuredAgent[I, O any](name string, llmAgent agent.Agent[I, string], parser Parser[O]) agent.Agent[I, O]
```

### Integration with Existing Packages

`NewStructuredAgent` returns a standard `Agent[I, O]`, making it fully composable with `Chain`, `Parallel`, `WithRetry`, and all other agent primitives. It bridges the untyped LLM world with the typed pipeline world.

```go
type Review struct {
    Score    int    `json:"score"`
    Summary  string `json:"summary"`
    Issues   []string `json:"issues"`
}

reviewer := output.NewStructuredAgent[string, Review]("reviewer", llmAgent, output.JSONParser[Review]{})
// reviewer is Agent[string, Review] — fully composable
pipeline := agent.Chain2(fetcher, reviewer)
```

### Thread Safety

`JSONParser` and `SchemaFor` are stateless and safe for concurrent use. `NewStructuredAgent` delegates concurrency guarantees to the underlying LLM agent.

---

## 15. Prompt Templates — `pkg/prompt`

### Purpose and Design Rationale

The prompt package provides a lightweight, dependency-free template system for constructing LLM prompts. It avoids external template engines (e.g., `text/template`) in favor of a simpler `{{var}}` placeholder syntax that is easier to read, less error-prone, and aligns with the zero-dependency philosophy.

### Key Interfaces and Types

#### Template — Immutable Variable Substitution

`Template` uses `{{var}}` placeholders and follows an immutable design: `WithVar` returns a new `Template` rather than mutating the original, making templates safe to share and reuse across goroutines.

```go
type Template struct {
    raw  string
    vars map[string]string
}

func New(raw string) Template
func (t Template) WithVar(name, value string) Template
func (t Template) Render() string
```

#### FewShotBuilder — Example-Based Prompts

`FewShotBuilder` constructs few-shot prompts with a system instruction, a set of input/output examples, and a final input. This pattern is effective for guiding LLMs to produce consistent output formats.

```go
type FewShotBuilder struct {
    instruction string
    examples    []Example
}

type Example struct {
    Input  string
    Output string
}

func NewFewShot(instruction string) *FewShotBuilder
func (b *FewShotBuilder) Add(input, output string) *FewShotBuilder
func (b *FewShotBuilder) BuildWithInput(input string) string
```

#### AsPromptFunc — LLMAgent Compatibility

`AsPromptFunc()` converts a `Template` into a `PromptFunc` compatible with `llm.NewLLMAgent`, bridging the prompt and LLM layers.

```go
func (t Template) AsPromptFunc() func(ctx context.Context, input string) ([]llm.Message, error)
```

### Integration with Existing Packages

Templates integrate with the LLM layer by converting to `PromptFunc` values that `NewLLMAgent` accepts. They can also be used standalone for any string-formatting need.

```go
tmpl := prompt.New("Analyze {{language}} code:\n{{code}}")
agent := llm.NewLLMAgent[string]("analyzer", provider,
    llm.WithPromptFunc(tmpl.WithVar("language", "Go").AsPromptFunc()),
)
```

### Thread Safety

`Template` is immutable — `WithVar` returns a new instance. This makes templates inherently safe for concurrent use without synchronization.

---

## 16. Observable Pipelines — `pkg/stream`

### Purpose and Design Rationale

The stream package adds observability to agent pipelines by emitting structured `Step` events at each agent boundary. This enables real-time progress tracking, debugging, and integration with the web visualization layer via SSE.

### Key Interfaces and Types

#### Step — Pipeline Event

```go
type Step struct {
    AgentName string
    StepType  StepType  // "started", "chunk", "completed", "error"
    Content   string
    Index     int
    Timestamp time.Time
}

type StepType string

const (
    StepStarted   StepType = "started"
    StepChunk     StepType = "chunk"
    StepCompleted StepType = "completed"
    StepError     StepType = "error"
)
```

#### Observer — Event Sink

```go
// Observer receives pipeline step events.
type Observer interface {
    OnStep(step Step)
}

// ObserverFunc is a function adapter for Observer.
type ObserverFunc func(Step)

func (f ObserverFunc) OnStep(step Step) { f(step) }
```

#### Observable Chains

`ObservableChain2` and `ObservableChain3` wrap `agent.Chain2`/`Chain3` with step emission at each agent boundary:

```go
func ObservableChain2[A, B, C any](
    a agent.Agent[A, B],
    b agent.Agent[B, C],
    observers ...Observer,
) agent.Agent[A, C]

func ObservableChain3[A, B, C, D any](
    a agent.Agent[A, B],
    b agent.Agent[B, C],
    c agent.Agent[C, D],
    observers ...Observer,
) agent.Agent[A, D]
```

#### MultiObserver and Collector

`MultiObserver` fans out steps to multiple observers. `Collector` accumulates steps in a slice for testing and inspection.

```go
type MultiObserver struct {
    observers []Observer
}

type Collector struct {
    mu    sync.Mutex
    Steps []Step
}

func (c *Collector) OnStep(step Step)
```

### Integration with Existing Packages

Observable chains are drop-in replacements for standard chains — they return the same `Agent[I, O]` type. Steps can be forwarded to the `web/sse.go` hub for real-time UI updates in the dashboard.

```go
obs := stream.ObserverFunc(func(s stream.Step) {
    sseHub.Broadcast(s) // forward to web dashboard
})
pipeline := stream.ObservableChain2(fetcher, parser, obs)
```

### Thread Safety

`Collector` uses `sync.Mutex` to protect the `Steps` slice for safe concurrent accumulation. `ObserverFunc` and `MultiObserver` delegate thread safety to the underlying observer implementations.

---

## 17. RAG Pipeline — `pkg/rag`

### Purpose and Design Rationale

The RAG (Retrieval-Augmented Generation) package provides a complete pipeline for grounding LLM responses in external knowledge. It defines interfaces for embedding, vector storage, and text splitting, with in-memory implementations that require zero external dependencies.

### Key Interfaces and Types

#### Embedder — Text to Vectors

```go
// Embedder converts text into vector embeddings.
type Embedder interface {
    // Embed converts a batch of text strings into embedding vectors.
    Embed(ctx context.Context, texts []string) ([][]float64, error)
    // Dimensions returns the dimensionality of the embedding vectors.
    Dimensions() int
}
```

#### VectorStore — Similarity Search

```go
// Document represents a text chunk with its embedding and metadata.
type Document struct {
    ID        string
    Content   string
    Embedding []float64
    Metadata  map[string]string
}

// VectorStore persists and searches document embeddings.
type VectorStore interface {
    // Add stores documents with their embeddings.
    Add(ctx context.Context, docs []Document) error
    // Search finds the top-K most similar documents to the query vector.
    Search(ctx context.Context, vector []float64, topK int) ([]Document, error)
}
```

#### InMemoryStore — Zero-Dependency Vector Search

`InMemoryStore` implements `VectorStore` using brute-force cosine similarity search. It is protected by `sync.RWMutex` for safe concurrent access and requires no external vector database.

```go
type InMemoryStore struct {
    mu   sync.RWMutex
    docs []Document
}
```

#### Splitter — Text Chunking

```go
// Splitter breaks text into chunks suitable for embedding.
type Splitter interface {
    Split(text string) []string
}

// TokenSplitter splits text into chunks of approximately N tokens.
type TokenSplitter struct {
    ChunkSize    int
    ChunkOverlap int
}

// ParagraphSplitter splits text on paragraph boundaries.
type ParagraphSplitter struct{}
```

#### NewPipeline — End-to-End RAG

`NewPipeline` composes the full RAG flow into a single `Agent[string, string]`:

```
Query ──> Embed(query) ──> VectorStore.Search(topK) ──> Build context prompt ──> LLM ──> Answer
```

```go
func NewPipeline(
    embedder Embedder,
    store VectorStore,
    llmAgent agent.Agent[string, string],
    topK int,
) agent.Agent[string, string]
```

### Integration with Existing Packages

`NewPipeline` returns a standard `Agent[string, string]`, so it composes with chains, decorators, and orchestration patterns. The LLM agent parameter can be any agent that accepts a string prompt, including agents built with `pkg/output` for structured responses.

```go
ragAgent := rag.NewPipeline(embedder, vectorStore, llmAgent, 5)
pipeline := agent.Chain2(inputProcessor, ragAgent)
reliable := agent.WithRetry(pipeline, agent.WithMaxAttempts(3))
```

### Thread Safety

`InMemoryStore` uses `sync.RWMutex` — `Add` acquires a write lock, `Search` acquires a read lock. This allows concurrent searches while serializing writes. The pipeline itself is stateless beyond the store and delegates concurrency to its component agents.

---

## 18. Multi-Agent Conversations — `pkg/conv`

### Purpose and Design Rationale

The conv package enables multi-agent conversations where multiple participants exchange messages in a structured, turn-taking protocol. This is useful for debate-style reasoning, collaborative problem solving, and agent-to-agent negotiation. A `Moderator` orchestrates the conversation flow with configurable rounds, termination conditions, and turn order.

### Key Interfaces and Types

#### Envelope — Message Container

```go
// Envelope wraps a message with routing metadata.
type Envelope struct {
    From    string // sender participant name
    To      string // recipient participant name ("" for broadcast)
    Content string
    Round   int
}
```

#### Channel — Thread-Safe Message Queue

`Channel` is a thread-safe FIFO queue for passing envelopes between participants:

```go
type Channel struct {
    mu       sync.Mutex
    messages []Envelope
}

func (c *Channel) Send(env Envelope)
func (c *Channel) Receive() ([]Envelope, bool)
func (c *Channel) Clear()
```

#### Participant — Conversation Member

```go
// Participant is an entity that can engage in multi-agent conversations.
type Participant interface {
    // Name returns the participant's unique identifier.
    Name() string
    // Respond generates a response given the conversation history.
    Respond(ctx context.Context, history []Envelope) (Envelope, error)
}

// FuncParticipant adapts a function into a Participant.
type FuncParticipant struct {
    name string
    fn   func(ctx context.Context, history []Envelope) (Envelope, error)
}
```

#### Moderator — Turn-Taking Orchestrator

The `Moderator` manages conversation flow:

```go
type Moderator struct {
    participants []Participant
    maxRounds    int
    turnOrder    []string          // custom turn order (optional)
    termination  func([]Envelope) bool // early termination condition
}

func NewModerator(participants []Participant, opts ...ModeratorOption) *Moderator
func (m *Moderator) Run(ctx context.Context, topic string) ([]Envelope, error)
```

**Conversation flow:**

```
Topic ──> Round 1: Participant A responds
                   Participant B responds
          Round 2: Participant A responds
                   Participant B responds
          ...
          termination condition met OR maxRounds reached
          ──> return full conversation history
```

#### AsAgent — Moderator as Agent

`AsAgent()` converts a `Moderator` into a standard `Agent[string, []Envelope]`, enabling conversations to be embedded in larger pipelines.

```go
func (m *Moderator) AsAgent() agent.Agent[string, []Envelope]
```

### Integration with Existing Packages

Through `AsAgent()`, a multi-agent conversation becomes a regular pipeline node. Participants can internally use `pkg/llm` agents, `pkg/memory` stores, or `pkg/rag` pipelines, enabling rich conversational workflows.

```go
debater1 := conv.FuncParticipant("optimist", optimistFn)
debater2 := conv.FuncParticipant("critic", criticFn)
mod := conv.NewModerator([]conv.Participant{debater1, debater2},
    conv.WithMaxRounds(5),
    conv.WithTermination(consensusReached),
)
// Use as an agent in a pipeline
debate := mod.AsAgent() // Agent[string, []Envelope]
pipeline := agent.Chain2(topicGenerator, agent.Erase(debate))
```

### Thread Safety

`Channel` uses `sync.Mutex` for safe concurrent send and receive operations. The `Moderator` itself drives conversation sequentially (one participant at a time per round), so it does not require additional synchronization. Participants are responsible for their own internal thread safety.

---

*Waggle v0.6.0 — Apache 2.0 License*
