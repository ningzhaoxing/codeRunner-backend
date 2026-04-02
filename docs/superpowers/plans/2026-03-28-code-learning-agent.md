# 代码学习 Agent — MVP 实现方案

> **状态**：设计草案 · **日期**：2026-03-28

---

## 1. 概述

代码学习 Agent 是一个独立 Go 微服务，基于 **Eino ReAct + Skill Middleware** 架构构建，不复用 CodeRunner 现有代码（CodeRunner 本身零改动）。Agent 让博客读者可以就文章内的任意代码进行自然语言对话，支持调试报错、解释逻辑、生成测试，并可在用户确认后通过 CodeRunner 实际运行修改后的代码。

**核心流程：**

1. 用户在博客页面打开文章，前端初始化 SSE 连接，调用 `POST /chat` 发送首条消息，携带文章内容与所有代码块
2. Agent 创建 Session，将文章上下文注入 SmartMemory，返回 `session_id`
3. Agent 进入 Eino ReAct 循环，根据用户意图选择调用相应 Skill（解释/调试/生成测试/提议执行）
4. Skill 执行结果注入 LLM 上下文，LLM 生成自然语言回复，通过 SSE 流式推送给前端
5. 若 LLM 调用了 `propose-execution` Skill，前端收到 `proposal` 事件，展示"运行修复版本"按钮
6. 用户点击确认 → `POST /confirm` → Agent 异步调用 CodeRunner gRPC Execute → 结果通过 HTTP 回调返回
7. Agent 将执行结果推送至 SSE，注入下一轮对话上下文，用户可继续追问

> **关键约束**：`POST /confirm` 必须立即返回（202 语义），gRPC Execute 异步执行；SSE 连接在 `done` 事件后保持打开，等待 `execution_result` 异步推送；同一 `session_id` 的新 `/chat` 请求会向旧连接发送 `interrupted` 后替换连接。

**MVP 包含：**

- 一个 Go Tool Skill：`propose-execution`（有真实副作用：生成 Proposal、推送 SSE 事件）
- 三种对话行为（`explain-code`、`debug-code`、`generate-tests`）通过 System Prompt 指令实现，无需 tool call
- Session 内存存储（TTL 1 小时）
- CodeRunner gRPC 集成（含 Token 自动刷新）
- SSE 流式输出与 `interrupted` 机制
- API Key 鉴权、Prometheus 指标

**MVP 不包含：**
- 跨文章持久化记忆
- Agent 水平扩展（当前内存 Session 不支持多实例共享）
- 安全审查规则引擎

**文件结构：**

```text
internal/agent/
├── agent.go                         # Agent 初始化与 Stream() 入口
├── system_prompt.txt                # Agent 系统提示词
├── handler/
│   ├── chat.go                      # POST /chat SSE 处理器
│   ├── confirm.go                   # POST /confirm 处理器
│   ├── callback.go                  # POST /internal/callback 回调处理器
│   └── middleware.go                # API Key 鉴权中间件
├── session/
│   ├── types.go                     # AgentSession、Proposal、ExecResult 结构体
│   └── store.go                     # 内存 Session 存储（sync.Map + TTL）
├── coderunner/
│   └── client.go                    # CodeRunner gRPC 客户端 + Token 管理
└── skills/
    ├── explain-code/
    │   └── SKILL.md                 # 代码解释 Skill 描述
    ├── debug-code/
    │   └── SKILL.md                 # 代码调试 Skill 描述
    ├── generate-tests/
    │   └── SKILL.md                 # 测试生成 Skill 描述
    └── propose-execution/
        └── SKILL.md                 # 提议执行 Skill 描述

cmd/agent/
└── main.go                          # 服务入口，依赖装配与优雅退出

configs/
└── agent.yaml                       # 配置模板（敏感字段通过环境变量注入）
```

---

## 2. 架构

