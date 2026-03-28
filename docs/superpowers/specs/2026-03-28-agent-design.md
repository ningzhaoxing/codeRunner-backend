# CodeRunner Agent 功能设计文档

**日期**：2026-03-28
**状态**：草稿
**涉及模块**：代码学习 Agent、安全审查 Agent

---

## 一、背景与目标

CodeRunner 当前定位是博客平台的分布式代码执行引擎：用户在博客前端提交代码，经 gRPC → WebSocket → Docker 沙箱执行，结果异步回调返回。

本次新增两个 Agent 功能，将 CodeRunner 从"执行工具"升级为"学习工具 + 安全防线"：

| Agent | 核心价值 |
|-------|---------|
| 代码学习 Agent | 用户可对话式探索博客内所有代码：调试报错、解释逻辑、生成测试用例 |
| 安全审查 Agent | 在代码进入容器前静态扫描恶意模式，降低安全成本 |

两个 Agent 职责独立，互不耦合。

---

## 二、产品需求

### 2.1 代码学习 Agent

#### 代码学习 Agent 用户场景

1. **调试**：用户运行代码报错，不知道原因，向 Agent 提问 → Agent 分析错误并给出修复建议 → 用户确认后运行修复版本
2. **解释**：用户读到看不懂的代码 → 问 Agent "这段 goroutine 为什么用 WaitGroup？" → Agent 结合文章上下文解释
3. **测试**：用户实现了一个函数 → 让 Agent 生成边界测试用例 → 用户确认后运行验证

#### 交互模式

- **对话式**：用户用自然语言自由提问，Agent 按需响应
- **上下文范围**：博客文章级别——Agent 了解整篇文章的内容和所有代码块
- **执行确认**：Agent 可建议修改代码并触发执行，但必须经用户确认后才真正运行
- **流式输出**：Agent 回复通过 gRPC server-side streaming 推送，博客后端通过 SSE 转发给前端

#### 功能边界（不做的事）

- 不支持跨文章的历史记忆（每篇文章独立会话）
- Agent 不能直接修改用户代码，只能"提议"，用户手动确认
- 不提供代码自动补全（那是 IDE 的职责）

---

### 2.2 安全审查 Agent

#### 安全审查 Agent 用户场景

- 博客读者（可能是恶意用户）提交包含 fork bomb、系统调用滥用、加密矿工特征的代码
- 在进入 Docker 容器之前拦截，保护执行节点资源

#### 行为规范

- 命中规则 → 返回明确的拒绝错误，拒绝原因对调用方可见
- 未命中规则 → 透明放行，对现有执行流程零侵入
- 不使用 AI 做安全审查（延迟高、有误判、成本高）；纯规则引擎，响应 < 1ms

---

## 三、技术方案

### 3.1 整体架构

```text
博客前端
  │ 用户发消息 / 点击"确认运行修复代码"
  ▼
博客后端
  │ gRPC AgentChat(session_id, article_ctx, user_message)
  │ 返回：server-side streaming，逐 chunk 推送 Agent 回复
  ▼
CodeRunner AgentService（新增）
  ├── 维护会话状态（session_id → AgentSession）
  ├── 调用 AI Provider（Claude / OpenAI / 可配置）
  │     Agent Tools：explain_code / debug_code / generate_tests / propose_execution
  └── 用户确认后，以进程内函数调用方式调用 application/service/server.Execute()
        │ WebSocket → client 节点 → Docker 沙箱
        └── 执行结果通过原有 callBackUrl 回调机制返回
```

**现有 `Execute` 接口完全保留**，两条链路并行存在：

- 用户直接点"运行" → 博客后端调 `Execute` gRPC（不变）
- 用户与 Agent 对话，Agent 建议执行代码 → 博客后端调 `AgentChat` + `AgentConfirmExecution` → AgentService **以进程内函数调用方式**复用 `application/service/server/service.go` 的 `Execute()` 方法，不发起 gRPC 网络请求

