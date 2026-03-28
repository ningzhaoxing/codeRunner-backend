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

#### 用户场景

1. **调试**：用户运行代码报错，不知道原因，向 Agent 提问 → Agent 分析错误并给出修复建议 → 用户确认后运行修复版本
2. **解释**：用户读到看不懂的代码 → 问 Agent "这段 goroutine 为什么用 WaitGroup？" → Agent 结合文章上下文解释
3. **测试**：用户实现了一个函数 → 让 Agent 生成边界测试用例 → 用户确认后运行验证

#### 交互模式

- **对话式**：用户用自然语言自由提问，Agent 按需响应
- **上下文范围**：博客文章级别——Agent 了解整篇文章的内容和所有代码块
- **执行确认**：Agent 可建议修改代码并触发执行，但必须经用户确认后才真正运行
- **流式输出**：Agent 回复通过 SSE 流式推送，用户无需等待完整响应

#### 功能边界（不做的事）

- 不支持跨文章的历史记忆（每篇文章独立会话）
- Agent 不能直接修改用户代码，只能"提议"，用户手动确认
- 不提供代码自动补全（那是 IDE 的职责）

---

### 2.2 安全审查 Agent

#### 用户场景

- 博客读者（可能是恶意用户）提交包含 fork bomb、系统调用滥用、加密矿工特征的代码
- 在进入 Docker 容器之前拦截，保护执行节点资源

#### 行为规范

- 命中规则 → 返回明确的拒绝错误，拒绝原因对调用方可见
- 未命中规则 → 透明放行，对现有执行流程零侵入
- 不使用 AI 做安全审查（延迟高、有误判、成本高）；纯规则引擎，响应 < 1ms

---

## 三、技术方案

### 3.1 整体架构

```
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
  └── 用户确认后，复用 application/service/server.Execute() 触发代码执行
        │ WebSocket → client 节点 → Docker 沙箱
        └── 执行结果注入对话上下文，Agent 继续推理
```

**现有 `Execute` 接口完全保留**，两条链路并行存在：

- 用户直接点"运行" → 博客后端调 `Execute`（不变）
- 用户与 Agent 对话，Agent 建议执行代码 → 博客后端调 `AgentChat` → AgentService 内部调用 Execute 业务逻辑（不绕 gRPC）

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
| `explain_code` | "这段代码是什么意思" | `block_id` | 代码内容 + 语言 + 最近执行结果 |
| `debug_code` | "为什么报错" / 代码运行失败后 | `block_id`, `error_message` | 代码内容 + 报错信息 + 执行输出 |
| `generate_tests` | "帮我生成测试" | `block_id` | 代码内容 + 语言 |
| `propose_execution` | Agent 生成修复代码后 | `new_code`, `language` | 返回 `ProposedExecution` 响应，挂起等待确认 |

**行动空间隔离**：

- Agent 只能通过 `block_id` 访问**当前 `article_id` 下的代码块**
- 无效的 `block_id` 返回工具错误，不暴露其他文章数据

**两阶段上下文注入**：

- 首次调用 `AgentChat` 时传入完整文章内容 + 所有代码块，注入 `AgentSession`
- 后续调用只需传 `session_id` + 用户消息，减少重复 token 消耗

**System Prompt 中的工具说明（每个工具附 Use when / NOT for）**：

```
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
| 会话持久化 | 内存 map（`sync.Map`），TTL 1 小时 | 用户读完一篇文章足够；无需引入 Redis |
| 上下文压缩 | 对话历史超过 **8000 tokens** 时，将早期历史压缩为摘要替换 | 保持 Agent 对整篇文章的感知，防止上下文溢出 |
| 断连处理 | SSE 写入失败 → 立即 cancel Agent context → 停止 LLM 调用 | 避免资源浪费 |
| 可观测性 | 复用现有 Prometheus + TraceID，新增指标：`agent_chat_duration_seconds`、`agent_tool_calls_total`、`agent_sessions_active` | 与现有监控体系统一 |
| 失败恢复 | LLM 调用失败（超时/限流）→ 返回错误 chunk 给前端，保留 session 状态，用户可重试 | 不回滚整个对话，只回退当前轮次 |

---

### 3.3 DDD 模块结构

遵循现有 DDD 分层架构，新增以下模块：

```
internal/
├── domain/agent/
│   ├── entity/
│   │   └── session.go          # AgentSession：会话ID、对话历史、文章上下文、TTL
│   └── service/
│       └── agentService.go     # 领域服务接口定义
│
├── application/service/agent/
│   └── service.go              # 编排：调 AI Provider → 解析 Tool Call → 调 Execute
│
├── infrastructure/
│   ├── ai/
│   │   ├── provider.go         # AIProvider 接口
│   │   ├── claude/claude.go    # Claude 实现（Anthropic SDK）
│   │   └── openai/openai.go    # OpenAI 实现
│   └── security/
│       └── screener.go         # 安全审查规则引擎
│
└── interfaces/
    ├── controller/agent/
    │   └── chat.go             # gRPC streaming handler
    └── adapter/initialize/
        └── agent.go            # AgentService 初始化（复用现有模式）
