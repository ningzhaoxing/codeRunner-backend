# 代码学习 Agent 功能设计文档

**日期**：2026-03-28
**状态**：草稿（v2 — 调整为独立微服务架构）
**涉及模块**：代码学习 Agent（独立微服务）

---

## 一、背景与目标

CodeRunner 当前定位是博客平台的分布式代码执行引擎，负责接收代码执行请求、调度 client 节点、在 Docker 沙箱中运行代码并异步回调结果。

代码学习 Agent 的职责远超"运行代码"——它需要理解整篇文章的内容与上下文、与用户多轮对话、调用 AI 模型进行推理。将其嵌入 CodeRunner 会使两个职责不清晰的系统相互耦合。因此，**代码学习 Agent 作为独立微服务存在**，博客前端直接与之通信，Agent 在需要运行代码时调用 CodeRunner 现有的 `Execute` gRPC 接口。

**CodeRunner 本身零改动。**

---

## 二、产品需求

### 代码学习 Agent 用户场景

1. **调试**：用户运行代码报错，向 Agent 提问 → Agent 分析错误并给出修复建议 → 用户确认后运行修复版本
2. **解释**：用户读到看不懂的代码 → 问 Agent "这段 goroutine 为什么用 WaitGroup？" → Agent 结合文章上下文解释
3. **测试**：用户实现了一个函数 → 让 Agent 生成边界测试用例 → 用户确认后运行验证

### 交互模式

- **对话式**：用户用自然语言自由提问，Agent 按需响应
- **上下文范围**：博客文章级别——Agent 了解整篇文章内容和所有代码块
- **执行确认**：Agent 可建议修改代码并触发执行，但必须经用户确认后才真正运行
- **流式输出**：Agent 回复通过 SSE 直接推送给博客前端，无需中转

### 功能边界（不做的事）

- 不支持跨文章的历史记忆（每篇文章独立会话，TTL 1 小时）
- Agent 不能直接修改用户代码，只能"提议"，用户手动确认
- 不提供代码自动补全

---

## 三、技术方案

### 3.1 整体架构

```text
博客前端
  ├── POST /chat             → 代码学习 Agent 微服务
  │     ← SSE 流式回复      ←
  └── POST /confirm          → 代码学习 Agent 微服务
        ← JSON 响应（已接受）←

代码学习 Agent 微服务
  ├── 维护会话状态（session_id → AgentSession）
  ├── 调用 AI Provider（Claude / OpenAI / 可配置）
  │     Tools: explain_code / debug_code / generate_tests / propose_execution
  ├── 需要运行代码时 → gRPC Execute → CodeRunner（现有接口，不变）
  │                                      │ WebSocket → client 节点 → Docker 沙箱
  └── Agent 内部 callback endpoint        ← HTTP 回调结果

CodeRunner（完全不改动）
```

**数据流说明**：

| 操作 | 调用方 | 被调用方 | 协议 |
|------|--------|---------|------|
| 用户发消息 | 博客前端 | Agent 微服务 `POST /chat` | HTTP + SSE |
| 用户确认运行修复代码 | 博客前端 | Agent 微服务 `POST /confirm` | HTTP |
| Agent 触发代码执行 | Agent 微服务 | CodeRunner `Execute` | gRPC |
| 执行结果回传 | CodeRunner | Agent 微服务 `POST /internal/callback` | HTTP 回调 |
| 用户普通运行代码（不经 Agent） | 博客后端 | CodeRunner `Execute` | gRPC（不变） |

---

### 3.2 CAR 框架设计

本方案使用 CAR（Control / Agency / Runtime）框架组织 Agent 的 Harness 层设计，参考 *Harness Engineering for Language Agents*（Preprints.org, 2026.03）。

> 可靠的 Agent 行为是被设计出来的，而不是从模型中涌现出来的。