---

### 3.2 CAR 框架设计

本方案使用 CAR（Control / Agency / Runtime）框架组织 Agent 的 Harness 层设计，参考 *Harness Engineering for Language Agents*（Preprints.org, 2026.03）。

> 可靠的 Agent 行为是被设计出来的，而不是从模型中涌现出来的。

#### Control — 谁在控制 Agent 的行为边界

| 决策点 | 设计选择 | 理由 |
|--------|---------|------|
| 执行预算 | 每次对话最大 **10 步** ReAct 循环 | 防止 Agent 陷入无限推理，控制 token 消耗 |
| 人工门控 | `propose_execution` 返回 `ProposedExecution` 结构，执行必须等用户确认 | 代码执行是不可逆副作用，高风险操作需人工介入 |
| 并行工具调用 | **禁用**，强制顺序调用 | 防止 Agent 并发调用工具时猜测上下文，产生幻觉 |
| 路由策略 | 解释/问答类 → 直接 AI 回答；涉及执行/调试 → Tool Calling 路径 | 减少不必要的工具开销，加快简单问题响应速度 |
| 停止控制 | SSE 连接断开时立即取消 Agent context | 避免用户已离开但 Agent 继续消耗 API 额度 |

#### Agency — Agent 能看到什么、能做什么

**工具列表（4 个，少而精）**：

| Tool | 触发场景 | 输入 | 输出给 AI |
|------|---------|------|----------|
| `explain_code` | "这段代码是什么意思" | `block_id` | 代码内容 + 语言 + 该 block 在 session 中最近一次执行的输出（stdout/stderr，无则为空） |
| `debug_code` | "为什么报错" / 代码运行失败后 | `block_id`, `error_message` | 代码内容 + 报错信息 + 最近执行输出 |
| `generate_tests` | "帮我生成测试" | `block_id` | 代码内容 + 语言 |
| `propose_execution` | Agent 生成修复代码后 | `new_code`, `language` | 在 AgentSession 写入 ProposedExecution 记录，向调用方返回 `ProposedExecution` 消息体 |

**执行结果的存储**：

当 AgentService 调用内部 `Execute()` 时，构造如下内部回调地址（复用现有 Gin 路由，新增路由 `POST /internal/agent/callback`）作为 `callBackUrl`：

```text
http://localhost:{port}/internal/agent/callback?session_id={session_id}&proposal_id={proposal_id}
```

执行完成后，executor 将结果 POST 到此 URL。内部回调 handler 从 query 参数中读取 `session_id` 定位 session，读取 `proposal_id` 构造存储 key，将结果写入 `AgentSession.ExecutionResults["proposal:{proposal_id}"]`，供后续 Tool 调用（`explain_code`、`debug_code`）通过此 key 读取。

若客户端在 `AgentConfirmRequest.callback_url` 中提供了非空 URL，内部回调 handler 在写入 session 后**同时转发**原始 payload 给该 URL（与原有 Execute 回调格式完全一致），确保博客前端的 SSE 通知正常工作。`callback_url` 字段为**可选**：若为空，结果仅写入 session，Agent 在下一轮对话中引用。

**行动空间隔离**：

- Agent 只能通过 `block_id` 访问**当前 `article_id` 下的代码块**
- 无效的 `block_id` 返回工具错误，不暴露其他文章数据

**两阶段上下文注入**：

