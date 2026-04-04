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
核心包 (pkg/agent, pkg/waggle, pkg/observe, pkg/web):
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

*Waggle v0.1.0 — Apache 2.0 License*