#### Control — 谁在控制 Agent 的行为边界

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 执行预算 | 每次对话最大 **10 步** ReAct 循环 | 防止 Agent 陷入无限推理，控制 token 消耗 |
| 人工门控 | `propose_execution` 返回 `ProposedExecution` 结构，执行必须等用户通过 `POST /confirm` 确认 | 代码执行是不可逆副作用，高风险操作需人工介入 |
| 并行工具调用 | **禁用**，强制顺序调用 | 防止 Agent 并发调用工具时猜测上下文，产生幻觉 |
| 路由策略 | 解释/问答类 → 直接 AI 回答；涉及执行/调试 → Tool Calling 路径 | 减少不必要的工具开销，加快简单问题响应速度 |
| 停止控制 | SSE 连接断开时立即取消 Agent context | 避免用户已离开但 Agent 继续消耗 API 额度 |

#### Agency — Agent 能看到什么、能做什么

**工具列表（4 个，少而精）**：

| Tool | 触发场景 | 输入 | 输出给 AI |
|------|---------|------|----------|
| `explain_code` | "这段代码是什么意思" | `block_id` | 代码内容 + 语言 + 该 block 最近一次执行输出（无则为空） |
| `debug_code` | "为什么报错" / 代码运行失败后 | `block_id`, `error_message` | 代码内容 + 报错信息 + 最近执行输出 |
| `generate_tests` | "帮我生成测试" | `block_id` | 代码内容 + 语言 |
| `propose_execution` | Agent 生成修复代码后 | `new_code`, `language` | 在 AgentSession 写入 Proposal，向前端返回 `ProposedExecution` 结构 |

**执行结果的存储与反馈**：

Agent 调用 CodeRunner `Execute` 时，将如下 URL 作为 `callBackUrl`：

```text
http://localhost:{agent_port}/internal/callback?session_id={sid}&proposal_id={pid}
```

执行完成后，CodeRunner 将结果 POST 到此 URL。内部 handler 从 query 参数读取 `session_id` 定位 session，将结果写入 `AgentSession.ExecutionResults["proposal:{pid}"]`，供后续 Tool 调用读取。同时通过对应 SSE 连接向前端推送执行完成通知。

**行动空间隔离**：

- Agent 只能通过 `block_id` 访问**当前 `article_id` 下的代码块**
- 无效的 `block_id` 返回工具错误，不暴露其他文章数据

**两阶段上下文注入**：

- 首次调用 `POST /chat`（`article_ctx.article_id` 非空）：服务端创建新 session，写入文章上下文；UUID v4 `session_id` 通过响应第一个 SSE chunk 返回
- 后续调用携带 `session_id`，`article_ctx` 省略
- 若 `session_id` 存在且传入非空 `article_ctx`：覆盖 session 中的文章上下文（支持文章更新场景）

**System Prompt 中的工具说明**：

```text
Tools available:
- explain_code: Use when user asks what code does or how it works.
  NOT for: fixing bugs, generating new code.
- debug_code: Use when user reports an error or code produced unexpected output.
  NOT for: explaining working code.
- generate_tests: Use when user asks for test cases or edge case coverage.
  NOT for: running tests (use propose_execution after generating).
- propose_execution: Use when you have a concrete code change ready to run.
  NOT for: speculative or incomplete code.
```

#### Runtime — 系统怎么在长时间运行中保持连贯

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 会话持久化 | 内存 `sync.Map`，TTL 1 小时 | 用户读完一篇文章足够；无需引入 Redis |
| 上下文压缩 | 对话历史（不含文章上下文）超过 **8000 tokens** 时，通过额外 LLM 调用生成摘要替换早期历史；文章上下文始终保留 | 防止历史消息溢出，保持文章感知 |
| 断连处理 | SSE 写入失败 → 立即 cancel Agent context → 停止 LLM 调用 | 避免资源浪费 |
| 可观测性 | Prometheus + 请求链路 ID，新增指标（见下） | 独立服务独立监控 |
| 失败恢复 | LLM 调用失败（超时/限流）→ SSE 推送错误 chunk，保留 session，用户可重试 | 不回滚整个对话，只回退当前轮次 |