- 首次调用 `AgentChat` 时（`article_ctx.article_id` 非空）：服务端创建新 session，写入文章上下文
- 后续调用（`article_ctx` 为默认空值）：服务端通过 `session_id` 查找已有 session，直接读取上下文
- 若 `session_id` 存在但同时传入非空 `article_ctx`：以新传入的 `article_ctx` 覆盖 session 中的文章上下文（支持文章更新场景）

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
| 上下文压缩 | 对话历史（不含首次注入的文章上下文）token 数超过 **8000** 时，通过额外的 LLM 调用将早期对话历史生成摘要替换；文章上下文始终保留不压缩 | 保持 Agent 对整篇文章的感知，防止历史消息溢出 |
| 断连处理 | gRPC 流写入失败 → 立即 cancel Agent context → 停止 LLM 调用 | 避免资源浪费 |
| 可观测性 | 复用现有 Prometheus + TraceID，新增指标（见下） | 与现有监控体系统一 |
| 失败恢复 | LLM 调用失败（超时/限流）→ 向流返回含错误信息的 chunk，保留 session 状态，用户可重试 | 不回滚整个对话，只回退当前轮次 |

**新增 Prometheus 指标**：

| 指标名 | 类型 | 标签 | 含义 |
|--------|------|------|------|
| `agent_chat_duration_seconds` | Histogram | `status`(success/error) | 单次 AgentChat 请求耗时 |
| `agent_tool_calls_total` | Counter | `tool_name`, `status`(success/error) | 各 Tool 调用次数 |
| `agent_sessions_active` | Gauge | — | 当前活跃 session 数 |

---

### 3.3 Session 生命周期与 ProposedExecution 状态

#### Session ID 生成

- **由服务端生成**：首次调用 `AgentChat`（`article_ctx.article_id` 非空）时，服务端生成 UUID v4 作为 `session_id`，通过第一个流式 chunk 的 `session_id` 字段返回给调用方
- 后续调用由调用方在请求中携带此 `session_id`

#### proposal_id 生成

- 由服务端在 `propose_execution` Tool 执行时生成 UUID v4，写入 `AgentSession.Proposals` 并通过 `ProposedExecution.proposal_id` 字段返回给调用方
- Proposal 不设独立 TTL，其生命周期与所属 session 一致（session 过期时连同 proposal 一起销毁）

#### AgentSession 数据结构

```go
type AgentSession struct {
    ID               string
    ArticleID        string
    ArticleContext   ArticleContext          // 完整文章上下文，始终保留
    Messages         []Message              // 对话历史（可被压缩）
    ExecutionResults map[string]ExecResult  // block_id → 最近执行结果
    Proposals        map[string]Proposal    // proposal_id → 待确认的执行提议
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

- `propose_execution` Tool 被调用时：在 `AgentSession.Proposals` 中写入记录，`Confirmed = false`
- `AgentConfirmExecution` 到达时：
  1. 检查 `session_id` 是否存在（不存在 → `codes.NotFound`）
  2. 检查 `proposal_id` 是否存在（不存在 → `codes.NotFound`）
  3. 检查 `Confirmed` 是否已为 `true`（已确认 → `codes.AlreadyExists`，防重复）
  4. 标记 `Confirmed = true`，触发内部 Execute 调用

#### uid 的传递

- `AgentConfirmRequest` 中显式包含 `uid uint32` 字段（与 `ExecuteRequest` 一致，由博客后端填写）
- `AgentService` 调用内部 `Execute()` 时直接从 `AgentConfirmRequest.uid` 读取，不依赖 JWT claims 提取
- StreamInterceptor 只负责验证 JWT 有效性（调用现有 `Verify()`），不需要提取 claims

---

### 3.4 DDD 模块结构

遵循现有 DDD 分层架构，新增以下模块：

```text
internal/
├── domain/agent/
│   ├── entity/
│   │   └── session.go          # AgentSession、Proposal、ExecResult 结构定义
│   └── service/
│       └── agentService.go     # 领域服务接口定义
│
├── application/service/agent/
│   └── service.go              # 编排：调 AI Provider → 解析 Tool Call → 调 server.Execute()
│
├── infrastructure/
│   ├── ai/
│   │   ├── provider.go         # AIProvider 接口 + ChatChunk 类型定义
│   │   ├── claude/claude.go    # Claude 实现
│   │   └── openai/openai.go    # OpenAI 实现
│   └── security/
│       └── screener.go         # 安全审查规则引擎
│
└── interfaces/
    ├── controller/agent/
    │   └── chat.go             # gRPC streaming handler
    └── adapter/initialize/
        ├── agent.go            # AgentService 初始化
        └── grpc.go             # 扩展现有 gRPC 注册，新增 StreamInterceptor