```

---

### 3.4 gRPC 接口定义

在现有 proto 文件中扩展（`Execute` 接口不变）：

```protobuf
service CodeRunner {
  // 现有接口，不变
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // 新增：Agent 对话（server-side streaming）
  rpc AgentChat(AgentChatRequest) returns (stream AgentChatResponse);

  // 新增：确认执行 Agent 提议的代码
  rpc AgentConfirmExecution(AgentConfirmRequest) returns (ExecuteResponse);
}

message AgentChatRequest {
  string session_id     = 1;
  string user_message   = 2;
  ArticleContext article_ctx = 3;  // 首次调用时传入，后续为空
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
  string content              = 1;  // 文字 chunk（流式）
  ProposedExecution proposed  = 2;  // 非空时表示 Agent 提议执行代码
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
  string callback_url = 3;  // 执行结果回调地址
}
```

---

### 3.5 AI Provider 抽象

```go
// infrastructure/ai/provider.go
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
  context_token_limit: 8000
  session_ttl: 3600         # 秒
  claude:
    api_key: ${CLAUDE_API_KEY}
    model: claude-opus-4-6
  openai:
    api_key: ${OPENAI_API_KEY}
    model: gpt-4o
```

---

### 3.6 安全审查 Agent

在 `Execute` gRPC handler 入口处插入，对现有执行链路零侵入：

```
gRPC Execute 请求进来
  ↓
SecurityScreener.Screen(code, language) — 纯规则引擎，< 1ms
  ├── 命中规则 → 返回 gRPC codes.PermissionDenied + 拒绝原因
  └── 未命中  → 放行，进入现有执行链路（完全不变）
```

**初始规则集（按语言分类）**：

| 类别 | 规则示例 | 适用语言 |
|------|---------|---------|
| Fork Bomb | `:(){:\|:&};:` 模式 | Shell、Python |
| 系统调用滥用 | `os.system("rm -rf")` / `subprocess` 调用敏感命令 | Python、Go |
| 网络扫描 | `socket.connect` 到非本地地址 | Python、Go、JS |
| 加密矿工特征 | stratum+tcp 协议字符串 | 全语言 |
| 无限资源消耗 | `while True: pass`（无 sleep/break）+ 大内存分配组合 | Python、JS |

规则以配置文件维护，支持热更新，不需要重启服务。

---

## 四、不在本期范围内

- 跨文章的持久化对话历史（当前 TTL 1 小时后消失）
- Agent 主动推送通知（执行完成后 Agent 自动解释结果）
- 安全审查的 AI 增强模式
- 多语言 Security 规则的完整覆盖（初期覆盖 Python / Go / JS）

---

## 五、HARNESSCARD（参考）

遵循 *Harness Engineering for Language Agents* 建议，记录本 Harness 的关键设计参数：

| 维度 | 参数 | 值 |
|------|-----|----|
| Control | 最大执行步数 | 10 |
| Control | 人工门控 | propose_execution 确认 |
| Control | 并行工具调用 | 禁用 |
| Agency | 工具数量 | 4 |
| Agency | 行动空间隔离 | article_id 级别过滤 |
| Agency | 上下文注入策略 | 首次全量，后续增量 |
| Runtime | 会话存储 | 内存，TTL 1h |
| Runtime | 上下文压缩阈值 | 8000 tokens |
| Runtime | 断连处理 | context cancel |
| Runtime | 可观测性 | Prometheus + TraceID |
