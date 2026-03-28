# 代码学习 Agent 实现计划

**目标：** 构建一个独立的 Go 微服务，让博客读者可以就文章内的代码与 AI Agent 对话——调试报错、解释逻辑、生成测试用例，Agent 可以提议修改代码并在用户确认后通过 CodeRunner 运行。

**架构：** 新服务复用现有 `codeRunner-siwu` 仓库。博客前端直接通过 HTTP + SSE 与 Agent 通信（SSE 连接在 `done` 后保持打开）。Agent 在需要运行代码时调用 CodeRunner 现有的 `Execute` gRPC 接口，CodeRunner 本身不做任何改动。

**Agent 框架：** [cloudwego/Eino](https://github.com/cloudwego/eino)，负责 LLM 接入、工具调用、ReAct 循环编排和流式输出。

**AI 模型：** 默认 `claude-opus-4-6`，支持配置切换至 OpenAI。

**设计文档：** `docs/superpowers/specs/2026-03-28-agent-design.md`

---

## 前置决策：代码组织方式

在开始开发前，两位开发者需要先共同决定：

**Agent 服务如何与现有 CodeRunner 代码共存？**

| 方案 | 说明 | 适用场景 |
|------|------|---------|
| 同一 `go.mod` | Agent 作为 `cmd/agent/` 入口，共享现有模块依赖 | 依赖高度重叠（gRPC proto、公共工具库等），希望简化维护 |
| 独立 `go.mod` + Go Workspace | Agent 在子目录下有自己的模块，通过 `go.work` 联调 | 希望依赖隔离，Agent 可独立演进版本 |
| 独立 Git 仓库 | Agent 完全独立，通过 gRPC 调用 CodeRunner | 团队分工明确，未来可能独立部署和发布 |

**建议：** 如果只是不想引入 Eino 等新依赖污染主模块，选"独立 `go.mod` + Go Workspace"是成本最低的隔离方案；如果未来有独立迭代或多人维护的需求，则考虑独立仓库。

---

## 并行开发分工

本计划按两条并行 Track 分配，两位开发者先共同完成 **Task 0**（接口约定和框架搭建），再各自独立开发，最后合并集成。

**开发者 A — 基础设施 Track：** 配置加载、Session 存储、CodeRunner gRPC 客户端（含 Token 管理）、`/confirm` 接口、`/internal/callback` 接口

**开发者 B — AI & Agent Track：** Eino 框架接入、工具层实现、Agent 编排逻辑、`/chat` SSE 处理器

**关键依赖：** 开发者 B 的工具实现依赖开发者 A 的 Session 存储完成后才能开始。

---

## Task 0：框架搭建与接口约定（两人一起完成）

**目的：** 在分开开发前对齐核心数据结构、接口契约，以及完成 Eino 框架的基础接入验证。

**需要共同完成的事项：**

1. **确定代码组织方案**（见上方"前置决策"）并初始化项目结构
2. **约定共享数据结构：** Session、Proposal、ExecResult、ArticleContext 等核心结构体，以及 SSE 事件的 JSON 格式（扁平化，不嵌套 payload 字段）
3. **验证 Eino 基础接入：** 引入 `cloudwego/eino`，跑通一个最小的 Claude 调用示例，确认流式输出、工具调用注册等核心能力正常工作
4. **约定 HTTP API 格式：** `/chat`、`/confirm`、`/internal/callback` 的请求/响应结构

**验收：** 双方对数据结构无歧义，Eino 基础调用通过，可以分别开始各自 Track。

---

## Track A — 开发者 A

---

### Task 1：配置加载

使用 Viper 加载 YAML 配置，涵盖服务端口、内部回调地址、API Key 鉴权、CodeRunner gRPC 地址和服务账号、AI 供应商选择及各自的 Key 和模型、Session TTL、Proposal TTL 等。

敏感字段（API Key、密码）通过 `BindEnv` 显式绑定环境变量，不在 YAML 中写明文。

**验收：** 配置加载测试通过，环境变量注入正常工作。

---

### Task 2：Session 存储

实现线程安全的内存 Session 存储，核心能力：

- Session 的创建、读取、更新
- Proposal 管理：新增（附带 `CallbackToken`、过期时间）、确认（含幂等保护防止重复确认）、过期检测
- 执行结果存储：将 CodeRunner 回调结果写入对应 Session
- SSE 通道管理：替换 SSEChan 时返回旧 channel，供调用方发送 `interrupted` 通知；断连后将未送达的结果暂存，待下次 `/chat` 注入
- TTL 自动过期清理

**验收：** 单元测试覆盖并发安全（使用 `-race` 检测）、Proposal 幂等、TTL 驱逐等场景。

---

### Task 3：CodeRunner gRPC 客户端

封装对 CodeRunner 的 gRPC 调用，包含：

- **语言标准化：** 将用户/AI 可能输入的各种写法（`go`、`js`、`py` 等）统一映射为 CodeRunner 能识别的精确字符串，不在映射表中的返回错误
- **Token 管理：** 启动时调用 CodeRunner 的 `GenerateToken` 接口获取初始 Token，每 23 小时自动刷新（早于 24 小时过期）；刷新失败时沿用旧 Token 并打印警告日志
- **Execute 调用：** 注入 Token 后发起 gRPC 请求

**验收：** 语言标准化测试覆盖全部有效/无效输入，Token 缓存测试确认不重复请求。

---

### Task 4：/internal/callback 接口

接收 CodeRunner 执行完成后的 HTTP 回调（query 参数含 `session_id`、`proposal_id`、`token`）：

- 验证 `token` 与 Session 中的 `Proposal.CallbackToken` 一致，不一致返回 403
- 解析 CodeRunner 回调 JSON，将执行结果（`result`、`err` 字段）写入 Session
- 若 SSEChan 活跃则推送 `execution_result` 事件；否则写入待推送队列

**验收：** 测试覆盖 token 合法/非法、结果写入、SSE 推送和断连存储场景。

---

### Task 5：/confirm 接口

接收用户确认执行 Agent 提议的代码：

- 校验顺序：Session 不存在 → 404；Proposal 不存在 → 404；Proposal 已过期 → 410；已确认 → 409
- 通过后立即返回 200，在后台 goroutine 异步发起 gRPC Execute 调用（不阻塞响应）
- 异步失败时通过 SSEChan 推送错误事件

**验收：** 测试覆盖各错误状态码和异步执行路径。

---

## Track B — 开发者 B

*(前两个 Task 可与 Track A 同时进行，工具实现需等 Task 2 完成后开始)*

---

### Task 6：Eino 框架接入与 AI Provider 封装

基于 Eino 框架接入 Claude 和 OpenAI：

- 配置化选择供应商（通过配置文件中的 `provider` 字段切换）
- 流式输出支持，结果实时推送给 SSE 连接
- **并行工具调用禁用**（Eino 配置 `parallel_tool_calls: false`），防止 Agent 并发调用工具产生幻觉
- 错误处理：LLM 调用失败时通过 SSE 推送错误事件

**验收：** 编译时接口检查通过，可跑通完整的流式对话和工具调用。

---

### Task 7：工具层实现

基于 Eino 的工具接口，实现 4 个工具：

- **`explain_code`：** 从 Session 中读取指定代码块，附加最近执行结果（若有），返回供 AI 分析
- **`debug_code`：** 同上，额外附加用户报告的报错信息
- **`generate_tests`：** 返回代码块内容和语言，供 AI 生成测试用例
- **`propose_execution`：** 先做语言标准化（调用 Task 3 中的函数），再在 Session 中写入 Proposal（含 `CallbackToken`、过期时间），返回 `proposal_id`

每个工具在 Eino 中注册时附带清晰的 `Use when` / `NOT for` 说明，帮助 AI 正确选择工具。

**验收：** 工具测试覆盖合法 block_id、非法 block_id、不支持的语言、成功创建 Proposal 等场景。

---

### Task 8：Agent 编排与 /chat SSE 处理器

基于 Eino 的 Agent 编排能力，实现完整的对话流程：

**编排逻辑：**
- 注入待推送的执行结果（PendingResults）作为本轮上下文前缀
- 上下文压缩：对话历史超过阈值时，使用 Eino 的上下文压缩机制（或 LLM 摘要）精简早期消息，文章上下文始终保留
- System prompt 包含文章全文和代码块列表
- 最大步数限制（防止无限循环）

**SSE 处理器：**
- 首次请求需传入文章上下文（`article_id`、`article_content`、`code_blocks`），创建 Session 并在第一个 SSE 事件中返回 `session_id`
- 后续请求携带 `session_id`，若同时传入文章上下文则视为重置（清空对话历史、覆盖文章内容）
- 替换 SSEChan 前，向旧连接发送 `interrupted` 通知
- SSE 连接在 `done` 事件后**保持打开**（等待异步执行结果），仅在 `interrupted` 或客户端断开时关闭

**验收：** 测试覆盖纯文本响应、工具调用循环、最大步数限制、SSE 连接生命周期管理。

---

## 集成阶段（两人合作）

---

### Task 9：鉴权中间件与路由

- 鉴权中间件：读取 `X-Agent-API-Key` 请求头，不匹配返回 401
- 路由：`/metrics`（无需鉴权）、`/internal/callback`（callback_token 验证）、`/chat`（需 API Key）、`/confirm`（需 API Key）
- 注册 Recovery 中间件防止 panic

---

### Task 10：服务入口与端到端冒烟测试

**服务入口：** 根据配置初始化 Eino + AI Provider，连接 CodeRunner gRPC，启动 HTTP 服务，监听信号优雅退出。

**冒烟测试验证以下场景：**

1. 无 API Key 调用 `/chat` → 返回 401
2. 首次 `/chat` 传入文章上下文 → SSE 第一个事件包含 `session_id`，后续有文本流和 `done`
3. `/confirm` 传入不存在的 session → 404；过期的 proposal → 410
4. `/metrics` 返回 Prometheus 指标
5. 所有单元测试通过，无竞态警告：`go test ./... -race -count=1`

---

## 开发前置步骤

两位开发者需要先完成：

1. 共同决定代码组织方案并初始化项目结构（Task 0）
2. 引入 Eino 依赖：`go get github.com/cloudwego/eino`
3. 根据 `docs/superpowers/specs/2026-03-28-agent-design.md` 设置本地环境变量
