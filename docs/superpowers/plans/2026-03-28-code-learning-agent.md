# 代码学习 Agent 实现计划

**目标：** 构建一个独立的 Go 微服务，让博客读者可以就文章内的代码与 AI Agent 对话——调试报错、解释逻辑、生成测试用例，Agent 可以提议修改代码并在用户确认后通过 CodeRunner 运行。

**架构：** 新服务位于 `cmd/agent/`，复用现有 `codeRunner-siwu` 模块。博客前端直接通过 HTTP + SSE 与 Agent 通信（SSE 连接在 `done` 后保持打开）。Agent 在需要运行代码时调用 CodeRunner 现有的 `Execute` gRPC 接口，CodeRunner 本身不做任何改动。

**技术栈：** Go 1.23、Gin、gRPC（现有）、Anthropic SDK、OpenAI SDK、UUID（现有）、Prometheus（现有）、Zap（现有）、Viper（现有）

**设计文档：** `docs/superpowers/specs/2026-03-28-agent-design.md`

---

## 并行开发分工

本计划按两条并行 Track 分配，两位开发者先共同完成 **Task 0**（接口约定），再各自独立开发，最后合并集成。

**开发者 A — 基础设施 Track：** 配置加载、Session 存储、CodeRunner gRPC 客户端、`/confirm` 接口、`/internal/callback` 接口

**开发者 B — AI & Agent Track：** AI Provider 抽象、Claude 实现、OpenAI 实现、工具层、Agent ReAct 循环

**关键依赖：** 开发者 B 的 Task 8（工具实现）依赖开发者 A 的 Task 2（Session 存储）完成后才能开始。

---

## 新增文件清单

```text
cmd/agent/main.go                          新建 — 服务入口
configs/agent.yaml                         新建 — Agent 配置模板
internal/agent/config/config.go            新建 — 配置结构体与加载逻辑
internal/agent/config/config_test.go       新建
internal/agent/session/types.go            新建 — 共享数据结构（Task 0，两人共同完成）
internal/agent/session/store.go            新建 — Session 内存存储
internal/agent/session/store_test.go       新建
internal/agent/coderunner/client.go        新建 — gRPC 客户端 + Token 管理
internal/agent/coderunner/client_test.go   新建
internal/agent/ai/provider.go              新建 — AI Provider 接口（Task 0，两人共同完成）
internal/agent/ai/claude/claude.go         新建 — Claude 实现
internal/agent/ai/claude/claude_test.go    新建
internal/agent/ai/openai/openai.go         新建 — OpenAI 实现
internal/agent/ai/openai/openai_test.go    新建
internal/agent/tools/tools.go              新建 — 4 个工具实现
internal/agent/tools/tools_test.go         新建
internal/agent/service/agent.go            新建 — Agent 服务与 ReAct 循环
internal/agent/service/agent_test.go       新建
internal/agent/handler/chat.go             新建 — POST /chat SSE 处理器
internal/agent/handler/chat_test.go        新建
internal/agent/handler/confirm.go          新建 — POST /confirm 处理器
internal/agent/handler/confirm_test.go     新建
internal/agent/handler/callback.go         新建 — POST /internal/callback 处理器
internal/agent/handler/callback_test.go    新建
internal/agent/handler/middleware.go       新建 — API Key 鉴权中间件
internal/agent/router/router.go            新建 — Gin 路由装配
```

---

## Task 0：共享接口约定（两人一起完成）

**负责人：** 双方共同

**目的：** 在分开开发前，先对齐两条 Track 都会用到的核心数据结构和接口，避免后续出现不兼容问题。

**需要约定的内容：**

1. **Session 数据结构**（`internal/agent/session/types.go`）：定义 `AgentSession`、`Proposal`、`ExecResult`、`ArticleContext`、`CodeBlock`、`Message` 等结构体，以及 `SSEEvent` 的类型（`map[string]any`，保证 SSE 事件格式扁平化）
2. **AI Provider 接口**（`internal/agent/ai/provider.go`）：定义 `Provider` 接口、`ChatChunk`、`ChatRequest`、`Tool`、`ToolCall` 等类型

