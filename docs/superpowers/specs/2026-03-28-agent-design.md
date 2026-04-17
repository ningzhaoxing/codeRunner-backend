# 代码学习 Agent 功能设计文档

**日期**：2026-04-17（v5，HITL 流程对齐 Eino 推荐实践）
**状态**：草稿
**涉及模块**：代码学习 Agent（嵌入 CodeRunner Server）

---

## 一、背景与目标

CodeRunner 当前定位是博客平台的分布式代码执行引擎，负责接收代码执行请求、调度 client 节点、在 Docker 沙箱中运行代码并通过 HTTP 回调返回结果。

代码学习 Agent 在 CodeRunner Server 进程内提供 AI 对话能力，让博客读者可以就文章内的任意代码进行自然语言交互。Agent 作为 `internal/agent/` 独立模块存在，通过 `CodeExecutor` 接口调用 Server 现有的代码执行链路，共享 Gin HTTP Server、Logger、Prometheus 等基础设施。

**与 v3 方案的核心变更：**

| 维度 | v3（独立微服务） | v4（嵌入 Server） |
|------|-----------------|------------------|
| 部署形态 | 独立进程，`cmd/agent/main.go`，端口 8081 | 嵌入 Server，复用 Gin (50012) |
| 代码执行 | 外部 gRPC 调 CodeRunner (50011) | 进程内 `CodeExecutor` 接口 |
| Agent 框架 | flow/agent/react + 自定义 SmartMemory | ADK Runner + `ChatModelAgent` + 原生中间件 |
| SSE 事件 | 6 种自定义事件 | AgentEvent 透出 + 2 个业务事件 |
| Proposal 流程 | 自建状态机 | ADK HITL interrupt/resume |

---

## 二、产品需求

### 用户场景

1. **调试**：用户运行代码报错，向 Agent 提问 → Agent 分析错误并给出修复建议 → 用户确认后运行修复版本
2. **解释**：用户读到看不懂的代码 → 问 Agent "这段 goroutine 为什么用 WaitGroup？" → Agent 结合文章上下文解释
3. **测试**：用户实现了一个函数 → 让 Agent 生成边界测试用例 → 用户确认后运行验证

### 交互模式

- **对话式**：用户用自然语言自由提问，Agent 按需响应
- **上下文范围**：博客文章级别——Agent 了解整篇文章内容和所有代码块
- **执行确认**：Agent 可建议修改代码并触发执行，但必须经用户确认后才真正运行（HITL）
- **流式输出**：Agent 回复通过 SSE 直接推送给博客前端

### 功能边界（不做的事）

- 不支持跨文章的历史记忆（每篇文章独立会话，TTL 1 小时）
- Agent 不能直接修改用户代码，只能"提议"，用户手动确认
- 不提供代码自动补全

---

## 三、技术方案

### 3.1 整体架构

```text
博客前端
  ├── POST /agent/chat            → CodeRunner Server (Gin 50012)
  │     ← SSE AgentEvent 流      ←     └── internal/agent/ 模块
  │     （interrupt 时 SSE 关闭）
  │
  └── POST /agent/confirm         → CodeRunner Server
        ← SSE AgentEvent 流      ←     （等待执行 → Resume → 流式返回）

CodeRunner Server 内部
  ├── AgentService (Runner wrapping ChatModelAgent)
  │     ├── summarization 中间件（自动压缩历史）
  │     ├── reduction 中间件（截断大输出）
  │     └── propose-execution Tool (HITL interrupt)
  │
  ├── CodeExecutor 接口 ────→ Application Service ────→ WebSocket → Worker → Docker
  │
  └── POST /agent/internal/callback ← HTTP 回调（Worker 现有机制）
        └── 通过 channel 唤醒 confirm Handler
```

**数据流说明**：

| 操作 | 调用方 | 被调用方 | 协议 |
|------|--------|---------|------|
| 用户发消息 | 博客前端 | Server `POST /agent/chat` | HTTP + SSE |
| 用户确认运行 | 博客前端 | Server `POST /agent/confirm` | HTTP + SSE |
| Agent 触发执行 | AgentService | CodeExecutor（进程内接口调用） | Go interface |
| 执行结果回传 | Worker 节点 | Server `POST /agent/internal/callback` | HTTP |
| 用户普通运行（不经 Agent） | 博客后端 | CodeRunner `Execute` | gRPC（不变） |