```

---

### 3.5 gRPC Interceptor 扩展

现有 gRPC 注册（`grpcRegistry.go`）仅配置了 `UnaryInterceptor`（用于 JWT 鉴权和 TraceID 注入），`AgentChat` 是 server-side streaming RPC，需要额外注册 `StreamInterceptor`。

**改动**：在 `internal/adapter/initialize/grpc.go` 中新增 `grpc.StreamInterceptor`，实现与现有 UnaryInterceptor 相同的 JWT 鉴权和 TraceID 注入逻辑，确保 streaming RPC 同样受到认证保护。所有 Agent streaming RPC 均需鉴权，无豁免路径。

---

### 3.6 gRPC 接口定义

在现有 proto 文件中扩展（`Execute` 接口不变）：

```protobuf
service CodeRunner {
  // 现有接口，不变
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // 新增：Agent 对话（server-side streaming）
  rpc AgentChat(AgentChatRequest) returns (stream AgentChatResponse);

  // 新增：确认执行 Agent 提议的代码（返回"已接受"语义，结果通过 callBackUrl 异步返回）
  rpc AgentConfirmExecution(AgentConfirmRequest) returns (AgentConfirmResponse);
}

message AgentChatRequest {
  string session_id          = 1;  // 首次调用为空，服务端生成后通过响应返回
  string user_message        = 2;
  ArticleContext article_ctx = 3;  // 首次调用时传入；后续调用省略（默认空值）
                                   // 若 session 已存在且此字段非空，则覆盖 session 中的文章上下文
}

message ArticleContext {
  string article_id      = 1;
  string article_content = 2;  // 文章正文（Markdown）
  repeated CodeBlock code_blocks = 3;
}

message CodeBlock {
  string block_id  = 1;
  string language  = 2;
  string code      = 3;
}

message AgentChatResponse {
  string session_id           = 1;  // 首次响应时携带服务端生成的 session_id，后续为空
  string content              = 2;  // 文字 chunk（流式，可为空）
  ProposedExecution proposed  = 3;  // 非空时表示 Agent 提议执行代码
}

message ProposedExecution {
  string proposal_id = 1;
  string code        = 2;
  string language    = 3;
  string description = 4;  // Agent 对修改的说明
}

message AgentConfirmRequest {
  string session_id   = 1;
  string proposal_id  = 2;
  uint32 uid          = 3;  // 与 ExecuteRequest.uid 一致，由博客后端填写
  string callback_url = 4;  // 可选：非空时执行结果同时转发给此 URL（供博客前端 SSE 使用）
}

// 与 Execute 接口语义对齐：仅表示"已接受"，执行结果通过 callback_url 异步返回
message AgentConfirmResponse {
  string request_id = 1;   // 对应内部 Execute 调用的 ID，用于追踪
  string grpc_code  = 2;   // "accepted"
}
```

---

### 3.7 AI Provider 抽象

```go
// infrastructure/ai/provider.go

type ChatChunk struct {
    Content    string      // 文字内容片段（流式文本）
    ToolCall   *ToolCall   // 非空时表示 AI 发起工具调用
    FinishReason string    // "stop" | "tool_calls" | "max_steps" | "error"
    Err        error       // 非 nil 时表示流式传输中发生错误
}

type ToolCall struct {
    Name  string
    Input json.RawMessage
}

type ChatRequest struct {
    Messages    []Message
    Tools       []Tool
    MaxSteps    int
    Stream      bool
}

