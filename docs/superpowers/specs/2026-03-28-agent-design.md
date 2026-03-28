# 代码学习 Agent 功能设计文档

**日期**：2026-03-28
**状态**：草稿（v3）
**涉及模块**：代码学习 Agent（独立微服务）

---

## 一、背景与目标

CodeRunner 当前定位是博客平台的分布式代码执行引擎，负责接收代码执行请求、调度 client 节点、在 Docker 沙箱中运行代码并通过 HTTP 回调返回结果。

代码学习 Agent 的职责远超"运行代码"——它需要理解整篇文章内容、与用户多轮对话、调用 AI 模型推理。将其嵌入 CodeRunner 会导致职责不清晰的耦合。因此，**代码学习 Agent 作为独立微服务存在**，博客前端直接与之通信，Agent 在需要运行代码时调用 CodeRunner 现有的 `Execute` gRPC 接口。

**CodeRunner 本身零改动。**

---

## 二、产品需求

### 用户场景

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
  └── POST /internal/callback            ← HTTP 回调（CodeRunner 现有机制）

CodeRunner（完全不改动）
```

**数据流说明**：

| 操作 | 调用方 | 被调用方 | 协议 |
|------|--------|---------|------|
| 用户发消息 | 博客前端 | Agent `POST /chat` | HTTP + SSE |
| 用户确认运行 | 博客前端 | Agent `POST /confirm` | HTTP |
| Agent 触发执行 | Agent | CodeRunner `Execute` | gRPC |
| 执行结果回传 | CodeRunner client 节点 | Agent `POST /internal/callback` | HTTP |
| 用户普通运行（不经 Agent） | 博客后端 | CodeRunner `Execute` | gRPC（不变） |

---

### 3.2 CAR 框架设计

本方案使用 CAR（Control / Agency / Runtime）框架组织 Agent 的 Harness 层设计，参考 *Harness Engineering for Language Agents*（Preprints.org, 2026.03）。

> 可靠的 Agent 行为是被设计出来的，而不是从模型中涌现出来的。

#### Control — 谁在控制 Agent 的行为边界

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 执行预算 | 每次对话最大 **10 步** ReAct 循环 | 防止 Agent 陷入无限推理，控制 token 消耗 |
| 人工门控 | `propose_execution` 后需 `POST /confirm` 确认 | 代码执行是不可逆副作用，需人工介入 |
| 并行工具调用 | **禁用**，强制顺序调用 | 防止 Agent 并发调用工具时猜测上下文，产生幻觉 |
| 路由策略 | 解释/问答类 → 直接 AI 回答；涉及执行/调试 → Tool Calling 路径 | 减少不必要的工具开销 |
| 停止控制 | SSE 写入失败 → 立即 cancel Agent context | 避免用户已离开但 Agent 继续消耗 API 额度 |

#### Agency — Agent 能看到什么、能做什么

**工具列表（4 个，少而精）**：

| Tool | 触发场景 | 输入 | 输出给 AI |
|------|---------|------|----------|
| `explain_code` | "这段代码是什么意思" | `block_id` | 代码内容 + 语言 + 该 block 最近执行的 `result` 字段（无则为空） |
| `debug_code` | "为什么报错" | `block_id`, `error_message` | 代码内容 + 报错信息 + 最近执行 `result` |
| `generate_tests` | "帮我生成测试" | `block_id` | 代码内容 + 语言 |
| `propose_execution` | Agent 生成修复代码后 | `new_code`, `language`, `description` | 写入 Proposal，返回 ProposedExecution（description 由 AI 生成，描述修改内容） |

**语言标准化**：`propose_execution` 的 `language` 参数必须经过标准化后才能传给 CodeRunner。CodeRunner 仅接受以下精确字符串（`getFileExtension` 已确认）：

| AI 可能返回的值 | 标准化为 |
|----------------|---------|
| go, Go, golang | golang |
| python, Python, py | python |
| javascript, js, JavaScript | javascript |
| java, Java | java |
| c, C | c |

不在上表中的语言值 → 工具返回错误，Agent 告知用户"暂不支持此语言"。

**执行结果的存储与反馈**：

Agent 调用 CodeRunner `Execute` 时：
- 将 `ExecuteRequest.id` 设为 `proposal_id`（UUID v4），用于和回调中的 `id` 字段对应
- 将 `ExecuteRequest.uid` 设为 `0`（Agent 无用户 uid 概念）
- 将 `ExecuteRequest.callBackUrl` 设为如下内部地址：

```text
http://{agent_internal_base_url}/internal/callback?session_id={sid}&proposal_id={pid}&token={callback_token}
```

其中 `callback_token` 是每个 Proposal 生成时一并生成的 UUID v4，存入 `Proposal.CallbackToken`，用于验证回调合法性。

CodeRunner client 节点执行完毕后，将 `json.Marshal(ExecuteResponse)` POST 到此 URL，body 格式为：

```json
{
  "id": "proposal_id",
  "uid": 0,
  "grpcCode": "",
  "result": "执行输出（stdout + stderr 合并）",
  "callBackUrl": "...",
  "err": "执行错误信息（无则为空字符串）"
}
```

内部 callback handler 验证 `token` 后，将 `result` 和 `err` 写入 `AgentSession.ExecutionResults["proposal:{pid}"]`，并通过 SSE 连接推送给前端。

**行动空间隔离**：Agent 只能通过 `block_id` 访问当前 `article_id` 下的代码块，无效 `block_id` 返回工具错误。

**上下文注入策略**：

- 首次 `POST /chat`（`article_ctx.article_id` 非空，`session_id` 为空）：服务端创建新 session，写入文章上下文；UUID v4 `session_id` 通过第一个 SSE chunk 返回
- 后续请求携带 `session_id`，`article_ctx` 省略
- `session_id` 存在且传入非空 `article_ctx`：**清空 `Messages` 并覆盖文章上下文**（视为新文章的新会话，保留 session_id 方便前端）

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
  Input: new_code (string), language (one of: golang/python/javascript/java/c), description (brief explanation of changes).
```