---

### 3.2 CAR 框架设计

本方案使用 CAR（Control / Agency / Runtime）框架组织 Agent 的 Harness 层设计。

#### Control — 谁在控制 Agent 的行为边界

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 执行预算 | 每次对话最大 **10 步** ReAct 循环（`MaxIterations: 10`） | 防止 Agent 陷入无限推理，控制 token 消耗 |
| 人工门控 | `propose-execution` Tool 使用 `tool.StatefulInterrupt()` 暂停 Agent，需 `POST /agent/confirm` 确认 | 代码执行是不可逆副作用，需人工介入 |
| 并行工具调用 | **禁用**，强制顺序调用 | 防止 Agent 并发调用工具时猜测上下文，产生幻觉 |
| 路由策略 | 解释/问答类 → 直接 AI 回答；涉及执行 → Tool Calling 路径 | 减少不必要的工具开销 |
| 停止控制 | SSE 写入失败 → 立即 cancel Agent context | 避免用户已离开但 Agent 继续消耗 API 额度 |

#### Agency — Agent 能看到什么、能做什么

**工具列表（1 个 Go Tool，少而精）**：

| Tool | 触发场景 | 输入 | 行为 |
|------|---------|------|------|
| `propose_execution` | Agent 生成修复/测试代码后 | `new_code`, `language`, `description` | 语言标准化 → `tool.StatefulInterrupt()` 暂停 Agent，interrupt data 含 proposal 信息 |

**三种 Prompt 行为（非 Tool）**：

| 行为 | 触发场景 | 实现方式 |
|------|---------|---------|
| `explain-code` | 用户问代码含义 | Instruction 中的 `Use when` / `NOT for` 指令 |
| `debug-code` | 用户报告报错 | 同上 |
| `generate-tests` | 用户要求生成测试 | 同上，生成后自动调用 `propose_execution` |

**语言标准化**：`propose_execution` 的 `language` 参数必须经过标准化后才能传给 CodeRunner。

| AI 可能返回的值 | 标准化为 |
|----------------|---------|
| go, Go, golang | golang |
| python, Python, py | python |
| javascript, js, JavaScript | javascript |
| java, Java | java |
| c, C | c |

不在上表中的语言值 → Tool 返回错误，Agent 告知用户"暂不支持此语言"。

> **语言说明**：CodeRunner 实际支持的是 C 语言（`c`），不是 C++。`getFileExtension()` 中 `"c"` 映射到 `.c` 扩展名。CLAUDE.md 中标注的"C++"应更正为"C"。`c++`、`cpp` 等值会被 Tool 拒绝。

**上下文注入策略（`/agent/chat` 的三种模式）**：

| 模式 | 条件 | 行为 |
|------|------|------|
| **创建** | `session_id` 为空，`article_ctx` 非空 | 创建新 session（SessionStore + CheckPointStore），文章上下文写入 Instruction，返回 `session_id` |
| **继续** | `session_id` 非空，`article_ctx` 为空 | 从 SessionStore 读取历史消息，拼接新消息传入 `runner.Run()` |
| **重置** | `session_id` 非空，`article_ctx` 非空 | 清空 SessionStore 和 CheckPointStore 中该 session 的数据，覆盖文章上下文（切换到新文章） |

**冲突与边界处理**：

- `session_id` 为空 且 `article_ctx` 为空 → **400 Bad Request**（无法创建无上下文的 session）
- `session_id` 非空但 session 已过期（TTL 1h） → **404 Not Found**（session 已失效，前端需重新创建）
- `session_id` 非空 + `article_ctx` 非空 + session 已过期 → **以创建模式处理**（生成新 session_id 返回，等效于首次调用）
- Agent 处于 interrupt 态时收到新 /chat → 丢弃 pending interrupt，以新消息重新 `Run()`

**System Prompt 中的工具说明与意图澄清指令**：