| 组件 | 选型 |
| --- | --- |
| Agent 框架 | cloudwego/Eino（ReAct + Skill Middleware） |
| Skill 运行时 | Eino Tool 接口，Go 原生实现（无 Python 脚本） |
| 记忆系统 | Eino SmartMemory，内存存储，TTL 1 小时 |
| 流式输出 | Server-Sent Events（Gin `c.Stream`） |
| AI 接入 | Eino ModelProvider，默认 Claude Opus 4.6，可切换 OpenAI |
| 代码执行 | CodeRunner gRPC Execute（现有接口） |
| HTTP 框架 | Gin（与 CodeRunner 主服务一致） |
| 指标监控 | Prometheus（复用现有 client_golang） |

**架构链路：**

```text
博客前端 (HTTP + SSE)
    │
    ▼
Gin Handler（chat.go / confirm.go / callback.go）
    │  API Key 鉴权中间件
    ▼
AgentService.Stream()（agent.go）
    ├── SmartMemory.Load()           # 加载历史对话 + 文章上下文
    ├── Eino ReAct Loop（Eino v0.x）
    │   ├── Claude / OpenAI ModelProvider（流式）
    │   └── Skill Middleware
    │       └── propose-execution    # → 写入 Proposal（含 CallbackToken）
    ├── ChatModelAgentMiddleware 钩子
    │   ├── AfterChatModel           # 注入 chunk / proposal / done 事件
    │   └── AfterToolCall            # 注入 tool_result 事件（调试用）
    ├── SmartMemory.Persist()        # 持久化本轮对话
    └── SSEWriter                    # 流式推送给前端
          ↑
CodeRunner gRPC Execute（confirm 异步触发）
    └── HTTP 回调 → callback.go → SSEWriter
```

---

## 3. Skill 设计

前三种对话行为（`explain-code`、`debug-code`、`generate-tests`）**不实现为 Go Tool**。博客全文和所有代码块已在首次 `/chat` 时嵌入 System Prompt，LLM 可直接引用，无需 tool call 取数据。对应的行为指令（`Use when` / `NOT for`）写入 System Prompt，指导 LLM 在合适时机输出相应内容。

---

### 对话行为 1：`explain-code`（Prompt 指令）

```text
Use when: 用户询问某段代码是什么意思；用户问"为什么这样写"；用户希望理解代码的行为或设计
NOT for: 修复 bug；生成新代码；解释报错信息（应走 debug-code 行为）
行为：直接从已嵌入的代码块中引用对应代码，结合文章背景解释其功能与设计意图
```

---

### 对话行为 2：`debug-code`（Prompt 指令）

```text
Use when: 用户报告代码报错；用户说"跑不起来"；用户描述了非预期的运行结果
NOT for: 解释正常运行的代码逻辑（走 explain-code）；生成测试（走 generate-tests）
行为：直接引用已嵌入的代码块及执行结果（若有），分析报错原因，给出修复建议
```

---

### 对话行为 3：`generate-tests`（Prompt 指令）

```text
Use when: 用户要求生成测试用例；用户说"帮我验证这个函数的边界情况"
NOT for: 直接运行测试（需用户通过 propose-execution 确认后再运行）；解释代码逻辑
行为：直接引用已嵌入的代码块，生成边界测试代码，随后调用 propose-execution 工具提交执行
```

---

### Skill 4：`propose-execution`（Go Tool）

当 LLM 准备好一段具体可运行的代码时调用。在 Session 中创建 Proposal 记录（含 `CallbackToken`、过期时间），向前端推送 `proposal` SSE 事件，等待用户通过 `POST /confirm` 确认后异步触发 CodeRunner 执行。

```text
Use when: LLM 生成了修复后的代码，需要用户确认是否运行；LLM 生成了测试代码，需要实际执行验证
NOT for: 尚未完成的代码片段；猜测性代码；解释用途（无需运行）
```

Skill 实现（Go Tool）：