#### Runtime — 系统怎么在长时间运行中保持连贯

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 会话持久化 | 内存 `sync.Map`，TTL 1 小时 | 用户读完一篇文章足够；无需引入 Redis |
| 上下文压缩 | **触发**：每次 LLM 调用前检查；**计数**：字符数 ÷ 4 估算；**策略**：保留最近 3 条消息，对更早历史调用额外 LLM 生成摘要替换；文章上下文始终不压缩；**失败**：跳过压缩继续（可能被 provider 因超长拒绝，返回 error chunk） | 防止历史消息溢出，保持文章感知 |
| 并发 /chat 处理 | 同一 `session_id` 收到新 `/chat` 请求时：向旧 SSE 连接推送 `{"type":"interrupted"}` 并关闭，替换 `SSEChan`，新请求使用新连接继续 | 防止 SSEChan 被覆盖导致旧流悬挂 |
| 断连后 callback 到达 | callback 写入 `ExecutionResults` 后，若 `SSEChan` 为 nil 或已关闭，暂存结果；用户下次 `/chat` 时，Agent 在 system message 中注入 "上次执行结果" 继续推理 | 避免结果丢失 |
| 可观测性 | Prometheus + 请求链路 ID | 独立服务独立监控 |
| 失败恢复 | LLM 调用失败 → SSE 推送错误 chunk，保留 session，用户可重试 | 不回滚对话 |

**Prometheus 指标**：

| 指标名 | 类型 | 标签 | 含义 |
|--------|------|------|------|
| `agent_chat_duration_seconds` | Histogram | `status`(success/error) | 单次 /chat 请求耗时 |
| `agent_tool_calls_total` | Counter | `tool_name`, `status`(success/error) | 各 Tool 调用次数 |
| `agent_sessions_active` | Gauge | — | 当前活跃 session 数 |

---

### 3.3 Session 生命周期与 Proposal 状态

#### Session ID 与 Proposal ID 生成

- **session_id**：首次 `/chat` 时由服务端生成 UUID v4，通过第一个 SSE chunk 返回
- **proposal_id**：`propose_execution` Tool 执行时由服务端生成 UUID v4，随 `ProposedExecution` 返回
- **callback_token**：`propose_execution` Tool 执行时同时生成 UUID v4，存入 `Proposal.CallbackToken`，嵌入 callback URL，用于验证回调合法性
- Proposal TTL：独立于 session，默认 **10 分钟**（可配置）；过期后 `/confirm` 返回 HTTP 410 Gone

#### AgentSession 数据结构