```text
Tools available:
- propose_execution: Use when you have a concrete code change ready to run.
  NOT for: speculative or incomplete code.
  Input: new_code (string), language (one of: golang/python/javascript/java/c), description (brief explanation of changes).

Intent clarification (IMPORTANT):
- Before calling propose_execution, make sure the user's intent is clear.
- If the user's request is ambiguous (e.g., "帮我看看", "能不能修一下"), ask a clarifying
  question first. Do NOT guess and propose code.
- If you are unsure whether the user wants to run code or just understand it, ask.
- Only call propose_execution when you are confident about WHAT code to generate and WHY.
- Once propose_execution is called, the user must confirm before code runs — there is no
  way to "discuss the proposal" after that. Get alignment BEFORE proposing.
```

#### Runtime — 系统怎么在长时间运行中保持连贯

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 会话持久化 | `SessionStore`（JSONL 文件持久化，TTL 1h）+ `CheckPointStore`（HITL interrupt 状态，Runner 自动管理） | SessionStore 用 JSONL 存对话历史，重启可恢复；CheckPointStore 仅用于 interrupt/resume |
| 上下文压缩 | Eino ADK `summarization` 中间件，`TriggerCondition{ContextTokens: 8000}` | 框架原生支持，无需自定义 |
| 大输出截断 | Eino ADK `reduction` 中间件，`MaxLengthForTrunc: 50000` | 防止执行结果过大溢出上下文 |
| HITL 机制 | `tool.StatefulInterrupt()` → `agent.Resume()` | 框架原生，无需自建 Proposal 状态机 |
| 并发 /chat 处理 | 同一 `session_id` 的 `/agent/chat` 请求：若前一个 SSE 仍打开则推送 `interrupted` 并关闭；若 Agent 处于 interrupt 态则丢弃 pending interrupt，以新消息重新 `Run()` | 前端一次只维护一个 SSE 连接 |
| HITL 中新 /chat | 丢弃 pending interrupt，以新消息重新 `Run()`（不 Resume） | 意图澄清应在 propose 之前通过多轮对话完成（System Prompt 指导）；到达 interrupt 态说明 LLM 已有信心，用户发新消息视为意图变更 |
| SSE 连接模型 | `/agent/chat` SSE 在 interrupt 或正常结束时关闭；`/agent/confirm` 开启新 SSE 等执行完成后 Resume 并推送结果后关闭 | 对齐 Eino 推荐实践：每个 SSE 对应一个完整的 Agent 生命周期（Run 或 Resume），无需跨请求桥接 |
| callback 桥接 | 不需要 — CodeExecutor.Execute() 同步返回结果（底层 WebSocket `execute_sync` → `result`） | 无跨请求协调，无 channel，无竞态 |
| 可观测性 | Prometheus + 请求链路 ID（复用 Server 现有机制） | 统一监控 |
| 失败恢复 | LLM 调用失败 → AgentEvent 包含 Err，保留 session，用户可重试 | 不回滚对话 |

**Prometheus 指标**：

| 指标名 | 类型 | 标签 | 含义 |
|--------|------|------|------|
| `agent_chat_duration_seconds` | Histogram | `status`(success/error) | 单次 /agent/chat 请求耗时 |
| `agent_tool_calls_total` | Counter | `tool_name`, `status`(success/error) | Tool 调用次数 |
| `agent_sessions_active` | Gauge | — | 当前活跃 session 数 |

---

### 3.3 Session 生命周期与 HITL 流程

#### Session 管理

- **session_id**：首次 `/agent/chat` 时由服务端生成 UUID v4，通过 `session_created` 业务事件返回
- **SessionStore**：存储对话消息历史（`[]Message`），每次 `runner.Run()` 前从 SessionStore 读取历史拼接新消息传入，Run 结束后将新消息追加回 SessionStore
- **CheckPointStore**：仅用于 HITL interrupt/resume，由 Runner 自动管理（interrupt 时保存，Resume 时加载）
- **TTL**：SessionStore 和 CheckPointStore 共享 TTL 1 小时，过期自动清理

#### SessionStore 实现

消息历史使用 JSONL 文件持久化（每个 session 一个文件），对齐 Eino chatwitheino 示例的做法：