**Prometheus 指标**：

| 指标名 | 类型 | 标签 | 含义 |
|--------|------|------|------|
| `agent_chat_duration_seconds` | Histogram | `status`(success/error) | 单次 /chat 请求耗时 |
| `agent_tool_calls_total` | Counter | `tool_name`, `status`(success/error) | 各 Tool 调用次数 |
| `agent_sessions_active` | Gauge | — | 当前活跃 session 数 |

---

### 3.3 Session 生命周期与 ProposedExecution 状态

#### Session ID 与 Proposal ID 生成

- **session_id**：首次 `POST /chat` 时由服务端生成 UUID v4，通过第一个 SSE chunk 返回
- **proposal_id**：`propose_execution` Tool 执行时由服务端生成 UUID v4，随 `ProposedExecution` 返回给前端
- Proposal 不设独立 TTL，生命周期与所属 session 一致

#### AgentSession 数据结构

```go
type AgentSession struct {
    ID               string
    ArticleID        string
    ArticleContext   ArticleContext
    Messages         []Message              // 对话历史（可被压缩）
    ExecutionResults map[string]ExecResult  // "proposal:{pid}" → 执行结果
    Proposals        map[string]Proposal    // proposal_id → 执行提议
    SSEChan          chan SSEEvent          // 当前活跃 SSE 连接的推送通道
    CreatedAt        time.Time
    LastActiveAt     time.Time
    TTL              time.Duration          // 默认 1 小时
}

type Proposal struct {
    ID        string
    Code      string
    Language  string
    CreatedAt time.Time
    Confirmed bool  // 防止重复确认
}
```

#### ProposedExecution 状态管理

- `propose_execution` Tool 被调用时：写入 `AgentSession.Proposals`，`Confirmed = false`
- `POST /confirm` 到达时：
  1. 检查 `session_id` 是否存在（否 → HTTP 404）
  2. 检查 `proposal_id` 是否存在（否 → HTTP 404）
  3. 检查 `Confirmed` 是否已为 `true`（是 → HTTP 409 Conflict，防重复提交）
  4. 标记 `Confirmed = true`，向 CodeRunner 发起 gRPC `Execute` 调用

#### 对 CodeRunner 的鉴权

Agent 微服务调用 CodeRunner gRPC 时需携带 JWT Token。Agent 在配置中保存一个预生成的 service-level JWT（由运维通过现有 `/generateToken` 接口签发），用于所有对 CodeRunner 的调用。Token 过期时通过配置刷新。

---

### 3.4 HTTP API 定义

Agent 微服务对博客前端暴露以下 HTTP 接口：

#### `POST /chat`

**请求体（JSON）**：

```json
{
  "session_id": "",           // 首次为空，后续填写
  "user_message": "为什么这段代码报错？",
  "article_ctx": {            // 首次调用时传入，后续省略
    "article_id": "article-123",
    "article_content": "...", // 文章 Markdown 正文
    "code_blocks": [
      { "block_id": "block-1", "language": "go", "code": "..." }
    ]
  }
}
```

**响应**：`Content-Type: text/event-stream`（SSE）

```text
data: {"type":"session","session_id":"uuid-v4"}        // 首次响应，携带 session_id

data: {"type":"chunk","content":"这段代码报错的原因是"}  // 文字流式 chunk

data: {"type":"chunk","content":"..."}

data: {"type":"proposal","proposal_id":"uuid-v4","code":"...","language":"go","description":"修复了数组越界"}

data: {"type":"done"}
```

#### `POST /confirm`

用户确认执行 Agent 提议的代码。

**请求体（JSON）**：

```json
{
  "session_id": "uuid-v4",
  "proposal_id": "uuid-v4"
}
```