- 输入：`new_code`（代码内容），`language`（必须为 CodeRunner 支持的精确字符串：golang / python / javascript / java / c），`description`（LLM 对本次修改的简短说明）
- 执行：语言标准化校验（`go`/`Go` → `golang` 等）；调用 `store.AddProposal()` 生成 `proposal_id` + `CallbackToken`；写入 Session；推送 `proposal` SSE 事件
- 输出（注入 LLM 上下文）：`{ "proposal_id": "uuid", "status": "pending_confirmation" }`

---

## 6. 核心推理逻辑

**LLM 输入（每轮）：**

- System Prompt：内嵌文章全文 + 所有代码块列表（block_id、语言、代码内容）+ 三种对话行为的 `Use when` / `NOT for` 指令 + `propose-execution` 工具说明
- SmartMemory 注入的历史对话（超过 8000 token 时自动压缩早期消息为摘要）
- 若有待推送的执行结果（PendingResults），前置注入为本轮 user message 的上下文前缀

**LLM 决策：**

- 纯回答（`finish_reason: stop`）→ 流式输出文本，推送 `done` 事件，ReAct 循环结束
- 工具调用（`finish_reason: tool_calls`）→ 执行对应 Skill，结果注入上下文，继续循环
- 达到最大步数（默认 10 步）→ 推送错误事件，强制退出循环

**歧义处理：**

- `block_id` 不存在 → Skill 返回工具错误，LLM 收到后提示用户确认代码块 ID
- 语言不支持 → `propose-execution` Skill 返回错误，LLM 告知用户当前支持的语言范围
- `propose-execution` 连续两轮被调用但用户未确认 → LLM 上下文中已有 `pending_confirmation` 状态，通常不会重复提议

---

## 7. 上下文管理（SmartMemory）

```go
memory := eino.NewSmartMemory(eino.SmartMemoryConfig{
    MaxTokens:     8000,       // 历史消息 token 上限（文章上下文单独存储，不参与压缩）
    CompressModel: llmProvider, // 压缩时调用同一 LLM 生成摘要
    Storage:       sessionStore, // 自定义 Storage 接口，底层为 sync.Map + TTL 1h
})
```

**注入方式：** Eino `MessageModifier` 机制，在每次 ReAct 循环前将 Session 中的文章上下文（首次写入后固定不变）和历史对话拼接注入，消息序列为：

```
[system prompt + 文章上下文] + [历史对话（压缩后）] + [当前 user message]
```

**跨轮次恢复：**

1. `SmartMemory.Load(session_id)` → 从 Session Store 读取历史消息
2. `MessageModifier` 注入 → 构建完整上下文传给 LLM
3. ReAct 循环结束后 `SmartMemory.Persist(session_id)` → 写回 Session Store

**上下文压缩触发条件：** 历史消息（不含文章上下文）token 估算（字符数 ÷ 4）超过 8000 时，保留最近 3 条消息，对更早历史调用额外 LLM 生成摘要替换。

---

## 8. 用户意图澄清

**触发条件：** 用户提问模糊（如"帮我看看"而不指定代码块）时，LLM 直接输出澄清问句，不调用任何 Skill。

**ReAct 终止与恢复：**

- LLM 输出纯文本且无 tool_call → ReAct Loop 自然结束，推送 `done` 事件，SSE 连接保持打开
- 用户补充信息后发起新的 `/chat` 请求 → 携带同一 `session_id`，SmartMemory 加载历史后继续推理
- 新 `/chat` 到达时，向旧 SSE 连接推送 `interrupted` 事件后替换连接

**SSE 事件分类：** `AfterChatModel` 钩子检查输出内容：

- 包含 tool_call → 继续 ReAct，暂不推送 `done`
- 纯文本输出 → 推送文本 chunk + `done`（可能是澄清问句，也可能是最终回答）

---

## 9. SSE 事件机制

**两个来源：**

1. **Eino 框架 AsyncIterator**：LLM 流式输出的文本 chunk
2. **ChatModelAgentMiddleware 钩子**：业务自定义事件（proposal、execution_result、error、interrupted）