```go
// internal/agent/session/store.go
type SessionStore struct {
    baseDir string            // 存储目录，如 data/agent/sessions/
    mu      sync.Map          // session_id → *sync.Mutex（文件锁）
    ttl     time.Duration     // 默认 1 小时
}

type Session struct {
    ID           string
    Instruction  string      // 文章上下文构建的 System Prompt（创建时固定）
    FilePath     string      // JSONL 文件路径：{baseDir}/{session_id}.jsonl
    LastActiveAt time.Time
}

func (s *SessionStore) Create(sessionID, instruction string) *Session
func (s *SessionStore) GetMessages(sessionID string) ([]Message, error)  // 读 JSONL 全部行
func (s *SessionStore) Append(sessionID string, msgs ...Message) error   // 追加写入 JSONL
func (s *SessionStore) Exists(sessionID string) bool
func (s *SessionStore) Delete(sessionID string)                          // 删除 JSONL 文件
```

**JSONL 格式**：每行一条 `schema.Message` 的 JSON 序列化：

```text
{"role":"user","content":"为什么这段代码 panic？"}
{"role":"assistant","content":"这段代码 panic 因为..."}
{"role":"user","content":"那怎么修？"}
{"role":"assistant","content":"建议这样修复...","tool_calls":[...]}
```

**优点**：session 数据可在 Server 重启后恢复（内存 sync.Map 不行）；追加写入性能好；便于调试（直接 cat 文件看对话历史）。

后台 goroutine 定期扫描清理过期 JSONL 文件，通过 `context.Context` 控制生命周期，Server 优雅退出时 cancel context 停止清理循环。

#### CheckPointStore 实现

```go
// internal/agent/checkpoint/store.go
type MemoryCheckPointStore struct {
    data sync.Map           // session_id → checkpointData
    ttl  time.Duration      // 默认 1 小时
}

// 实现 compose.CheckPointStore 接口（Runner 内部调用）
func (s *MemoryCheckPointStore) Get(ctx context.Context, id string) ([]byte, bool, error)
func (s *MemoryCheckPointStore) Put(ctx context.Context, id string, data []byte) error
```

> CheckPointStore 仅在 HITL 场景下被 Runner 使用：interrupt 时自动保存 Agent 执行状态，Resume 时自动加载。普通多轮对话不经过 CheckPointStore。

#### PendingCallbacks 注册表

`PendingCallbacks` 管理 `proposal_id` → `chan ExecResult` 的映射，用于 confirm Handler 等待 Worker 回调：

```go
// internal/agent/handler/pending.go
type PendingCallbacks struct {
    mu       sync.Mutex
    channels map[string]chan ExecResult  // proposal_id → 等待通道
}

func (p *PendingCallbacks) Register(proposalID string) chan ExecResult
func (p *PendingCallbacks) Resolve(proposalID string, result ExecResult) bool
func (p *PendingCallbacks) Remove(proposalID string)
```

生命周期简单：confirm Handler 调 `Register()` 创建 channel 并阻塞等待；callback Handler 调 `Resolve()` 写入结果唤醒；confirm Handler 被唤醒后自动 `Remove()` 清理。

#### HITL 流程（propose_execution → confirm → resume）

**两次 SSE，两个 HTTP 请求（对齐 Eino 推荐实践）：**

```text
1. LLM 生成修复代码 → 调用 propose_execution Tool
2. Tool 内部：
   a. 语言标准化校验
   b. 生成 proposal_id (UUID v4) + callback_token (UUID v4)
   c. 调用 tool.StatefulInterrupt(ctx, &ProposalInfo{
        ProposalID:    proposalID,
        CallbackToken: callbackToken,
        Code:          newCode,
        Language:      language,
        Description:   description,
      }, argumentsInJSON)
   d. Agent 暂停，Runner 自动保存 checkpoint
3. AgentEvent 流自然包含 interrupt 事件 → SSE 推送给前端
4. /agent/chat SSE 关闭（迭代器结束，HTTP 请求完成）
5. 前端展示"运行修复版本？"按钮
6. 用户点击 → POST /agent/confirm {session_id, proposal_id}
7. confirm Handler（返回 SSE 流）：
   a. session_id 不存在 → 404
   b. proposal_id 不匹配 → 404
   c. 已确认 → 409
   d. 过期（>10min） → 410
   e. 合法 → 开启 SSE 流，推送 {"type":"executing"}
   f. 注册 pendingCallbacks.Register(proposalID) → 拿到 channel
   g. 调用 CodeExecutor.Execute(ctx, req)（异步触发执行）
   h. 阻塞等待 channel ← ExecResult（或超时）
   i. 收到结果 → 调用 agent.Resume(ctx, resumeInfo)
   j. 遍历 Resume 返回的 AsyncIterator → SSE 推送 AgentEvent
   k. 迭代器结束 → SSE 关闭
8. Agent 恢复后继续回答，用户在 confirm SSE 中收到
```