**验收：** 两人对上述类型无歧义，可以分别开始各自 Track 的开发。

---

## Track A — 开发者 A

---

### Task 1：配置加载

**文件：** `configs/agent.yaml`、`internal/agent/config/config.go`、`config_test.go`

**工作内容：**

- 定义配置结构体，覆盖 Server（端口、内部回调地址、API Key）、CodeRunner（gRPC 地址、服务账号、密码、Token 刷新间隔）、Agent（AI 供应商、最大步数、上下文压缩阈值、Session TTL、Proposal TTL、Claude 和 OpenAI 各自的 Key 和模型）等字段
- 使用 Viper 加载 YAML，对敏感字段（API Key、密码）通过 `BindEnv` 显式绑定环境变量，不在 YAML 中写明文值
- 编写测试：验证环境变量注入正常工作

**验收：** `go test ./internal/agent/config/...` 通过

---

### Task 2：Session 存储

**文件：** `internal/agent/session/store.go`、`store_test.go`

**工作内容：**

- 基于 `sync.Map` 实现线程安全的 Session 存储，支持：创建 Session、按 ID 读取、获取 Proposal、新增 Proposal（附带 `CallbackToken`、过期时间）、确认 Proposal（含幂等保护，防止重复确认）、保存执行结果、推送 SSE 事件、替换 SSEChan（替换时返回旧 channel 供调用方发送 `interrupted` 事件）、清空待推送结果
- 实现 TTL 自动过期（后台 goroutine 定期清理）
- 错误类型：`ErrNotFound`、`ErrProposalNotFound`、`ErrAlreadyConfirmed`、`ErrProposalExpired`
- 编写测试：覆盖创建/读取、TTL 驱逐、Proposal 幂等、SSEChan 替换返回旧 channel 等场景，使用 `-race` 检测并发安全

**验收：** `go test ./internal/agent/session/... -race` 通过

---

### Task 3：CodeRunner gRPC 客户端 + Token 管理

**文件：** `internal/agent/coderunner/client.go`、`client_test.go`

**工作内容：**

- 实现语言标准化函数（`go/Go/golang` → `golang`，`py/python` → `python` 等），不在此表中的语言返回错误
- 实现 `TokenManager`：启动时调用 CodeRunner 的 `GenerateToken` gRPC（传入 `service_name` + `service_password`）获取初始 Token，每 23 小时自动刷新；刷新失败时沿用旧 Token 并打印警告日志
- 实现 `Client.Execute()`：从 `TokenManager` 获取 Token 后注入 gRPC metadata，发起 Execute 调用
- 导出 `Executor` 接口，方便测试时 mock
- 编写测试：覆盖语言标准化所有 case、Token 缓存不重复请求等场景

**验收：** `go test ./internal/agent/coderunner/...` 通过

---

### Task 4：内部回调处理器

**文件：** `internal/agent/handler/callback.go`、`callback_test.go`

**工作内容：**

- 处理 `POST /internal/callback`（query 参数：`session_id`、`proposal_id`、`token`）
- 验证 `token` 与 Session 中存储的 `Proposal.CallbackToken` 一致（不一致返回 403）
- 解析 CodeRunner 回调的 JSON 体（字段：`id`、`result`、`err` 等），将结果写入 Session
- 若 SSEChan 活跃则推送 `execution_result` 事件；否则写入 `PendingResults` 等待下次 `/chat` 注入
- 编写测试：覆盖 token 合法/非法、结果写入、SSE 推送、pending 存储等场景

**验收：** `go test ./internal/agent/handler/... -run TestCallback` 通过

---

### Task 5：/confirm 处理器

**文件：** `internal/agent/handler/confirm.go`、`confirm_test.go`

**工作内容：**

- 处理 `POST /confirm`（请求体：`session_id`、`proposal_id`）
- 校验顺序：Session 不存在 → 404；Proposal 不存在 → 404；Proposal 已过期 → 410；已确认 → 409
- 校验通过后立即返回 200（`{"request_id":"...","status":"accepted"}`），在后台 goroutine 异步发起 gRPC Execute 调用
- 异步调用失败时通过 SSEChan 推送错误事件；若 SSEChan 已关闭则写入 PendingResults
- 构造回调 URL 格式：`{internal_base_url}/internal/callback?session_id=...&proposal_id=...&token=...`
- 编写测试：覆盖各错误状态码、成功返回 202 语义

