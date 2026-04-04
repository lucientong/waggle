# Waggle

> 轻量级 Go AI Agent 编排引擎。

[![Go Version](https://img.shields.io/badge/go-1.26+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/lucientong/waggle)](https://goreportcard.com/report/github.com/lucientong/waggle)

**Waggle** 的名字来源于蜜蜂的「摇摆舞」（Waggle Dance）——蜜蜂通过摇摆舞告诉同伴花蜜的方向和距离。在 Waggle 中，每个 Agent 是一只蜜蜂，goroutine 是它的翅膀，channel 是蜂群的通信舞步。

## 为什么选择 Waggle？

现有的 AI Agent 编排框架（LangChain、CrewAI、AutoGen、LangGraph）全部是 Python 实现，存在五大核心问题，Waggle 逐一解决：

| 问题 | Python 框架 | Waggle |
|---|---|---|
| **性能** | GIL 限制，无法真正并发 | Goroutine-per-agent，轻松启动数千并发 Agent |
| **类型安全** | `dict/str/Any`，运行时才报错 | `Agent[I, O any]` 泛型，编译期类型检查 |
| **编排模式** | 线性流程或简单 DAG | 6 种内置模式：Chain、Parallel、Race、Vote、Router、Loop |
| **部署** | 依赖 Python 环境，需要 Docker | 作为 Go 库嵌入，单一二进制，零外部依赖 |
| **可观测性** | 黑盒执行，难以调试 | 内置 DAG 可视化面板、OpenTelemetry 追踪、结构化日志 |

## 安装

```bash
go get github.com/lucientong/waggle
```

## 快速上手

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

    // 从普通函数创建 Agent
    upper := agent.Func[string, string]("upper", func(ctx context.Context, s string) (string, error) {
        return strings.ToUpper(s), nil
    })

    exclaim := agent.Func[string, string]("exclaim", func(ctx context.Context, s string) (string, error) {
        return s + "!", nil
    })

    // 串联成管道：upper -> exclaim
    pipeline := agent.Chain2(upper, exclaim)

    result, err := pipeline.Run(ctx, "hello waggle")
    if err != nil {
        panic(err)
    }
    fmt.Println(result) // 输出：HELLO WAGGLE!
}
```

## 核心概念

### Agent

带编译期类型安全的最小执行单元：

```go
type Agent[I, O any] interface {
    Name() string
    Run(ctx context.Context, input I) (O, error)
}
```

### FuncAgent

从任意函数创建 Agent：

```go
fetcher := agent.Func[string, []byte]("fetcher", func(ctx context.Context, url string) ([]byte, error) {
    // 获取 URL 内容
})

summarizer := agent.Func[[]byte, string]("summarizer", func(ctx context.Context, data []byte) (string, error) {
    // 用 LLM 摘要内容
})
```

### Chain — 串行管道

将 Agent 串联，前一个输出作为后一个输入：

```go
// Chain2：Agent[A,B] + Agent[B,C] => Agent[A,C]
pipeline := agent.Chain2(fetcher, summarizer)

// Chain3：三个 Agent
pipeline := agent.Chain3(fetcher, summarizer, reviewer)

// 还支持 Chain4、Chain5
```

### 装饰器 — 包装器

为任意 Agent 添加横切关注点：

```go
// 重试（指数退避 + jitter）
reliable := agent.WithRetry(myAgent,
    agent.WithMaxAttempts(3),
    agent.WithBaseDelay(100*time.Millisecond),
)

// 超时控制
bounded := agent.WithTimeout(myAgent, 5*time.Second)

// 缓存（相同输入返回缓存结果）
cached := agent.WithCache(myAgent, func(input string) string {
    return input // 用输入作为缓存 key
})

// 组合装饰器
pipeline := agent.Chain2(
    agent.WithRetry(agent.WithTimeout(fetcher, 3*time.Second), agent.WithMaxAttempts(3)),
    agent.WithCache(summarizer, func(data []byte) string { return string(data) }),
)
```

### Waggle — 编排器（Phase 2）

```go
w := waggle.New()
w.Chain(fetcher, summarizer, reviewer)
result, err := w.Run(ctx, "https://example.com")
```

### 编排模式（Phase 3）

```go
// Parallel：并行扇出，收集所有结果
results := waggle.Parallel(agent1, agent2, agent3)

// Race：最快返回的获胜
fastest := waggle.Race(primaryAgent, backupAgent)

// Vote：多数一致的结果获胜
decision := waggle.Vote(judge1, judge2, judge3)

// Router：条件路由到不同分支
routed := waggle.Router(classifyFn, map[string]Agent{
    "code":    codeReviewer,
    "docs":    docReviewer,
    "default": generalReviewer,
})

// Loop：循环直到满足条件
refined := waggle.Loop(improveAgent, func(result string) bool {
    return qualityScore(result) >= 0.9
})
```

## 项目结构

```
waggle/
├── pkg/
│   ├── agent/      # Agent 接口、FuncAgent、Chain、装饰器（Retry/Timeout/Cache）
│   ├── waggle/     # 核心编排器、DAG、执行器、编排模式
│   ├── llm/        # LLM Provider（OpenAI、Anthropic、Ollama）+ LLM/Tool Agent
│   ├── observe/    # 事件、追踪、指标、结构化日志
│   └── web/        # 内嵌 Web 可视化面板
├── cmd/waggle/     # CLI：run / serve / validate / dot
├── web/            # 前端源码（D3.js DAG 可视化）
└── examples/       # code_review / research / customer_support
```

## 开发阶段

- [x] **Phase 1** — Agent 接口、FuncAgent、Chain、Retry/Timeout/Cache 包装器
- [x] **Phase 2** — DAG 编排器、拓扑执行器
- [x] **Phase 3** — Parallel / Race / Vote / Router / Loop 编排模式
- [x] **Phase 4** — LLM Provider（OpenAI、Anthropic、Ollama）+ LLM/Tool Agent
- [x] **Phase 5** — 可观测性（事件、追踪、指标、slog）
- [x] **Phase 6** — 内嵌 Web DAG 可视化面板
- [x] **Phase 7** — CLI（`waggle run / serve / validate / dot`）
- [x] **Phase 8** — 实战示例（代码审查、调研助手、智能客服）

## 环境要求

- Go 1.26+

## 开源协议

Apache 2.0 — 详见 [LICENSE](LICENSE)