type AIProvider interface {
    Chat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
}
```

配置文件切换（`configs/dev.yaml`）：

```yaml
agent:
  provider: claude          # 切换为 openai 即可换供应商
  max_steps: 10
  context_token_limit: 8000 # 触发上下文压缩的历史消息 token 阈值（不含文章上下文）
  session_ttl: 3600         # 秒
  claude:
    api_key: ${CLAUDE_API_KEY}
    model: claude-opus-4-6  # 按实际 Anthropic API 支持的 model ID 填写
  openai:
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
```

---

### 3.8 安全审查 Agent

**调用位置**：在 `application/service/server/service.go` 的 `Execute()` 方法入口处调用 `SecurityScreener.Screen()`，位于应用层，遵循 DDD 分层（基础设施实现，应用层调用）。

```text
gRPC Execute 请求
  → interfaces/controller/server/execute.go（大小校验）
  → application/service/server/service.go → SecurityScreener.Screen(code, language)
      ├── 命中规则 → 返回 domain error → controller 转换为 gRPC codes.PermissionDenied
      └── 未命中  → 继续现有执行逻辑（完全不变）
```

**规则存储与热更新**：

- 规则以 YAML 文件维护，默认路径 `configs/security_rules.yaml`
- 使用 `fsnotify` 监听文件变更，变更时原子替换内存中的规则集（`sync.RWMutex` 保护）
- 规则更新失败（YAML 解析错误）时保留旧规则，打印错误日志，不影响服务

**规则模型**：支持两种类型：

```yaml
rules:
  - id: fork_bomb_shell
    language: [shell, bash]
    type: regex          # 单条正则
    pattern: ':\(\)\{:\|:&\};:'
    message: "Fork bomb pattern detected"

  - id: infinite_loop_with_alloc
    language: [python, javascript]
    type: compound       # 多条正则同时命中
    patterns:
      - 'while\s+True\s*:'
      - '(?:bytearray|bytes)\s*\(\s*\d{7,}'  # 分配 >= 10MB
    message: "Potential infinite loop with large memory allocation"
```

**初始规则集（按语言分类）**：

| 类别 | 规则类型 | 适用语言 |
|------|---------|---------|
| Fork Bomb | regex | Shell、Python |
| 系统调用滥用（rm -rf、shutdown 等） | regex | Python、Go |
| 网络扫描（socket 连接非本地地址） | regex | Python、Go、JS |
| 加密矿工特征（stratum+tcp） | regex | 全语言 |
| 无限循环 + 大内存分配 | compound | Python、JS |

初期覆盖 Python / Go / JS / Shell，其余语言透明放行。

---

## 四、不在本期范围内

- 跨文章的持久化对话历史（当前 TTL 1 小时后消失）
- Agent 主动推送通知（执行完成后 Agent 自动解释结果）
- 安全审查的 AI 增强模式
- 多语言 Security 规则的完整覆盖（初期覆盖 Python / Go / JS / Shell）

---

## 五、HARNESSCARD（参考）

遵循 *Harness Engineering for Language Agents* 建议，记录本 Harness 的关键设计参数：

| 维度 | 参数 | 值 |
|------|-----|----|
| Control | 最大执行步数 | 10 |
| Control | 人工门控 | propose_execution 需用户确认 |
| Control | 并行工具调用 | 禁用 |
| Agency | 工具数量 | 4 |
| Agency | 行动空间隔离 | article_id 级别过滤 |
| Agency | 上下文注入策略 | 首次全量注入，后续通过 session_id 复用 |
| Runtime | 会话存储 | 内存 sync.Map，TTL 1h |
| Runtime | 上下文压缩阈值 | 8000 tokens（历史消息，不含文章上下文） |
| Runtime | 压缩方式 | 额外 LLM 调用生成摘要替换早期历史 |
| Runtime | 断连处理 | gRPC 流写入失败 → context cancel |
| Runtime | 可观测性 | Prometheus（3 个指标）+ TraceID via StreamInterceptor |