```go
type ArticleContext struct {
    ArticleID      string
    ArticleContent string       // 文章正文（Markdown）
    CodeBlocks     []CodeBlock
}

type CodeBlock struct {
    BlockID  string
    Language string
    Code     string
}

type AgentSession struct {
    ID               string
    ArticleID        string
    ArticleContext   ArticleContext
    Messages         []Message              // 对话历史（可被压缩）
    ExecutionResults map[string]ExecResult  // "proposal:{pid}" → 执行结果
    Proposals        map[string]Proposal    // proposal_id → 执行提议
    SSEChan          chan SSEEvent          // 当前活跃 SSE 连接的推送通道（nil 表示无活跃连接）
    PendingResults   []ExecResult           // 断连期间到达的执行结果，下次 /chat 注入
    CreatedAt        time.Time
    LastActiveAt     time.Time
    TTL              time.Duration          // 默认 1 小时
}

type Proposal struct {
    ID            string
    Code          string
    Language      string
    Description   string
    CallbackToken string    // UUID v4，用于验证回调合法性
    CreatedAt     time.Time
    ExpiresAt     time.Time // CreatedAt + proposal_ttl
    Confirmed     bool
}

type ExecResult struct {
    ProposalID string
    Result     string    // ExecuteResponse.Result（stdout+stderr 合并）
    Err        string    // ExecuteResponse.Err
    ReceivedAt time.Time
}
```

#### ProposedExecution 状态管理

- `propose_execution` Tool 被调用时：写入 `AgentSession.Proposals`，`Confirmed = false`
- `POST /confirm` 到达时：
  1. `session_id` 不存在 → HTTP 404
  2. `proposal_id` 不存在 → HTTP 404
  3. `Proposal.ExpiresAt` 已过 → HTTP 410 Gone
  4. `Confirmed` 已为 `true` → HTTP 409 Conflict（防重复提交）
  5. 标记 `Confirmed = true`，立即返回 HTTP 200（`{"request_id":"...","status":"accepted"}`）
  6. 在后台 goroutine 中向 CodeRunner 发起 gRPC `Execute` 调用（异步，不阻塞 /confirm 响应）
     - gRPC 调用失败：通过 SSEChan 推送 `{"type":"error","message":"..."}` 或写入 PendingResults

#### 对 CodeRunner 的鉴权

CodeRunner gRPC 接口要求 JWT Token（有效期 24 小时），通过 `tokenIssuer/GenerateToken` RPC 签发（需 `name` + `password`，不是 JWT）。

Agent 的 token 管理策略：
- **启动时**：调用 `GenerateToken` gRPC 获取初始 token
- **定期刷新**：每 **23 小时**自动刷新一次（早于 24 小时过期）
- **失败处理**：刷新失败时保留旧 token 继续使用，打印警告日志；旧 token 过期前重试
- 配置中存储 `service_name` + `service_password`，不存储 token 本身

---

### 3.4 HTTP API 定义

**鉴权**：`POST /chat` 和 `POST /confirm` 均需在请求头携带 API Key：

```text
X-Agent-API-Key: {api_key}
```

`api_key` 在 `configs/agent.yaml` 中通过环境变量配置（`${AGENT_API_KEY}`）。校验失败返回 HTTP 401。`POST /internal/callback` 通过 `token` query 参数鉴权（见 callback_token 机制），不需要 API Key。

#### `POST /chat`

**请求体（JSON）**：

```json
{
  "session_id": "",
  "user_message": "为什么这段代码报错？",
  "article_ctx": {
    "article_id": "article-123",
    "article_content": "...",
    "code_blocks": [
      { "block_id": "block-1", "language": "go", "code": "..." }
    ]
  }
}
```

**响应**：`Content-Type: text/event-stream`（SSE）

SSE 连接生命周期：**连接在 `done` 事件后保持打开**，继续监听异步事件（如 `execution_result`）。前端在用户离开文章页面时主动关闭连接，或收到 `interrupted` 时重新发起 `/chat`。`done` 仅表示"本轮 AI 回复完成，可提交下一条消息"，不关闭连接。

```text
data: {"type":"session","session_id":"uuid-v4"}   （首次，携带 session_id）

data: {"type":"chunk","content":"这段代码报错的原因是..."}

data: {"type":"proposal","proposal_id":"uuid-v4","code":"...","language":"golang","description":"修复了数组越界"}

data: {"type":"done"}   （本轮回复结束，连接保持打开）

... 用户点击"确认运行" → POST /confirm → 异步执行 → callback 到达 ...

data: {"type":"execution_result","proposal_id":"uuid-v4","result":"Hello World","err":""}

data: {"type":"interrupted"}   （仅当同 session 发起新 /chat 时，取代旧连接前发送）

data: {"type":"error","message":"..."}   （LLM 调用失败或 gRPC 调用失败时）
```