**时序图（正常路径）：**

```text
前端                 chat Handler         confirm Handler                    Worker
 │                       │                      │                              │
 │ POST /agent/chat      │                      │                              │
 │──────────────────────>│                      │                              │
 │  ← SSE: AgentEvent    │  runner.Run()        │                              │
 │  ← SSE: interrupt     │  ← 迭代器结束        │                              │
 │  ← SSE 关闭           │  ← HTTP 完成         │                              │
 │                       │                      │                              │
 │ POST /agent/confirm   │                      │                              │
 │─────────────────────────────────────────────>│                              │
 │  ← SSE: executing     │                      │                              │
 │                       │                      │ executor.Execute() 同步调用    │
 │                       │                      │ → WS execute_sync ──────────>│
 │                       │                      │   阻塞等待...                 │ Docker 执行
 │                       │                      │                              │
 │                       │                      │ ← WS result <───────────────│
 │                       │                      │ 拿到 ExecResult              │
 │                       │                      │ runner.Resume()              │
 │  ← SSE: AgentEvent    │                      │ ← 遍历迭代器                 │
 │  ← SSE 关闭           │                      │ ← HTTP 完成                  │
```

#### CodeExecutor 接口

```go
// internal/agent/executor.go
type CodeExecutor interface {
    // Execute 同步执行代码，阻塞直到 Worker 返回结果或超时
    Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error)
}

type ExecuteRequest struct {
    ProposalID string
    Code       string
    Language   string  // 已标准化
}

type ExecResult struct {
    Result string  // stdout + stderr 合并
    Err    string  // 执行错误信息（无则为空）
}
```

Server Application Service 实现此接口，内部调用 `ExecuteSync()`（WebSocket 同步模式）：

```go
func (e *codeExecutor) Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error) {
    client, err := e.clientManager.GetClientByBalance()
    if err != nil {
        return ExecResult{}, err
    }
    resp, err := client.SendSync(&proto.ExecuteRequest{
        Id:        req.ProposalID,
        Uid:       0,
        Language:  req.Language,
        CodeBlock: req.Code,
    }, 30*time.Second)
    if err != nil {
        return ExecResult{}, err
    }
    return ExecResult{Result: resp.Result, Err: resp.Err}, nil
}
```

> **待确认**：需验证 Worker 端对 `uid=0` 是否存在特殊处理逻辑。实现前需在现有 Worker 代码中排查 `uid` 的使用路径。

#### CodeRunner 同步执行模式（WebSocket 协议扩展）

Agent 的 `CodeExecutor.Execute()` 依赖 CodeRunner 新增的同步执行模式。此模式与现有异步模式并行存在，互不影响：

**协议扩展**（`protocol/message.go`）：

```go
MsgTypeExecuteSync MsgType = "execute_sync"  // Server → Worker：同步执行请求
MsgTypeResult      MsgType = "result"        // Worker → Server：执行结果回传
```

**Server 端改动**（`websocket/server/client.go`）：

- 新增 `SendSync(req, timeout)` 方法：发送 `execute_sync` 消息 → 注册 `pendingSync[requestID] = chan` → 阻塞等待 channel 或超时
- `Read()` 循环中识别 `MsgTypeResult` 消息 → `pendingSync[requestID]` 投递结果

**Worker 端改动**（`application/service/client/service.go`）：

- `Run()` 中根据消息类型区分：
  - `execute` → 执行后走 `CallBackSend()`（现有逻辑不变）
  - `execute_sync` → 执行后走 `WebsocketSend()` 回传 `result` 消息