**事件注入点：**

| 事件类型 | 注入方式 | 触发时机 |
| --- | --- | --- |
| `session` | Handler 层直接写入 | 首次 `/chat`，Session 创建后 |
| `chunk` | AfterChatModel AsyncIterator | LLM 每个 text delta |
| `proposal` | propose-execution Skill 内部 | Skill 写入 Proposal 后 |
| `done` | AfterChatModel | LLM finish_reason == stop |
| `execution_result` | callback.go | CodeRunner 回调到达 |
| `error` | AfterChatModel / confirm goroutine | LLM 报错 / gRPC 执行失败 |
| `interrupted` | chat.go，ReplaceSSEChan 前 | 同 session_id 新 /chat 到达 |

**事件字段映射：**

| 事件 | 关键字段 |
| --- | --- |
| `session` | `session_id` |
| `chunk` | `content`（字符串） |
| `proposal` | `proposal_id`、`code`、`language`、`description` |
| `execution_result` | `proposal_id`、`result`（stdout+stderr 合并）、`err` |
| `error` | `message` |
| `done` | —（无附加字段） |
| `interrupted` | —（无附加字段） |

所有事件格式为扁平 JSON，通过 `data: {...}\n\n` 推送：

```text
data: {"type":"session","session_id":"uuid-v4"}
data: {"type":"chunk","content":"这段代码..."}
data: {"type":"proposal","proposal_id":"uuid","code":"...","language":"golang","description":"修复了数组越界"}
data: {"type":"done"}
data: {"type":"execution_result","proposal_id":"uuid","result":"Hello World","err":""}
data: {"type":"interrupted"}
```

---

## 11. 关键技术决策

### 决策 1：Agent 作为独立微服务而非嵌入 CodeRunner

**结论**：独立微服务，不修改 CodeRunner 任何代码。

**原因**：

- Agent 需要理解文章全文、维护多轮对话状态、调用 LLM，职责与 CodeRunner（代码执行调度）完全不同
- 独立部署使两个服务可以独立演进、独立扩展
- 不引入 Eino 等新依赖污染 CodeRunner 主模块

---

### 决策 2：三种对话行为通过 Prompt 指令实现，仅 `propose-execution` 为 Go Tool

**结论**：`explain-code`、`debug-code`、`generate-tests` 不实现为 Go Tool；`propose-execution` 作为唯一 Go Tool 通过 Eino Tool 接口注册。

**原因**：

- 前三种行为本质上只是"取代码块数据给 LLM"——而博客全文和代码块已在首次 `/chat` 时嵌入 System Prompt，LLM 直接可见，tool call 取数据是多余的
- 去掉三个 Go Tool 大幅简化实现，减少 ReAct 轮次，降低延迟
- `propose-execution` 有真实副作用（生成 proposal_id、写入 Session、推送 SSE 事件），必须作为 Tool 实现

---

### 决策 3：`POST /confirm` 立即返回，gRPC Execute 异步执行

**结论**：`/confirm` 返回 202 Accepted，在后台 goroutine 中异步调用 CodeRunner。

**原因**：

- CodeRunner 执行可能耗时数秒，同步等待会阻塞 HTTP 连接
- 执行结果已有异步回调机制（`/internal/callback`），天然适合异步模式
- 与 CodeRunner 原有的"提交即接受，结果走回调"语义一致

---

### 决策 4：SSE 连接在 `done` 后保持打开

**结论**：`done` 仅表示本轮 AI 回复完成，SSE 连接持续存活，等待异步 `execution_result`。

**原因**：

- 用户确认执行后，执行结果需通过 SSE 推送。若 `done` 关闭连接，`execution_result` 无法送达
- 用户提交下一条消息时会发起新的 `/chat`，自然通过 `interrupted` 替换旧连接

---

### 决策 5：CodeRunner Token 23 小时自动刷新