#### `POST /confirm`

**请求体（JSON）**：

```json
{
  "session_id": "uuid-v4",
  "proposal_id": "uuid-v4"
}
```

**响应（JSON）**：

```json
{ "request_id": "proposal_id", "status": "accepted" }
```

执行完成后，通过活跃 SSE 连接推送（若 SSE 已断开则存入 `PendingResults`）：

```text
data: {"type":"execution_result","proposal_id":"uuid-v4","result":"执行输出（stdout+stderr 合并）","err":""}
```

**错误响应**：HTTP 404（session/proposal 不存在）、HTTP 409（已确认）、HTTP 410（proposal 已过期）

#### `POST /internal/callback`（内部接口，不对外暴露）

供 CodeRunner client 节点回传执行结果。

**Query 参数**：`session_id`, `proposal_id`, `token`（callback_token，用于验证合法性）

**请求体**（CodeRunner 现有回调格式，`json.Marshal(*proto.ExecuteResponse)`）：

```json
{
  "id": "proposal_id",
  "uid": 0,
  "grpcCode": "",
  "result": "执行输出（stdout+stderr 合并）",
  "callBackUrl": "...",
  "err": "错误信息或空字符串"
}
```

**处理逻辑**：
1. 验证 `token` 与 `Proposal.CallbackToken` 匹配（不匹配 → HTTP 403）
2. 写入 `AgentSession.ExecutionResults["proposal:{pid}"]`
3. 若 `SSEChan` 活跃 → 推送 `execution_result` event
4. 若 `SSEChan` 不活跃 → 追加到 `PendingResults`
5. 返回 HTTP 200（确保 CodeRunner 不重试）

---

### 3.5 项目结构

独立微服务，在 coderunner repo 中新增 `cmd/agent/` 入口：

```text
cmd/agent/
  └── main.go

internal/agent/
  ├── session/
  │   └── store.go             # AgentSession + sync.Map + TTL 清理
  ├── handler/
  │   ├── chat.go              # POST /chat SSE handler
  │   ├── confirm.go           # POST /confirm handler
  │   └── callback.go          # POST /internal/callback handler
  ├── service/
  │   └── agent.go             # 编排：AI Provider → Tool 执行 → Execute gRPC
  ├── tools/
  │   └── tools.go             # 4 个 Tool 实现 + 语言标准化
  ├── coderunner/
  │   └── client.go            # CodeRunner gRPC client + token 管理（获取/刷新）
  └── ai/
      ├── provider.go          # AIProvider 接口 + ChatChunk 类型
      ├── claude/claude.go
      └── openai/openai.go

configs/agent.yaml
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
  internal_base_url: "http://localhost:8081"
  api_key: ${AGENT_API_KEY}               # /chat 和 /confirm 鉴权

coderunner:
  grpc_addr: "coderunner-server:50011"
  service_name: "agent-service"               # GenerateToken 用
  service_password: ${CODERUNNER_SERVICE_PASSWORD}  # GenerateToken 用
  token_refresh_interval: 82800               # 秒（23h）

agent:
  provider: claude
  max_steps: 10
  context_token_limit: 8000
  session_ttl: 3600          # 秒
  proposal_ttl: 600          # 秒（10 分钟）
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
- Agent 微服务水平扩展（内存 session 不支持多实例共享，需引入 Redis 后支持）
- 博客前端具体 UI 实现

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
| Agency | 上下文注入策略 | 首次全量，后续 session_id 复用；覆盖时清空历史 |
| Agency | 语言标准化 | 5 种语言，强制规范化后传给 CodeRunner |
| Runtime | 会话存储 | 内存 sync.Map，TTL 1h |
| Runtime | Proposal TTL | 10 分钟 |
| Runtime | 上下文压缩阈值 | 8000 tokens（历史，不含文章上下文） |
| Runtime | 并发 /chat | 新请求打断旧流，SSEChan 替换 |
| Runtime | 断连后 callback | 结果存 PendingResults，下次 /chat 注入 |
| Runtime | 断连处理 | SSE 写入失败 → context cancel |
| Runtime | 可观测性 | Prometheus（3 指标）+ 请求链路 ID |