**响应（JSON）**：

```json
{ "request_id": "uuid-v4", "status": "accepted" }
```

执行结果通过 SSE 连接异步推送：

```text
data: {"type":"execution_result","proposal_id":"uuid-v4","stdout":"...","stderr":"","exit_code":0}
```

#### `POST /internal/callback`（内部接口，不对外暴露）

供 CodeRunner 回传执行结果，不对博客前端开放。

**Query 参数**：`session_id`, `proposal_id`

**请求体**：与 CodeRunner 现有回调格式一致。

---

### 3.5 项目结构

独立微服务，可在 coderunner repo 中新增 `cmd/agent/` 入口，也可作为独立仓库维护。推荐先放在同一 repo：

```text
cmd/agent/
  └── main.go                  # 入口，启动 Agent HTTP 服务

internal/agent/                # 仅被 agent 使用的内部包
  ├── session/
  │   └── store.go             # AgentSession + sync.Map 存储 + TTL 清理
  ├── handler/
  │   ├── chat.go              # POST /chat SSE handler
  │   ├── confirm.go           # POST /confirm handler
  │   └── callback.go          # POST /internal/callback handler
  ├── service/
  │   └── agent.go             # 编排：AI Provider → Tool 执行 → Execute gRPC
  ├── tools/
  │   └── tools.go             # 4 个 Tool 的实现
  └── ai/
      ├── provider.go          # AIProvider 接口 + ChatChunk 类型
      ├── claude/claude.go     # Claude 实现
      └── openai/openai.go     # OpenAI 实现

configs/agent.yaml             # Agent 微服务独立配置文件
```

---

### 3.6 AI Provider 抽象

```go
type ChatChunk struct {
    Content      string
    ToolCall     *ToolCall
    FinishReason string  // "stop" | "tool_calls" | "max_steps" | "error"
    Err          error
}

type ToolCall struct {
    Name  string
    Input json.RawMessage
}

type AIProvider interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
}
```

配置文件（`configs/agent.yaml`）：

```yaml
server:
  port: 8081
  internal_base_url: "http://localhost:8081"  # 用于构造 callback URL

coderunner:
  grpc_addr: "coderunner-server:50011"
  service_token: ${CODERUNNER_SERVICE_TOKEN}  # 预签发的 service-level JWT

agent:
  provider: claude
  max_steps: 10
  context_token_limit: 8000
  session_ttl: 3600
  claude:
    api_key: ${CLAUDE_API_KEY}
    model: claude-opus-4-6
  openai:
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
```

---

## 四、不在本期范围内

- 跨文章的持久化对话历史
- Agent 主动推送通知（执行完成后 Agent 自动解释结果）
- Agent 微服务的水平扩展（当前内存 session 不支持多实例共享，需引入 Redis 后支持）
- 博客前端的具体 UI 实现

---

## 五、HARNESSCARD（参考）

遵循 *Harness Engineering for Language Agents* 建议，记录本 Harness 的关键设计参数：

| 维度 | 参数 | 值 |
|------|-----|----|
| Control | 最大执行步数 | 10 |
| Control | 人工门控 | POST /confirm 确认后才执行 |
| Control | 并行工具调用 | 禁用 |
| Agency | 工具数量 | 4 |
| Agency | 行动空间隔离 | article_id 级别过滤 |
| Agency | 上下文注入策略 | 首次全量，后续通过 session_id 复用 |
| Runtime | 会话存储 | 内存 sync.Map，TTL 1h |
| Runtime | 上下文压缩阈值 | 8000 tokens（历史消息，不含文章上下文） |
| Runtime | 压缩方式 | 额外 LLM 调用生成摘要替换早期历史 |
| Runtime | 断连处理 | SSE 写入失败 → context cancel |
| Runtime | 可观测性 | Prometheus（3 个指标）+ 请求链路 ID |