**验收：** `go test ./internal/agent/handler/... -run TestConfirm` 通过

---

## Track B — 开发者 B

*(Task 6、7 可与 Track A 同时进行；Task 8 需等 Task 2 完成后开始)*

---

### Task 6：Claude AI Provider

**文件：** `internal/agent/ai/claude/claude.go`、`claude_test.go`

**依赖：** 先执行 `go get github.com/anthropics/anthropic-sdk-go`

**工作内容：**

- 实现 `Provider` 接口，封装 Anthropic SDK 的流式 Messages API
- 将 `session.Message` 的 `user`/`assistant` 角色映射为 SDK 格式（`tool` 角色折叠进下一轮 user turn 作为上下文）
- 将 `ai.Tool` 的 schema 转换为 Anthropic ToolParam 格式
- 处理流式事件：文本 delta → `ChatChunk.Content`；工具调用 stop 事件 → `ChatChunk.ToolCall`
- 根据 `StopReason` 设置 `FinishReason`（`tool_use` → `tool_calls`，其余 → `stop`）
- 编写编译时接口检查测试（确保 `*Provider` 实现了 `ai.Provider`）

**验收：** `go test ./internal/agent/ai/claude/...` 通过（编译检查即可，不需要真实 API Key）

---

### Task 7：OpenAI AI Provider

**文件：** `internal/agent/ai/openai/openai.go`、`openai_test.go`

**依赖：** 先执行 `go get github.com/openai/openai-go`

**工作内容：**

- 实现 `Provider` 接口，封装 OpenAI SDK 的流式 Chat Completions API
- **必须禁用并行工具调用**（`parallel_tool_calls: false`），防止 Agent 并发调用工具产生幻觉
- 将 `session.Message` 映射为 OpenAI 消息格式，System prompt 单独传入
- 使用 Accumulator 收集流式结果，结束后统一输出 ToolCall
- 根据是否有工具调用设置 `FinishReason`
- 编写编译时接口检查测试

**验收：** `go test ./internal/agent/ai/openai/...` 通过

---

### Task 8：工具层实现

**文件：** `internal/agent/tools/tools.go`、`tools_test.go`

**依赖：** Task 2（Session 存储）完成并合并

**工作内容：**

实现 `Registry`，提供工具定义（schema）和工具执行两个能力：

- **工具定义：** 返回 4 个工具的 JSON Schema，每个工具附带 `Use when` / `NOT for` 说明，`propose_execution` 的 `language` 字段枚举值限定为 5 种合法语言
- **工具执行：** 根据工具名分发调用
  - `explain_code(block_id)`：从 Session 中查找代码块，附加最近执行结果（若有）
  - `debug_code(block_id, error_message)`：同上，附加报错信息
  - `generate_tests(block_id)`：返回代码块内容和语言
  - `propose_execution(new_code, language, description)`：先做语言标准化（调用 Task 3 中的函数），再调用 Store 的 `AddProposal`，返回 `proposal_id`
- 编写测试：覆盖合法 block_id、非法 block_id、不支持的语言、成功创建 Proposal 等场景

**验收：** `go test ./internal/agent/tools/...` 通过

---

### Task 9：Agent 服务（ReAct 循环）

**文件：** `internal/agent/service/agent.go`、`agent_test.go`

**依赖：** Task 6、7（AI Provider）、Task 8（工具层）

**工作内容：**

- 导出 `AgentService` 接口（供 handler 和 router 依赖），构造函数返回接口类型
- `Chat()` 方法流程：
  1. 读取并清空 `PendingResults`，将结果前缀注入本轮 user message
  2. 上下文压缩检查（每次 LLM 调用前）：历史消息 token 估算（字符数 ÷ 4）超过阈值时，保留最近 3 条消息，对更早的历史发起额外 LLM 调用生成摘要替换；文章上下文始终保留不压缩；压缩失败则跳过
  3. 构建 system prompt（嵌入文章内容和代码块列表）
  4. ReAct 循环（最多 `maxSteps` 次）：调用 AI Provider → 流式输出文本 chunk → 检测工具调用 → 执行工具 → 追加工具结果 → 继续推理
  5. `FinishReason` 为 `stop` 或无工具调用时发送 `done` 事件退出
  6. 超过最大步数时发送错误事件退出