**不变的部分**：Docker 执行逻辑、负载均衡、现有异步流程（`execute` + `ack` + `CallBackSend`）。

**涉及文件**（6 个）：

| 文件 | 改动 |
|------|------|
| `infrastructure/websocket/protocol/message.go` | +2 常量 |
| `infrastructure/websocket/server/client.go` | +`SendSync()` 方法，`Read()` 处理 `result` |
| `domain/server/service/clientManager.go` | +`pendingSync` map + `RegisterWaiter` / `DeliverResult` |
| `application/service/server/service.go` | +`ExecuteSync()` 方法 |
| `infrastructure/websocket/client/client.go` | `Read()` 区分 `execute` / `execute_sync` |
| `application/service/client/service.go` | `Run()` 中按模式选择回传方式 |

---

### 3.4 HTTP API 定义

**鉴权**：`POST /agent/chat` 和 `POST /agent/confirm` 均需在请求头携带 API Key：

```text
X-Agent-API-Key: {api_key}
```

`api_key` 在 `configs/dev.yaml` 中通过环境变量配置（`${AGENT_API_KEY}`）。校验失败返回 HTTP 401。

#### `POST /agent/chat`

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

SSE 连接生命周期：**interrupt 或正常结束时 SSE 关闭**（HTTP 请求完成）。前端在收到 interrupt 事件后展示确认按钮，用户确认时发起新的 `/agent/confirm` 请求获取新 SSE 流。

```text
data: {"type":"session_created","session_id":"uuid-v4"}

data: {"agent_name":"code-learning-agent","content":"这段代码报错的原因是..."}

data: {"agent_name":"code-learning-agent","tool_calls":[...]}

data: {"agent_name":"code-learning-agent","action":{"interrupted":{"data":{...}}}}
     （interrupt 事件，含 proposal 信息：code、language、description、proposal_id）

（SSE 关闭，HTTP 请求完成）
```

#### `POST /agent/confirm`

**请求体（JSON）**：

```json
{
  "session_id": "uuid-v4",
  "proposal_id": "uuid-v4"
}
```

**响应**：`Content-Type: text/event-stream`（SSE）

confirm Handler 开启 SSE 流，等待 Worker 执行完成后调用 `agent.Resume()`，将 Resume 事件流式推送：

```text
data: {"type":"executing"}
     （立即推送，告知前端执行已开始）

... Worker 执行中，confirm Handler 同步等待 ExecuteSync 返回 ...

data: {"agent_name":"code-learning-agent","content":"执行结果显示..."}
     （Resume 后 Agent 分析执行结果）

data: {"agent_name":"code-learning-agent","content":"修复成功，输出正确"}

（SSE 关闭，HTTP 请求完成）
```

**错误响应**（非 SSE，直接 JSON）：HTTP 404（session/proposal 不存在）、HTTP 409（已确认）、HTTP 410（proposal 已过期）

**超时**：`CodeExecutor.Execute()` 内部通过 `SendSync()` 的 timeout 控制（默认 30 秒），超时推送 `{"type":"error","message":"execution timeout"}` 后关闭 SSE。

---

### 3.5 项目结构

`internal/agent/` 作为独立模块嵌入 Server，不遵循 DDD 四层：

```text
internal/agent/
├── agent.go                    # AgentService：初始化 Runner + ChatModelAgent + 中间件装配
├── executor.go                 # CodeExecutor 接口定义
├── config.go                   # Agent 配置结构体 + Viper 解析
├── handler/
│   ├── chat.go                 # POST /agent/chat → SSE（interrupt 时关闭）
│   ├── confirm.go              # POST /agent/confirm → SSE（同步执行 → Resume → 推送）
│   └── middleware.go           # API Key 鉴权中间件
├── session/
│   └── store.go                # SessionStore（消息历史 + proposal 元数据，sync.Map + TTL）
├── checkpoint/
│   └── store.go                # CheckPointStore（HITL interrupt 状态，Runner 自动管理）
├── tools/
│   └── propose_execution.go    # HITL interrupt Tool + 语言标准化
└── ai/
    ├── provider.go             # AI Model 工厂（根据配置返回 Claude / OpenAI BaseChatModel）
    └── claude.go               # Claude model.BaseChatModel 适配
```

