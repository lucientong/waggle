# Waggle 架构文档

> 深入解析 Waggle AI Agent 编排引擎的架构、设计原理和内部实现。

---

## 目录

- [1. 概述](#1-概述)
- [2. 设计理念](#2-设计理念)
- [3. 项目结构](#3-项目结构)
- [4. 核心层 — `pkg/agent`](#4-核心层--pkgagent)
  - [4.1 Agent 接口](#41-agent-接口)
  - [4.2 类型擦除桥梁](#42-类型擦除桥梁)
  - [4.3 Chain — 串行管道](#43-chain--串行管道)
  - [4.4 装饰器](#44-装饰器)
  - [4.5 错误类型](#45-错误类型)
- [5. 编排层 — `pkg/waggle`](#5-编排层--pkgwaggle)
  - [5.1 DAG 数据结构](#51-dag-数据结构)
  - [5.2 Waggle 编排器](#52-waggle-编排器)
  - [5.3 并发 DAG 执行器](#53-并发-dag-执行器)
  - [5.4 编排模式](#54-编排模式)
- [6. LLM 集成层 — `pkg/llm`](#6-llm-集成层--pkgllm)
  - [6.1 Provider 接口](#61-provider-接口)
  - [6.2 Provider 实现](#62-provider-实现)
  - [6.3 智能路由器](#63-智能路由器)
  - [6.4 LLM Agent](#64-llm-agent)
  - [6.5 Tool Agent（ReAct 循环）](#65-tool-agentreact-循环)
- [7. 可观测性层 — `pkg/observe`](#7-可观测性层--pkgobserve)
  - [7.1 事件系统](#71-事件系统)
  - [7.2 分布式追踪](#72-分布式追踪)
  - [7.3 指标采集](#73-指标采集)
  - [7.4 结构化日志](#74-结构化日志)
- [8. Web 可视化 — `pkg/web` + `web/`](#8-web-可视化--pkgweb--web)
  - [8.1 内嵌 HTTP 服务器](#81-内嵌-http-服务器)
  - [8.2 REST API](#82-rest-api)
  - [8.3 SSE 实时事件](#83-sse-实时事件)
  - [8.4 前端（D3.js）](#84-前端d3js)
- [9. CLI 命令行 — `cmd/waggle`](#9-cli-命令行--cmdwaggle)
  - [9.1 YAML 工作流定义](#91-yaml-工作流定义)
  - [9.2 命令列表](#92-命令列表)
- [10. 数据流架构](#10-数据流架构)
- [11. 并发模型](#11-并发模型)
- [12. 依赖策略](#12-依赖策略)
- [13. 记忆层 — `pkg/memory`](#13-记忆层--pkgmemory)
- [14. 结构化输出 — `pkg/output`](#14-结构化输出--pkgoutput)
- [15. 提示词模板 — `pkg/prompt`](#15-提示词模板--pkgprompt)
- [16. 可观测流水线 — `pkg/stream`](#16-可观测流水线--pkgstream)
- [17. RAG 管道 — `pkg/rag`](#17-rag-管道--pkgrag)
- [18. 多 Agent 对话 — `pkg/conv`](#18-多-agent-对话--pkgconv)

---

## 1. 概述

**Waggle** 是一个轻量级、可嵌入的 AI Agent 编排引擎，基于 Go 1.26+ 构建。名字源于蜜蜂的 *摇摆舞*（waggle dance）—— 蜜蜂通过摇摆舞向蜂群传达食物来源的方向和距离。在 Waggle 中，每个 Agent 是一只蜜蜂，goroutine 是它的翅膀，channel 是蜂群的通信之舞。

```
┌─────────────────────────────────────────────────────────────────┐
│                         应用程序                                 │
│   ┌─────────────────────────────────────────────────────────┐   │
│   │                  pkg/agent（核心层）                      │   │
│   │  Agent[I,O] · Func · Chain2-5 · Retry · Timeout · Cache │   │
│   └────────────────────────────┬────────────────────────────┘   │
│                                │                                 │
│   ┌────────────────────────────┴────────────────────────────┐   │
│   │              pkg/waggle（编排层）                          │   │
│   │  DAG · Executor · Parallel · Race · Vote · Router · Loop │   │
│   └────────────────────────────┬────────────────────────────┘   │
│                                │                                 │
│   ┌─────────────────┬──────────┴───────┬────────────────────┐   │
│   │   pkg/llm       │  pkg/observe     │   pkg/web          │   │
│   │ OpenAI/Claude   │ 事件/追踪         │ 仪表盘/SSE         │   │
│   │ Ollama/路由器    │ 指标/日志         │ D3.js 可视化       │   │
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
│   │             cmd/waggle（CLI 命令行）                      │   │
│   │  run · serve · validate · dot · version                  │   │
│   └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 设计理念

### 2.1 编译期类型安全

`Agent[I, O any]` 接口利用 Go 泛型在编译期强制类型匹配。当你串联 Agent 时，Go 编译器会验证前一个 Agent 的输出类型与后一个的输入类型是否一致：

```go
fetcher   := agent.Func[string, []byte]("fetcher", fetchURL)
parser    := agent.Func[[]byte, Document]("parser", parseHTML)
pipeline  := agent.Chain2(fetcher, parser) // 编译器强制确保 []byte 匹配
```

如果尝试 `Chain2[string, int, Document]`，编译时就会报错——而非等到运行时才发现。

### 2.2 核心零外部依赖

核心包（`pkg/agent`、`pkg/waggle`、`pkg/observe`、`pkg/web`）仅依赖 Go 标准库：`context`、`sync`、`net/http`、`embed`、`log/slog`、`encoding/json`。唯一的外部依赖 `gopkg.in/yaml.v3` 仅在 CLI 中用于 YAML 工作流解析。

### 2.3 Goroutine-per-Agent 并发模型

与受 GIL 限制的 Python 框架不同，Waggle 充分利用 Go 的原生并发。DAG 中的每个 Agent 在独立的 goroutine 中运行，通过类型化的 channel 通信，实现跨 CPU 核心的真正并行执行。

### 2.4 函数选项模式

所有可配置组件统一使用函数选项模式（Functional Options），API 简洁且可扩展：

```go
reliable := agent.WithRetry(myAgent,
    agent.WithMaxAttempts(5),
    agent.WithBaseDelay(200*time.Millisecond),
)
```

### 2.5 装饰器可组合性

装饰器（`WithRetry`、`WithTimeout`、`WithCache`）包装任意 `Agent[I,O]` 并返回新的 `Agent[I,O]`，支持任意嵌套：

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

## 3. 项目结构

```
waggle/
├── pkg/
│   ├── agent/          # 第 0 层：核心 Agent 接口 + 基础组件
│   │   ├── agent.go        # Agent[I,O]、UntypedAgent、Erase()
│   │   ├── func_agent.go   # Func() — 从函数创建 Agent
│   │   ├── chain.go        # Chain2 到 Chain5
│   │   ├── retry.go        # WithRetry（指数退避 + 抖动）
│   │   ├── timeout.go      # WithTimeout（截止时间约束）
│   │   ├── cache.go        # WithCache（记忆化缓存）
│   │   └── errors.go       # ErrTypeMismatch, RetryExhaustedError, TimeoutError
│   │
│   ├── waggle/         # 第 1 层：DAG 编排引擎
│   │   ├── dag.go          # DAG 数据结构 + 算法
│   │   ├── waggle.go       # Waggle 编排器
│   │   ├── executor.go     # 并发 DAG 执行器
│   │   └── patterns.go     # Parallel、Race、Vote、Router、Loop
│   │
│   ├── llm/            # 第 2 层：LLM Provider 集成
│   │   ├── provider.go     # Provider 接口 + 类型定义
│   │   ├── openai.go       # OpenAI Chat Completions
│   │   ├── anthropic.go    # Anthropic Messages API
│   │   ├── ollama.go       # Ollama 本地推理
│   │   ├── router.go       # 多 Provider 路由器
│   │   ├── llm_agent.go    # LLMAgent 构建器
│   │   └── tool_agent.go   # ToolAgent（ReAct 函数调用）
│   │
│   ├── observe/        # 第 2 层：可观测性
│   │   ├── event.go        # 事件类型 + 工厂函数
│   │   ├── tracer.go       # 基于 Span 的追踪（兼容 OTel）
│   │   ├── metrics.go      # 聚合指标采集器
│   │   └── logger.go       # 结构化日志（slog 包装器）
│   │
│   └── web/            # 第 2 层：可视化
│       ├── server.go       # HTTP 服务器（go:embed）
│       ├── api.go          # REST API 处理器
│       ├── sse.go          # Server-Sent Events Hub
│       └── static/         # 内嵌前端资源
│
├── web/                # 前端源码（镜像至 pkg/web/static/）
│   ├── index.html          # 单页应用
│   ├── app.js              # D3.js 力导向图
│   └── style.css           # 暗色主题 CSS
│
├── cmd/waggle/         # CLI 二进制
│   ├── main.go             # 入口 + 命令分发
│   └── commands/
│       ├── workflow.go     # YAML 工作流解析器 + 验证器
│       ├── run.go          # run / validate / dot 命令
│       └── serve.go        # serve / version 命令
│
└── examples/           # 实战示例
    ├── code_review/        # Chain4 + Cache + Retry 管道
    ├── research/           # Parallel + Race + Chain2
    └── customer_support/   # Router + Loop + Chain2
```

---

## 4. 核心层 — `pkg/agent`

### 4.1 Agent 接口

`Agent[I, O any]` 接口是最基本的构建模块：

```go
type Agent[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
}
```

**设计决策：**

- **两个类型参数**（`I` 为输入，`O` 为输出）提供完整的管道类型安全。
- **`context.Context`** 作为第一个参数，支持取消、超时和值传播。
- **`Name()`** 用于日志记录、追踪、错误消息和 DAG 节点标识。

创建 Agent 最简单的方式是通过 `Func()`：

```go
func Func[I, O any](name string, fn func(ctx context.Context, input I) (O, error)) Agent[I, O]
```

### 4.2 类型擦除桥梁

对于动态场景（YAML 工作流、DAG 执行），编译时无法确定类型。Waggle 提供了类型擦除机制：

```
Agent[I, O]  ──Erase()──>  UntypedAgent
     ▲                          │
     │                          │ RunUntyped(ctx, any) (any, error)
  编译期                         │
  类型安全                    运行时类型
                             断言 (I)
```

```go
type UntypedAgent interface {
    Name() string
    RunUntyped(ctx context.Context, input any) (any, error)
}

func Erase[I, O any](a Agent[I, O]) UntypedAgent
```

`Erase()` 包装一个有类型的 Agent。在运行时，`RunUntyped` 对输入执行类型断言。如果断言失败，返回 `*ErrTypeMismatch`，包含 Agent 名称和接收到的类型。

### 4.3 Chain — 串行管道

Chain 函数将 Agent 组合成类型安全的串行管道：

```
Chain2: Agent[A,B] + Agent[B,C] => Agent[A,C]
Chain3: Agent[A,B] + Agent[B,C] + Agent[C,D] => Agent[A,D]
Chain4: Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] => Agent[A,E]
Chain5: Agent[A,B] + Agent[B,C] + Agent[C,D] + Agent[D,E] + Agent[E,F] => Agent[A,F]
```

**实现方式：** Chain3-5 通过递归组合 Chain2 构建：
```go
func Chain3[A, B, C, D any](a Agent[A, B], b Agent[B, C], c Agent[C, D]) Agent[A, D] {
    return Chain2(Chain2(a, b), c)
}
```

**错误处理：** 任一阶段返回错误，立即短路退出。错误会被包装为失败阶段的名称：`chain stage "fetch": connection refused`。

**Context 检查：** 每个阶段之间检查 `ctx.Err()`，实现快速取消而不进入下一阶段。

### 4.4 装饰器

#### WithRetry — 指数退避重试

```
尝试 1 → 失败 → 等待(baseDelay × 2⁰ × jitter)
尝试 2 → 失败 → 等待(baseDelay × 2¹ × jitter)
尝试 3 → 失败 → RetryExhaustedError{Attempts: 3, LastErr: ...}
```

- **退避公式：** `min(baseDelay × 2^attempt, maxDelay)`
- **抖动（Jitter）：** 随机因子 `[0.5, 1.5)`，防止雷群效应
- **Context 感知：** 每次重试前检查 `ctx.Err()`，休眠期间使用 `select` 监听 `ctx.Done()`
- **默认值：** 3 次尝试、100ms 基础延迟、30s 最大延迟、启用抖动

#### WithTimeout — 超时约束

每次 `Run()` 调用都会使用 `context.WithTimeout` 包装。如果父 context 已有更短的截止时间，以更短的为准。超时返回 `*TimeoutError`（`Unwrap()` 返回 `context.DeadlineExceeded`）。

#### WithCache — 记忆化缓存

使用 `sync.Map` 实现无锁并发读取。`keyFunc` 将输入映射为字符串缓存键。**结果和错误都会被缓存** —— 如果不想缓存错误，应在 `WithCache` 之前组合 `WithRetry`：

```go
cached := agent.WithCache(
    agent.WithRetry(flaky, agent.WithMaxAttempts(3)),
    keyFunc,
)
```

### 4.5 错误类型

| 错误 | 触发时机 | Unwrap |
|------|---------|--------|
| `*ErrTypeMismatch` | `UntypedAgent` 接收到错误类型 | — |
| `*RetryExhaustedError` | 所有重试耗尽 | `LastErr` |
| `*TimeoutError` | 执行超过截止时间 | `context.DeadlineExceeded` |

所有错误类型都实现了 `Error() string`，适用的还实现了 `Unwrap() error`，支持 `errors.Is()` 和 `errors.As()`。

---

## 5. 编排层 — `pkg/waggle`

### 5.1 DAG 数据结构

DAG 是工作流执行的骨架：

```go
type DAG struct {
    nodes     map[string]*node        // id → 节点
    adjacency map[string][]string     // id → 后继节点 id（出边）
    reverse   map[string][]string     // id → 前驱节点 id（入边）
    edges     []edge                  // 所有边
}
```

**核心算法：**

| 操作 | 算法 | 复杂度 |
|------|------|--------|
| `addEdge(from, to)` | 迭代 DFS 环检测 | 每次 O(V + E) |
| `TopologicalSort()` | Kahn 算法（BFS） | O(V + E) |
| `Layers()` | BFS 分层遍历 | O(V + E) |
| `CriticalPath()` | 拓扑序上的动态规划 | O(V + E) |

**环检测：** 每次 `addEdge()` 都会实时检查：「是否存在从 `to` 到 `from` 的路径？」如果存在，则拒绝添加边并返回 `ErrCycleDetected`。这是增量式环检测，而非推迟到排序时才检查。

**关键路径分析：** 在拓扑序上使用动态规划。对每个节点，`dist[id] = max(dist[pred] + weight[id])`。关键路径决定了工作流的理论最短执行时间。

**分层：** 按 BFS 层级对节点分组。第 0 层 = 源节点。同一层的所有节点可并行执行：

```
第 0 层: [A, B]      ← 无前驱，立即启动
第 1 层: [C, D]      ← 依赖第 0 层
第 2 层: [E]          ← 依赖第 1 层
```

### 5.2 Waggle 编排器

`Waggle` 结构体是构建和运行工作流的高层 API：

```go
w := waggle.New()
w.Register(agent.Erase(fetcher), agent.Erase(parser))
w.Connect("fetcher", "parser")
result, err := w.RunFrom(ctx, "fetcher", "https://example.com")
```

`Register()` 将 UntypedAgent 注册为 DAG 节点。`Connect()` 声明数据流边并内置环检测。`RunFrom()` 从指定节点开始，按拓扑序执行管道。

`DAGInfo()` 返回只读的 `DAGSnapshot`，包含节点 ID、名称、前驱和后继，用于可视化。

### 5.3 并发 DAG 执行器

执行器实现 goroutine-per-node 执行模型：

```
源节点               中间节点              汇节点
┌───────┐         ┌───────┐           ┌───────┐
│Agent A│──ch──>  │Agent C│──ch──>    │Agent E│──> 结果
└───────┘    ╲    └───────┘           └───────┘
              ╲                          ╱
┌───────┐      ╲  ┌───────┐           ╱
│Agent B│──ch──> ─│Agent D│──ch──>  ─╱
└───────┘         └───────┘
```

**执行流程：**

1. **计算拓扑序** —— 通过 Kahn 算法
2. **分配触发通道** —— 每个节点一个（带缓冲，大小为 1）
3. **源节点** 立即获得触发信号
4. **启动 goroutine** —— 每个节点一个，各自等待其触发通道
5. 当一个 goroutine 完成时：
   - 将输出存入 `map[string]any`（互斥锁保护）
   - 为每个后继递增就绪计数
   - 如果后继的就绪计数 == 前驱数量，发送触发信号
6. **扇入（Fan-in）：** 多前驱节点收到 `[]any`（聚合输出）
7. **扇出（Fan-out）：** 一个节点的输出转发给所有后继
8. **错误传播：** 首个错误取消整个 context，所有 goroutine 退出

### 5.4 编排模式

所有模式返回 `Agent[I, O]`，因此可以与 Chain、装饰器和其他模式任意组合。

#### Parallel — 扇出，收集全部结果

```
输入 ─┬─> Agent1 ─┐
      ├─> Agent2 ─┤──> ParallelResults[O]{Results, Errors}
      └─> Agent3 ─┘
```

所有 Agent 以相同输入并发执行。结果按 Agent 索引顺序排列。Run 本身不返回错误——部分失败记录在 `ParallelResults.Errors` 中。

#### Race — 竞速，最快取胜

```
输入 ─┬─> Agent1 ──╮
      ├─> Agent2 ──┤──> 首个成功的结果
      └─> Agent3 ──╯    （其他通过 ctx 取消）
```

适用于延迟对冲：同时向多个 LLM 发送请求，取最先响应的结果。

#### Vote — 投票共识

```
输入 ─┬─> Judge1 ─┐
      ├─> Judge2 ─┤──> VoteFunc(candidates) ──> 获胜者
      └─> Judge3 ─┘
```

`MajorityVote[O]()` 使用 `fmt.Sprintf("%v", v)` 进行比较，要求超过 50% 的一致。

#### Router — 条件路由

```
输入 ──> routeFn(input) ──> "billing"   ──> billingAgent
                            "technical" ──> techAgent
                            "unknown"   ──> fallbackAgent
```

`WithFallback` 为未识别的键提供默认分支。

#### Loop — 迭代精炼

```
输入 ──> initAgent ──> 输出
              ▲            │
              │            ▼
              │       condition(output)?
              │        是  │    否
              │            │    └──> 返回输出
              └────────────┘
         bodyAgent(output)
```

`initAgent` 转换 `I → O`（首次执行）。`bodyAgent` 精炼 `O → O`（每次迭代）。循环在 `condition` 返回 `false` 或达到 `maxIterations` 时终止。

---

## 6. LLM 集成层 — `pkg/llm`

### 6.1 Provider 接口

```go
type Provider interface {
    Info() ProviderInfo
    Chat(ctx context.Context, messages []Message) (string, error)
    ChatStream(ctx context.Context, messages []Message) (<-chan string, error)
}
```

**设计决策：** 所有实现直接使用 `net/http` —— 不依赖外部 SDK。这保持了最小的依赖树，并对请求构造、错误处理和流式传输拥有完全控制。

### 6.2 Provider 实现

| Provider | API 端点 | 默认模型 | 流式方式 | 成本 | 上下文窗口 |
|----------|---------|----------|---------|------|-----------|
| **OpenAI** | `/v1/chat/completions` | `gpt-4o` | SSE（`data: [DONE]`） | $0.005/1K | 128K |
| **Anthropic** | `/v1/messages` | `claude-3-5-sonnet` | SSE（`content_block_delta`） | $0.003/1K | 200K |
| **Ollama** | `/api/chat` | `llama3.2` | NDJSON（行分隔） | 免费 | 8K |

**Anthropic 特殊处理：** System 消息被提取并作为顶层 `system` 字段发送（Anthropic API 要求），而非放在 messages 数组中。

**Ollama Chat 实现：** `Chat()` 通过调用 `ChatStream()` 并收集所有 token 实现——避免代码重复。

### 6.3 智能路由器

路由器将多个 Provider 封装在单一 `Provider` 接口之后，提供四种策略：

```
          ┌─> OpenAI    （成本: $0.005, 延迟: 800ms）
请求 ─────┤─> Anthropic （成本: $0.003, 延迟: 600ms）
          └─> Ollama    （成本: $0.000, 延迟: 2000ms）
```

| 策略 | 选择逻辑 |
|------|---------|
| `StrategyLowestCost` | 按 `CostPer1KTokens` 排序，优先使用最便宜的 |
| `StrategyLowestLatency` | 按 `AvgLatencyMs` 排序，优先使用最快的 |
| `StrategyRoundRobin` | 顺序轮询分发 |
| `StrategyFailover` | 按序尝试，失败时回退到下一个（默认策略） |

`ChatStream` 始终使用 failover 策略，并跳过 `SupportsStreaming` 为 false 的 Provider。

### 6.4 LLM Agent

`NewLLMAgent[I]` 将 Provider 接口桥接到 Agent 接口：

```
输入 I ──> PromptFunc(ctx, input) ──> []Message ──> Provider.Chat() ──> string
```

`SimplePrompt[I]` 是常见场景（固定 system prompt + 格式化 user 消息）的便捷构建器。

### 6.5 Tool Agent（ReAct 循环）

ToolAgent 实现 ReAct 风格的推理循环，用于函数调用：

```
                    ┌────────────────────────────┐
                    ▼                            │
用户输入 ──> LLM 调用 ──> 解析响应                │
                    │           │                │
                    │     ┌─────┴──────┐         │
                    │     │            │         │
                    │ tool_calls?  final_answer? │
                    │     │            │         │
                    │     ▼            ▼         │
                    │  执行工具     返回结果       │
                    │     │                      │
                    │     └──────────────────────┘
                    │     （将结果追加到
                    │      对话历史中）
```

**协议：** 工具定义以 JSON Schema 注入 system prompt。LLM 被指示以结构化 JSON 响应：

- 工具调用：`{"thought": "...", "tool_calls": [{"tool": "name", "args": "{...}"}]}`
- 最终回答：`{"final_answer": "回答文本"}`
- 非 JSON 响应被视为直接最终回答。

这种设计适用于**任何 LLM** —— 不要求原生的 function calling API 支持。

---

## 7. 可观测性层 — `pkg/observe`

### 7.1 事件系统

事件是所有可观测性的基础。六种事件类型覆盖完整生命周期：

```
workflow.start ──> agent.start ──> agent.end ──> data.flow ──> ... ──> workflow.end
                                   agent.error
```

每个 `Event` 携带：`Type`、`AgentName`、`WorkflowID`、`Timestamp`、`Duration`、`Error`、`InputSize`、`OutputSize` 和可扩展的 `Metadata`。

事件通过 channel 流转，实现解耦消费：

```
执行器 ──> chan Event ──┬──> Metrics.ConsumeEvents()
                        ├──> Logger.ConsumeEvents()
                        ├──> SSE Hub（Web 面板）
                        └──> 自定义消费者
```

### 7.2 分布式追踪

`Tracer` 记录与 OpenTelemetry 概念兼容的 `Span` 对象：

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

Span 通过 `WithTracer(ctx, tracer)` / `TracerFromContext(ctx)` 进行上下文传播。`SpanExporter` 接口允许导出到 Jaeger、Zipkin 或 OTLP 后端。

### 7.3 指标采集

`Metrics` 以并发安全的方式（`sync.RWMutex`）聚合每个 Agent 的性能数据：

```go
type AgentMetrics struct {
    AgentName        string
    TotalRuns        int64          // 总运行次数
    SuccessRuns      int64          // 成功次数
    ErrorRuns        int64          // 错误次数
    TotalDuration    time.Duration  // 累计执行时间
    MinDuration      time.Duration  // 最快执行时间
    MaxDuration      time.Duration  // 最慢执行时间
    TotalInputBytes  int64          // 总输入字节数
    TotalOutputBytes int64          // 总输出字节数
}
```

派生指标：`AvgDuration()`（平均耗时）和 `ErrorRate()`（错误率）。通过 `ConsumeEvents()` 自动更新。

### 7.4 结构化日志

`Logger` 包装 `*slog.Logger`，提供工作流感知的便捷方法：

- `AgentStart`、`AgentEnd`、`AgentError`、`AgentRetry`
- `WorkflowStart`、`WorkflowEnd`
- Context 注入：`WithLogger(ctx, l)` / `LoggerFromContext(ctx)`
- `ConsumeEvents()` 用于自动将事件转换为日志

---

## 8. Web 可视化 — `pkg/web` + `web/`

### 8.1 内嵌 HTTP 服务器

服务器使用 `//go:embed static` 将前端打包进二进制文件：

```go
//go:embed static
var staticFiles embed.FS
```

路由：
- `/` → 内嵌的 `index.html`
- `/api/dag` → DAG 结构（JSON）
- `/api/metrics` → Agent 指标（JSON）
- `/api/events` → SSE 事件流
- `/health` → 健康检查

### 8.2 REST API

**GET /api/dag：**
```json
{
  "nodes": [{"id": "fetch", "name": "fetch", "status": "waiting", "predecessors": [], "successors": ["parse"]}],
  "edges": [{"from": "fetch", "to": "parse"}]
}
```

**GET /api/metrics：**
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

### 8.3 SSE 实时事件

SSE Hub 使用注册/移除/广播模式：

```
                      ┌──> 客户端 1 (chan string)
事件 ──> sseHub ─────┤──> 客户端 2 (chan string)
                      └──> 客户端 3 (chan string)
```

- **非阻塞广播：** 慢速客户端的消息会被丢弃（无反压）
- **连接生命周期：** 连接时发送初始 `{"type":"connected"}` ping
- **Hub 在 `NewServer()` 中启动**，确保 `Start()` 和 `httptest.NewServer(Handler())` 两种模式都能正常工作

### 8.4 前端（D3.js）

单页应用渲染力导向图：

```
┌─────────────────────────────────────────────┐
│ 🐝 Waggle        Agent Orchestration Engine │
├─────────────────────────┬───────────────────┤
│                         │  详情              │
│   [fetch] ──> [parse]   │  Agent: fetch     │
│      │                  │  状态: success     │
│      v                  │  运行: 42 次       │
│   [review]              │  平均: 120ms       │
│                         │                   │
│   DAG 可视化             │  事件日志          │
│   (D3.js 力导向图        │  10:30 agent.start│
│    支持拖拽/缩放)        │  10:30 agent.end  │
└─────────────────────────┴───────────────────┘
```

**功能特性：**
- **力模拟** —— 碰撞避免、电荷排斥和链接约束
- **状态着色** —— 等待（灰色）、运行中（蓝色脉冲动画）、成功（绿色）、错误（红色）
- **实时更新** —— 通过 SSE，节点状态变化即时动画
- **交互式** —— 点击节点查看详情面板，拖拽节点重排布局，滚轮缩放
- **自动刷新** —— 每 5 秒轮询指标

---

## 9. CLI 命令行 — `cmd/waggle`

### 9.1 YAML 工作流定义

```yaml
name: code-review
description: 自动化代码审查管道
agents:
  - name: fetcher
    type: func
    description: 获取 PR 内容
  - name: reviewer
    type: llm
    model: gpt-4o
    provider: openai
    prompt: "审查以下代码："
    retry:
      max_attempts: 3
      base_delay_ms: 200
    timeout_secs: 30
flow:
  - from: fetcher
    to: reviewer
```

**验证规则：**
- `name` 为必填项
- 所有 agent 名称必须唯一
- 所有 flow 边必须引用已注册的 agent
- `from` 和 `to` 都不能为空

### 9.2 命令列表

| 命令 | 说明 |
|------|------|
| `waggle run <workflow.yaml>` | 解析 → 构建编排器 → 查找源节点 → 执行 |
| `waggle validate <file>...` | 仅解析和验证，不执行 |
| `waggle dot <workflow.yaml>` | 导出 Graphviz DOT 格式 |
| `waggle serve [--addr :8080]` | 启动 Web 可视化面板 |
| `waggle version` | 打印版本信息 |

`waggle run` 使用 `signal.NotifyContext(SIGINT, SIGTERM)` 实现优雅关闭。

---

## 10. 数据流架构

系统中的完整数据流：

```
用户代码            核心层              编排层               可观测性层          可视化层
─────────         ──────             ─────────           ─────────────      ─────────
                                                          
创建 Agent                                                
    │                                                     
    ▼                                                     
agent.Func()  ──> Agent[I,O]                              
    │                                                     
    ▼                                                     
agent.Erase() ──> UntypedAgent ──> waggle.Register()      
    │                                                     
    ▼                                                     
waggle.Connect() ────────────────> DAG.addEdge()          
                                   （环检测）               
    │                                                     
    ▼                                                     
waggle.RunFrom() ──────────────> TopologicalSort()        
                                       │                   
                                       ▼                   
                                 每节点一个 goroutine ──> 事件发射
                                       │                      │
                                       ▼                      ▼
                                 channel 触发          Metrics.Consume()
                                       │               Logger.Consume()
                                       ▼                      │
                                 输出收集                      ▼
                                       │               SSE Hub 广播
                                       ▼                      │
                                 返回结果                      ▼
                                                       D3.js 实时更新
```

---

## 11. 并发模型

### 线程安全保证

| 组件 | 机制 | 保证 |
|------|------|------|
| `Executor` 输出 | `sync.Mutex` | 输出 map 的并发安全写入 |
| `Executor` readyCount | `sync.Mutex` | 安全递增 + 阈值检查 |
| `Agent.WithCache` | `sync.Map` | 无锁并发读取，安全写入 |
| `observe.Metrics` | `sync.RWMutex` | 多读单写 |
| `observe.Tracer` | `sync.Mutex` | 安全的 Span 记录 |
| `sseHub` | 基于 channel（单 goroutine 事件循环） | 无需锁 |
| `memory.BufferStore` | `sync.RWMutex` | 安全的并发读写 |
| `memory.WindowStore` | `sync.RWMutex` | 安全的并发读写 |
| `memory.SummaryStore` | `sync.RWMutex` | 安全的并发读写（摘要在锁内执行） |
| `rag.InMemoryStore` | `sync.RWMutex` | 安全的并发添加/搜索 |
| `conv.Channel` | `sync.Mutex` | 安全的并发发送/接收 |

### Goroutine 生命周期

```
Executor.Run()
    │
    ├── goroutine: 节点 "A"（源节点，立即触发）
    ├── goroutine: 节点 "B"（源节点，立即触发）
    ├── goroutine: 节点 "C"（等待 A 和 B，通过触发通道）
    └── goroutine: 节点 "D"（等待 C）
    
    wg.Wait() 阻塞直到所有 goroutine 退出
    
    错误路径: 任何错误 → cancel() → 所有 goroutine 检查 ctx.Err() → 退出
```

---

## 12. 依赖策略

```
核心包 (pkg/agent, pkg/waggle, pkg/observe, pkg/web, pkg/memory, pkg/output, pkg/prompt, pkg/stream, pkg/rag, pkg/conv):
    └── 仅 Go 标准库
        ├── context, sync, time
        ├── net/http, encoding/json
        ├── log/slog
        ├── embed（用于 Web 静态文件）
        └── fmt, errors, strings, ...

CLI (cmd/waggle/commands):
    └── gopkg.in/yaml.v3（YAML 解析 —— 唯一外部依赖）
```

这意味着**核心引擎可以零传递依赖地嵌入任何 Go 应用**。YAML 依赖被隔离在 CLI 中，是可选的。

---

## 13. 记忆层 — `pkg/memory`

### 目的与设计原理

记忆包为 LLM Agent 提供对话记忆能力，支持多轮交互，让历史消息影响后续响应。记忆层与 LLM 包解耦，以避免循环导入，并允许多种记忆策略独立组合。

`Message` 类型故意使用 `string` 表示 `Role`（而非 `llm.Role`），以打破对 `pkg/llm` 包的依赖，保持记忆层自包含，可被任何包导入。

### 核心接口与类型

```go
// Store 是对话历史的核心记忆接口。
type Store interface {
    // Add 向对话历史追加一条消息。
    Add(ctx context.Context, msg Message) error
    // Messages 返回当前对话历史。
    Messages(ctx context.Context) ([]Message, error)
    // Clear 清除存储中的所有消息。
    Clear(ctx context.Context) error
}

// Message 表示一条对话消息。
type Message struct {
    Role    string // "system", "user", "assistant" — 使用 string 避免导入 llm
    Content string
}
```

#### BufferStore — 无界历史

`BufferStore` 是最简单的实现：一个由 `sync.RWMutex` 保护的追加式消息切片。适用于短对话或由调用方在外部管理截断的场景。

```go
type BufferStore struct {
    mu       sync.RWMutex
    messages []Message
}
```

#### WindowStore — 滑动窗口

`WindowStore` 保留最近的 `n` 条消息，同时始终将置顶的 system 消息保留在前端。当超过窗口限制时，最老的非 system 消息会被丢弃。

```go
type WindowStore struct {
    mu       sync.RWMutex
    messages []Message
    maxSize  int
}
```

#### SummaryStore — 摘要压缩

`SummaryStore` 监控消息数量，当超过阈值时触发 `Summarizer` 函数。摘要器将旧消息压缩为单条摘要消息，使对话保持在上下文限制内。

```go
// Summarizer 将一组消息压缩为一条摘要消息。
type Summarizer func(ctx context.Context, messages []Message) (Message, error)

type SummaryStore struct {
    mu         sync.RWMutex
    messages   []Message
    threshold  int
    summarizer Summarizer
}
```

### 与现有包的集成

记忆通过 `llm.WithMemory(store)` 选项与 LLM 层集成。当配置了记忆时，Agent 会在每次调用前自动加载对话历史，并在每次调用后追加用户输入和助手响应。

```go
agent := llm.NewLLMAgent[string]("chatbot", provider,
    llm.WithSystemPrompt("You are a helpful assistant."),
    llm.WithMemory(memory.NewWindowStore(20)),
)
```

### 线程安全

所有 Store 实现均使用 `sync.RWMutex` 保证安全的并发访问。`Add` 和 `Clear` 获取写锁；`Messages` 获取读锁。`SummaryStore` 在写锁下执行摘要操作，防止并发读取观察到部分压缩的历史。

---

## 14. 结构化输出 — `pkg/output`

### 目的与设计原理

输出包使 LLM Agent 能够返回有类型的 Go 结构体，而非原始字符串。这弥合了非结构化 LLM 文本与类型安全 `Agent[I, O]` 管道之间的鸿沟。该包使用三级提取策略，以最大化兼容不同 JSON 格式的 LLM。

### 核心接口与类型

```go
// Parser[O] 从原始 LLM 输出中提取有类型的值。
type Parser[O any] interface {
    // Parse 尝试从原始字符串中提取类型为 O 的值。
    Parse(raw string) (O, error)
    // FormatInstruction 返回追加到提示词的字符串，
    // 指导 LLM 按预期格式输出。
    FormatInstruction() string
}
```

#### JSONParser — 三级提取

`JSONParser[O]` 使用三级策略尝试将 LLM 输出解析为 JSON：

1. **直接解析：** 尝试对整个响应执行 `json.Unmarshal`。
2. **代码块提取：** 查找 `` ```json ... ``` `` 围栏代码块并解析其内容。
3. **括号匹配：** 查找最外层的 `{...}` 或 `[...]` 并解析该子串。

这优雅地处理了 LLM 将 JSON 包裹在 markdown 中、添加前导文本或包含尾部注释的情况。

```go
type JSONParser[O any] struct{}

func (p JSONParser[O]) Parse(raw string) (O, error)
func (p JSONParser[O]) FormatInstruction() string
```

#### SchemaFor — 基于反射的 JSON Schema

`SchemaFor[O]()` 从 Go 结构体的类型信息和 struct tag 生成 JSON Schema 字符串。该 Schema 包含在提示词指令中，让 LLM 准确知道应产生哪些字段和类型。

```go
func SchemaFor[O any]() string
```

#### NewStructuredAgent — 带解析和重试的 Agent

`NewStructuredAgent` 将 LLM Agent 和 `Parser[O]` 组合成 `Agent[I, O]`。如果解析失败，它会重试 LLM 调用（最多可配置次数），并在增强的提示词中包含解析错误，给 LLM 修正输出的机会。

```go
func NewStructuredAgent[I, O any](name string, llmAgent agent.Agent[I, string], parser Parser[O]) agent.Agent[I, O]
```

### 与现有包的集成

`NewStructuredAgent` 返回标准的 `Agent[I, O]`，因此可与 `Chain`、`Parallel`、`WithRetry` 及所有其他 Agent 原语完全组合。它将非类型化的 LLM 世界与类型化的管道世界桥接起来。

```go
type Review struct {
    Score    int    `json:"score"`
    Summary  string `json:"summary"`
    Issues   []string `json:"issues"`
}

reviewer := output.NewStructuredAgent[string, Review]("reviewer", llmAgent, output.JSONParser[Review]{})
// reviewer 是 Agent[string, Review] — 完全可组合
pipeline := agent.Chain2(fetcher, reviewer)
```

### 线程安全

`JSONParser` 和 `SchemaFor` 是无状态的，可安全并发使用。`NewStructuredAgent` 将并发安全性委托给底层的 LLM Agent。

---

## 15. 提示词模板 — `pkg/prompt`

### 目的与设计原理

提示词包提供轻量级、零依赖的模板系统，用于构造 LLM 提示词。它避免使用外部模板引擎（如 `text/template`），采用更简单的 `{{var}}` 占位符语法，更易读、更不容易出错，且符合零依赖的设计理念。

### 核心接口与类型

#### Template — 不可变变量替换

`Template` 使用 `{{var}}` 占位符，遵循不可变设计：`WithVar` 返回新的 `Template` 而非修改原始对象，使模板可安全地在 goroutine 间共享和重用。

```go
type Template struct {
    raw  string
    vars map[string]string
}

func New(raw string) Template
func (t Template) WithVar(name, value string) Template
func (t Template) Render() string
```

#### FewShotBuilder — 基于示例的提示词

`FewShotBuilder` 构建 few-shot 提示词，包含系统指令、一组输入/输出示例和最终输入。这种模式有效引导 LLM 产生一致的输出格式。

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

#### AsPromptFunc — LLMAgent 兼容

`AsPromptFunc()` 将 `Template` 转换为与 `llm.NewLLMAgent` 兼容的 `PromptFunc`，桥接提示词层和 LLM 层。

```go
func (t Template) AsPromptFunc() func(ctx context.Context, input string) ([]llm.Message, error)
```

### 与现有包的集成

模板通过转换为 `NewLLMAgent` 接受的 `PromptFunc` 值与 LLM 层集成。也可独立用于任何字符串格式化需求。

```go
tmpl := prompt.New("Analyze {{language}} code:\n{{code}}")
agent := llm.NewLLMAgent[string]("analyzer", provider,
    llm.WithPromptFunc(tmpl.WithVar("language", "Go").AsPromptFunc()),
)
```

### 线程安全

`Template` 是不可变的 — `WithVar` 返回新实例。这使模板天然支持并发使用，无需同步。

---

## 16. 可观测流水线 — `pkg/stream`

### 目的与设计原理

流式包为 Agent 管道添加可观测性，在每个 Agent 边界发射结构化的 `Step` 事件。这支持实时进度追踪、调试，以及通过 SSE 与 Web 可视化层集成。

### 核心接口与类型

#### Step — 管道事件

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

#### Observer — 事件接收器

```go
// Observer 接收管道步骤事件。
type Observer interface {
    OnStep(step Step)
}

// ObserverFunc 是 Observer 的函数适配器。
type ObserverFunc func(Step)

func (f ObserverFunc) OnStep(step Step) { f(step) }
```

#### 可观测 Chain

`ObservableChain2` 和 `ObservableChain3` 包装 `agent.Chain2`/`Chain3`，在每个 Agent 边界发射步骤事件：

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

#### MultiObserver 和 Collector

`MultiObserver` 将步骤扇出到多个观察者。`Collector` 将步骤累积到切片中，用于测试和检查。

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

### 与现有包的集成

可观测 Chain 是标准 Chain 的直接替代品 — 它们返回相同的 `Agent[I, O]` 类型。步骤可以转发到 `web/sse.go` Hub，用于仪表盘的实时 UI 更新。

```go
obs := stream.ObserverFunc(func(s stream.Step) {
    sseHub.Broadcast(s) // 转发到 Web 仪表盘
})
pipeline := stream.ObservableChain2(fetcher, parser, obs)
```

### 线程安全

`Collector` 使用 `sync.Mutex` 保护 `Steps` 切片，支持安全的并发累积。`ObserverFunc` 和 `MultiObserver` 将线程安全性委托给底层的 Observer 实现。

---

## 17. RAG 管道 — `pkg/rag`

### 目的与设计原理

RAG（检索增强生成）包提供完整的管道，用于将 LLM 响应基于外部知识。它定义了嵌入、向量存储和文本分割的接口，并提供零外部依赖的内存实现。

### 核心接口与类型

#### Embedder — 文本转向量

```go
// Embedder 将文本转换为向量嵌入。
type Embedder interface {
    // Embed 将一批文本字符串转换为嵌入向量。
    Embed(ctx context.Context, texts []string) ([][]float64, error)
    // Dimensions 返回嵌入向量的维度。
    Dimensions() int
}
```

#### VectorStore — 相似度搜索

```go
// Document 表示带有嵌入和元数据的文本块。
type Document struct {
    ID        string
    Content   string
    Embedding []float64
    Metadata  map[string]string
}

// VectorStore 持久化和搜索文档嵌入。
type VectorStore interface {
    // Add 存储带有嵌入的文档。
    Add(ctx context.Context, docs []Document) error
    // Search 查找与查询向量最相似的 Top-K 文档。
    Search(ctx context.Context, vector []float64, topK int) ([]Document, error)
}
```

#### InMemoryStore — 零依赖向量搜索

`InMemoryStore` 使用暴力余弦相似度搜索实现 `VectorStore`。由 `sync.RWMutex` 保护以支持安全的并发访问，无需外部向量数据库。

```go
type InMemoryStore struct {
    mu   sync.RWMutex
    docs []Document
}
```

#### Splitter — 文本分块

```go
// Splitter 将文本分割为适合嵌入的块。
type Splitter interface {
    Split(text string) []string
}

// TokenSplitter 将文本分割为约 N 个 token 的块。
type TokenSplitter struct {
    ChunkSize    int
    ChunkOverlap int
}

// ParagraphSplitter 按段落边界分割文本。
type ParagraphSplitter struct{}
```

#### NewPipeline — 端到端 RAG

`NewPipeline` 将完整的 RAG 流程组合为单个 `Agent[string, string]`：

```
查询 ──> Embed(query) ──> VectorStore.Search(topK) ──> 构建上下文提示词 ──> LLM ──> 回答
```

```go
func NewPipeline(
    embedder Embedder,
    store VectorStore,
    llmAgent agent.Agent[string, string],
    topK int,
) agent.Agent[string, string]
```

### 与现有包的集成

`NewPipeline` 返回标准的 `Agent[string, string]`，因此可与 Chain、装饰器和编排模式组合。LLM Agent 参数可以是任何接受字符串提示词的 Agent，包括使用 `pkg/output` 构建的结构化响应 Agent。

```go
ragAgent := rag.NewPipeline(embedder, vectorStore, llmAgent, 5)
pipeline := agent.Chain2(inputProcessor, ragAgent)
reliable := agent.WithRetry(pipeline, agent.WithMaxAttempts(3))
```

### 线程安全

`InMemoryStore` 使用 `sync.RWMutex` — `Add` 获取写锁，`Search` 获取读锁。这允许并发搜索同时序列化写入。管道本身除存储外无状态，将并发安全性委托给组件 Agent。

---

## 18. 多 Agent 对话 — `pkg/conv`

### 目的与设计原理

对话包支持多 Agent 对话，多个参与者在结构化的轮次协议中交换消息。这适用于辩论式推理、协作问题解决和 Agent 间协商。`Moderator` 通过可配置的轮次、终止条件和发言顺序来编排对话流程。

### 核心接口与类型

#### Envelope — 消息容器

```go
// Envelope 用路由元数据包装消息。
type Envelope struct {
    From    string // 发送方参与者名称
    To      string // 接收方参与者名称（"" 表示广播）
    Content string
    Round   int
}
```

#### Channel — 线程安全消息队列

`Channel` 是线程安全的 FIFO 队列，用于在参与者之间传递信封：

```go
type Channel struct {
    mu       sync.Mutex
    messages []Envelope
}

func (c *Channel) Send(env Envelope)
func (c *Channel) Receive() ([]Envelope, bool)
func (c *Channel) Clear()
```

#### Participant — 对话成员

```go
// Participant 是可参与多 Agent 对话的实体。
type Participant interface {
    // Name 返回参与者的唯一标识符。
    Name() string
    // Respond 根据对话历史生成响应。
    Respond(ctx context.Context, history []Envelope) (Envelope, error)
}

// FuncParticipant 将函数适配为 Participant。
type FuncParticipant struct {
    name string
    fn   func(ctx context.Context, history []Envelope) (Envelope, error)
}
```

#### Moderator — 轮次编排器

`Moderator` 管理对话流程：

```go
type Moderator struct {
    participants []Participant
    maxRounds    int
    turnOrder    []string          // 自定义发言顺序（可选）
    termination  func([]Envelope) bool // 提前终止条件
}

func NewModerator(participants []Participant, opts ...ModeratorOption) *Moderator
func (m *Moderator) Run(ctx context.Context, topic string) ([]Envelope, error)
```

**对话流程：**

```
话题 ──> 第 1 轮: 参与者 A 响应
                   参与者 B 响应
          第 2 轮: 参与者 A 响应
                   参与者 B 响应
          ...
          达到终止条件 或 达到最大轮次
          ──> 返回完整对话历史
```

#### AsAgent — Moderator 作为 Agent

`AsAgent()` 将 `Moderator` 转换为标准的 `Agent[string, []Envelope]`，使对话可以嵌入更大的管道。

```go
func (m *Moderator) AsAgent() agent.Agent[string, []Envelope]
```

### 与现有包的集成

通过 `AsAgent()`，多 Agent 对话成为常规管道节点。参与者内部可以使用 `pkg/llm` Agent、`pkg/memory` 存储或 `pkg/rag` 管道，支持丰富的对话工作流。

```go
debater1 := conv.FuncParticipant("optimist", optimistFn)
debater2 := conv.FuncParticipant("critic", criticFn)
mod := conv.NewModerator([]conv.Participant{debater1, debater2},
    conv.WithMaxRounds(5),
    conv.WithTermination(consensusReached),
)
// 在管道中作为 Agent 使用
debate := mod.AsAgent() // Agent[string, []Envelope]
pipeline := agent.Chain2(topicGenerator, agent.Erase(debate))
```

### 线程安全

`Channel` 使用 `sync.Mutex` 保证安全的并发发送和接收操作。`Moderator` 本身按顺序驱动对话（每轮一次一个参与者），因此不需要额外同步。参与者负责自身内部的线程安全。

---

*Waggle v0.6.0 — Apache 2.0 License*