- 编写测试：使用 mock Provider，覆盖纯文本响应、最大步数限制（避免无限循环）等场景

**验收：** `go test ./internal/agent/service/...` 通过

---

## 集成阶段（两人合作）

---

### Task 10：/chat SSE 处理器 + 鉴权中间件

**文件：** `internal/agent/handler/chat.go`、`chat_test.go`、`middleware.go`

**工作内容：**

- **鉴权中间件：** 读取请求头 `X-Agent-API-Key`，与配置值比对，不一致返回 401
- **chat 处理器：**
  - 首次请求（`session_id` 为空）：要求 `article_ctx.article_id` 非空，创建 Session，生成 `session_id`
  - 后续请求：按 `session_id` 查找 Session；若同时传入非空 `article_ctx`，调用 `OverrideArticleContext`（清空历史消息、覆盖文章上下文）
  - 替换 SSEChan：先通过 `ReplaceSSEChan` 拿到旧 channel，若旧 channel 非空则发送 `interrupted` 事件通知旧连接关闭
  - 设置响应头（`text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`）
  - 首次请求在第一个 SSE chunk 中返回 `session_id`
  - 使用 `c.Stream` 保持连接：**`done` 事件不关闭连接**（仅标志本轮 AI 回复完成），`interrupted` 事件才关闭连接；客户端断开（`ctx.Done()`）时清空 SSEChan
- 编写测试：覆盖首次请求返回 session_id、SSE content-type、401 无 Key 场景

**验收：** `go test ./internal/agent/handler/... -v -race` 通过

---

### Task 11：路由装配

**文件：** `internal/agent/router/router.go`

**工作内容：**

- 使用 Gin 装配所有路由：
  - `GET /metrics`：Prometheus 指标（无需鉴权）
  - `POST /internal/callback`：回调接口（callback_token 鉴权，无需 API Key）
  - `POST /chat`（需 API Key）
  - `POST /confirm`（需 API Key）
- 注册 `gin.Recovery()` 中间件防止 panic 导致服务崩溃

**验收：** `go build ./internal/agent/router/...` 通过

---

### Task 12：服务入口

**文件：** `cmd/agent/main.go`

**工作内容：**

- 加载配置，根据 `agent.provider` 字段初始化 Claude 或 OpenAI Provider
- 初始化 Session Store（使用配置中的 Session TTL）
- 初始化 CodeRunner gRPC 客户端（启动时即获取初始 Token，失败则退出）
- 注册 Prometheus 指标（`agent_chat_duration_seconds`、`agent_tool_calls_total`、`agent_sessions_active`）
- 装配所有依赖，启动 HTTP 服务
- 监听 SIGINT/SIGTERM，30 秒优雅退出

**验收：** `go build ./cmd/agent/...` 通过

---

### Task 13：端到端冒烟测试

**工作内容：**

逐一验证以下场景：

1. 无 API Key 调用 `/chat` → 返回 401
2. 首次 `/chat` 传入 `article_ctx` → SSE 第一个事件包含 `session_id`，后续有 `chunk` 和 `done` 事件
3. 调用 `/confirm` 传入不存在的 `session_id` → 返回 404
4. 调用 `/confirm` 传入已过期的 `proposal_id` → 返回 410
5. `/metrics` 端点返回 Prometheus 指标
6. 运行所有单元测试，无竞态警告：`go test ./internal/agent/... -race -count=1`

---

## 开发前置步骤

两位开发者都需要先执行：

```bash
go get github.com/anthropics/anthropic-sdk-go
go get github.com/openai/openai-go
go mod tidy
```

并设置本地开发环境变量（参考 `configs/agent.yaml` 中的字段说明）。