**注册入口**：在 `internal/interfaces/adapter/initialize/app.go` 的 `RunServer()` 中：

```go
// Agent 模块初始化
agentCfg := config.GetAgentConfig()
if agentCfg.Enabled {
    executor := NewCodeExecutor(serverService)  // Application Service 实现 CodeExecutor
    agentService := agent.NewAgentService(ctx, agentCfg, executor)
    agent.RegisterRoutes(router, agentService)  // /agent/chat, /agent/confirm
}
```

---

### 3.6 AI Model 配置

```go
// ChatModelAgent 初始化
chatAgent, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Name:          "code-learning-agent",
    Instruction:   buildInstruction(articleCtx),  // 文章 + 代码块 + 行为指令
    Model:         aiProvider.GetModel(),          // model.BaseChatModel
    ToolsConfig:   adk.ToolsConfig{
        Tools: []tool.BaseTool{proposeExecutionTool},
    },
    MaxIterations: cfg.MaxSteps,  // 默认 10
    Handlers:      []adk.ChatModelAgentMiddleware{summarizationMW, reductionMW},
})

// Runner 包装（管理 checkpoint + 流式输出）
runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           chatAgent,
    EnableStreaming:  true,
    CheckPointStore: checkPointStore,  // HITL interrupt/resume 用
})

// 每次对话：从 SessionStore 读历史 → 拼接新消息 → runner.Run()
history, _ := sessionStore.GetMessages(sessionID)
allMessages := append(history, schema.UserMessage(userMessage))
iter := runner.Run(ctx, allMessages, adk.WithCheckPointID(sessionID))
// 遍历 iter → SSE 推送
// Run 结束后追加 user + assistant 消息到 SessionStore（JSONL 追加写入）
sessionStore.Append(sessionID, schema.UserMessage(userMessage), assistantMsg)
```

配置文件（在 `configs/dev.yaml` 中新增 `agent:` 顶层块）：

```yaml
agent:
  enabled: true
  api_key: ${AGENT_API_KEY}
  provider: claude
  max_steps: 10
  session_ttl: 3600
  proposal_ttl: 600
  summarization:
    trigger_tokens: 8000
  reduction:
    max_length_for_trunc: 50000
    max_tokens_for_clear: 160000
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
- Agent 水平扩展（内存 SessionStore / CheckPointStore 不支持多实例共享，需引入 Redis 后支持）
- 博客前端具体 UI 实现

---

## 五、HARNESSCARD

| 维度 | 参数 | 值 |
|------|-----|----|
| Control | 最大执行步数 | 10 |
| Control | 人工门控 | HITL interrupt/resume（`tool.StatefulInterrupt` → `POST /agent/confirm` → `agent.Resume`） |
| Control | 并行工具调用 | 禁用 |
| Agency | 工具数量 | 1（propose_execution） |
| Agency | Prompt 行为数量 | 3（explain-code / debug-code / generate-tests） |
| Agency | 上下文注入策略 | 首次全量写入 Instruction；后续从 SessionStore 读历史传入 runner.Run() |
| Agency | 语言标准化 | 5 种语言，强制规范化后传给 CodeExecutor |
| Runtime | 会话存储 | SessionStore（JSONL 文件持久化，TTL 1h）+ CheckPointStore（HITL，Runner 管理） |
| Runtime | Proposal TTL | 10 分钟 |
| Runtime | 上下文压缩 | ADK summarization 中间件，阈值 8000 tokens |
| Runtime | 大输出截断 | ADK reduction 中间件，阈值 50000 字符 |
| Runtime | 并发 /chat | 新请求关闭旧 SSE；interrupt 态下视为意图变更，丢弃 proposal 重新 Run |
| Runtime | SSE 连接模型 | chat SSE + confirm SSE 分离，每个 SSE 对应完整 Agent 生命周期 |
| Runtime | 断连后 callback | 不适用 — 同步执行模式，无 callback |
| Runtime | 断连处理 | SSE 写入失败 → context cancel |
| Runtime | 可观测性 | Prometheus（3 指标）+ 请求链路 ID |