**结论**：Token Manager 在后台定时刷新，刷新失败时沿用旧 Token 并打印警告。

**原因**：

- CodeRunner JWT 有效期 24 小时，静态 Token 在配置文件中会在 24 小时后失效
- 服务不应因 Token 过期在夜间静默崩溃
- 刷新失败降级而非崩溃，保留最后已知有效 Token 争取时间

---

## 12. 迭代计划

### Iter 0：最窄端到端通路（~3d）

**目标**：走通从 `/chat` 到 SSE 流式回复的完整链路，不涉及工具调用。

**范围**：

- 服务启动、Gin 路由、API Key 鉴权
- Eino 框架接入，Claude 流式调用
- Session 创建与 SmartMemory 基础注入
- SSE 推送：`session`、`chunk`、`done` 事件

**不做**：工具调用、CodeRunner 集成、Proposal 机制

**验证**：发送"帮我解释一下 goroutine 是什么"→ SSE 收到流式文本回复 + `done` 事件，`session_id` 正确返回。

---

### Iter 1：完整工具调用（~4d）

**在 Iter 0 基础上新增**：

- `propose-execution` Go Tool 注册到 Eino ReAct
- 文章上下文（`article_ctx`）及三种对话行为指令注入 System Prompt
- `propose-execution` 写入 Proposal，推送 `proposal` SSE 事件
- 上下文压缩（超过 8000 token 时压缩历史）
- `interrupted` 机制（新 `/chat` 替换旧 SSE 连接）

**不做**：用户确认执行（`/confirm`）、CodeRunner 集成

**验证**：传入包含 Go 代码的文章 → 问"这段代码为什么会 panic？" → Agent 直接基于 prompt 中的代码分析，给出修复建议，调用 `propose-execution` Tool → 前端收到 `proposal` 事件（含代码和说明）。

---

### Iter 2：执行确认与结果回调（~3d）

**在 Iter 1 基础上新增**：

- `POST /confirm` 实现（校验 session/proposal/幂等/过期）
- CodeRunner gRPC 客户端（含 Token Manager 23h 刷新）
- `POST /internal/callback` 处理（token 验证、结果写入 Session、SSE 推送 `execution_result`）
- PendingResults 机制（SSE 断开时暂存结果，下次 `/chat` 注入）

**不做**：水平扩展、持久化存储

**验证**：Agent 给出修复代码 → 前端点击确认 → `/confirm` 返回 202 → CodeRunner 执行 → SSE 收到 `execution_result`（stdout 输出正确）→ 继续追问"为什么这样修能解决问题" → Agent 结合执行结果给出解释。

---

### Iter 3：收尾加固（~2d）

**在 Iter 2 基础上新增**：

- 边界处理：并发 `/chat`、Proposal 过期 410、session 不存在 404、LLM 超步 error 事件
- Prometheus 指标注册（`agent_chat_duration_seconds`、`agent_tool_calls_total`、`agent_sessions_active`）
- System Prompt 调优（根据 Iter 1/2 测试中发现的工具误用场景优化 `Use when` / `NOT for`）
- 单元测试覆盖：Session Store 并发安全、Proposal 幂等、语言标准化全 case

**不做**：新功能

**验证**：运行 `go test ./... -race -count=1` 全部通过；压测并发 `/chat` 无竞态；Prometheus `/metrics` 端点数据正确。

---

| 迭代 | 交付物 | 累计可用能力 | 预估 |
| --- | --- | --- | --- |
| Iter 0 | 服务启动 + SSE 流式回复 | 自然语言对话（无工具） | 3d |
| Iter 1 | propose-execution Tool + 文章上下文 + 对话行为 Prompt | 调试/解释/测试建议 + proposal 事件 | 3d |
| Iter 2 | /confirm + CodeRunner + 回调 | 端到端代码执行与结果反馈 | 3d |
| Iter 3 | 加固 + 指标 + 测试 | 生产就绪 | 2d |
