# 代码学习 Agent — MVP 实现方案

> **状态**：设计草案 v3 · **日期**：2026-04-17 · **上一版**：2026-04-16

---

## 1. 概述

代码学习 Agent 是嵌入 CodeRunner Server 端的 AI 对话模块，基于 **Eino ADK Runner + ChatModelAgent** 构建。Agent 让博客读者可以就文章内的任意代码进行自然语言对话，支持调试报错、解释逻辑、生成测试，并可在用户确认后通过 CodeRunner 现有执行链路实际运行修改后的代码。

**与 v1 方案的核心区别**：Agent 不再是独立微服务，而是作为 `internal/agent/` 模块嵌入 CodeRunner Server 进程，共享 Gin HTTP Server、Logger、Metrics 等基础设施。前端只需对接一套 HTTP API。

**核心流程：**

1. 用户在博客页面打开文章，调用 `POST /agent/chat` 发送首条消息，携带文章内容与所有代码块
2. Handler 创建 Session（checkpoint_id），SSE 开始推送 AgentEvent（含 `session_id`）
3. ChatModelAgent 进入 ReAct 循环，根据用户意图直接回答（解释/调试/测试建议）或调用 `propose-execution` Tool
4. `propose-execution` Tool 调用 `tool.StatefulInterrupt()` 暂停 Agent，AgentEvent 流包含 interrupt 事件（含代码和说明），**SSE 关闭**
5. 前端展示"运行修复版本"按钮，用户点击确认 → `POST /agent/confirm` → 开启新 SSE 流
6. confirm Handler 通过 `CodeExecutor` 接口触发异步执行，阻塞等待 Worker 回调
7. Worker 执行完毕 → HTTP 回调到 `/agent/internal/callback` → 通过 channel 唤醒 confirm Handler
8. confirm Handler 调用 `agent.Resume()` 恢复 Agent，Resume 事件通过 confirm SSE 推送
9. confirm SSE 关闭，用户可继续在 `/agent/chat` 追问

> **关键约束**：每个 SSE 连接对应一个完整的 Agent 生命周期（Run 或 Resume），interrupt 或正常结束时关闭。`/agent/chat` 和 `/agent/confirm` 各自独立返回 SSE 流，无需跨请求桥接。

**MVP 包含：**

- 一个 Go Tool：`propose-execution`（HITL interrupt 式，有真实副作用）
- 三种对话行为（`explain-code`、`debug-code`、`generate-tests`）通过 System Prompt 指令实现，无需 tool call
- Eino ADK summarization 中间件（自动压缩历史上下文）
- Eino ADK reduction 中间件（截断过大的 Tool 输出）
- CodeExecutor 接口对接 Server 内部执行链路
- SSE 流式输出（直接透出 AgentEvent）
- API Key 鉴权、Prometheus 指标

**MVP 不包含：**
- 跨文章持久化记忆
- Agent 水平扩展（当前内存 SessionStore / CheckPointStore 不支持多实例共享）
- 安全审查规则引擎

**文件结构：**

```text
internal/agent/
├── agent.go                         # AgentService：初始化 Runner + ChatModelAgent + 中间件装配
├── executor.go                      # CodeExecutor 接口定义
├── config.go                        # Agent 配置结构体 + Viper 解析
├── handler/
│   ├── chat.go                      # POST /agent/chat → SSE（interrupt 时关闭）
│   ├── confirm.go                   # POST /agent/confirm → SSE（同步执行 → Resume → 推送）
│   └── middleware.go                # API Key 鉴权中间件
├── session/
│   └── store.go                     # SessionStore（JSONL 文件持久化 + TTL）
├── checkpoint/
│   └── store.go                     # CheckPointStore（HITL interrupt 状态，Runner 管理）
├── tools/
│   └── propose_execution.go         # HITL interrupt 式 Tool + 语言标准化
└── ai/
    ├── provider.go                  # AI Model 工厂（Claude / OpenAI）
    └── claude.go                    # Claude model.BaseChatModel 实现
```

---

## 2. 架构

| 组件 | 选型 |
| --- | --- |
| Agent 框架 | cloudwego/eino ADK Runner + `ChatModelAgent` |
| Tool 运行时 | Eino `tool.BaseTool` 接口，Go 原生实现 |
| 上下文压缩 | Eino ADK `summarization` 中间件（自动触发） |
| 输出截断 | Eino ADK `reduction` 中间件（大 Tool 输出 offload） |
| HITL 机制 | Eino ADK `tool.StatefulInterrupt` / `agent.Resume` |
| 流式输出 | SSE（Gin `c.Stream`），直接透出 `AgentEvent` |
| AI 接入 | Eino `model.BaseChatModel`，默认 Claude Opus 4.6，可切换 OpenAI |
| 代码执行 | `CodeExecutor` 接口 → Server 内部执行链路（进程内调用） |
| HTTP 框架 | Gin（复用 Server 现有实例，端口 50012） |
| 指标监控 | Prometheus（复用现有 client_golang） |

**架构链路：**

```text
博客前端 (HTTP + SSE)
    │
    ▼
Gin Router（/agent/chat · /agent/confirm · /agent/internal/callback）
    │  API Key 鉴权中间件
    ▼
AgentService（agent.go）
    ├── runner.Run(ctx, messages, WithCheckPointID(sid))  # 首次对话
    │   ├── Claude / OpenAI BaseChatModel（流式）
    │   ├── summarization 中间件              # 自动压缩历史
    │   ├── reduction 中间件                  # 截断大输出
    │   └── propose-execution Tool
    │       └── tool.StatefulInterrupt()      # HITL 暂停 → SSE 关闭
    ├── runner.Resume(ctx, sid)              # 确认后恢复（confirm SSE 中）
    └── AgentEvent Iterator → SSE 直接透出
          ↑
CodeExecutor 接口 → Server Application Service → WS execute_sync → Worker → Docker → WS result
    └── confirm.go 同步拿到结果 → runner.Resume()
```

---

## 3. Skill 设计（对话行为与工具）

前三种对话行为（`explain-code`、`debug-code`、`generate-tests`）**不实现为 Go Tool**。博客全文和所有代码块已在首次 `/agent/chat` 时嵌入 System Prompt（ChatModelAgentConfig.Instruction），LLM 可直接引用，无需 tool call 取数据。对应的行为指令写入 Instruction，指导 LLM 在合适时机输出相应内容。

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
行为：直接引用已嵌入的代码块，生成边界测试代码，随后调用 propose-execution Tool 提交执行
```

---

### Skill 4：`propose-execution`（HITL Interrupt Tool）

当 LLM 准备好一段具体可运行的代码时调用。Tool 通过 `tool.StatefulInterrupt()` 暂停 Agent，Eino 自动将 interrupt 事件推入 AgentEvent 流（前端通过 SSE 收到），展示"运行修复版本"按钮。用户通过 `POST /agent/confirm` 确认后，`callback.go` 调用 `agent.Resume()` 恢复 Agent 循环。

```text
Use when: LLM 生成了修复后的代码，需要用户确认是否运行；LLM 生成了测试代码，需要实际执行验证
NOT for: 尚未完成的代码片段；猜测性代码；解释用途（无需运行）
```

Tool 实现：

- 输入：`new_code`（代码内容），`language`（必须为 CodeRunner 支持的精确字符串：golang / python / javascript / java / c），`description`（LLM 对本次修改的简短说明）
- 执行：语言标准化校验（`go`/`Go` → `golang` 等）；调用 `tool.StatefulInterrupt()` 暂停 Agent，将 proposal 信息（代码、语言、说明、proposal_id）作为 interrupt data 传出
- 恢复：`/agent/confirm` + CodeRunner 执行 + callback 到达后，`agent.Resume()` 将执行结果注入 Agent 上下文

---

## 4. 核心推理逻辑

**Runner + ChatModelAgent 配置：**

```go
chatAgent, _ := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
    Name:        "code-learning-agent",
    Instruction: systemPrompt,  // 文章上下文 + 代码块 + 行为指令
    Model:       claudeModel,   // model.BaseChatModel
    ToolsConfig: adk.ToolsConfig{
        Tools: []tool.BaseTool{proposeExecutionTool},
    },
    MaxIterations: 10,
    Handlers:      []adk.ChatModelAgentMiddleware{summarizationMW, reductionMW},
})

runner := adk.NewRunner(ctx, adk.RunnerConfig{
    Agent:           chatAgent,
    EnableStreaming:  true,
    CheckPointStore: checkPointStore,
})
```

**LLM 输入（每轮）：**

- Instruction：内嵌文章全文 + 所有代码块列表（block_id、语言、代码内容）+ 三种对话行为的 `Use when` / `NOT for` 指令 + `propose-execution` 工具说明
- 从 SessionStore 读取的历史对话（summarization 中间件超阈值时自动压缩）
- Resume 时 Runner 自动从 CheckPointStore 恢复 interrupt 状态

**LLM 决策：**

- 纯回答（`finish_reason: stop`）→ AgentEvent 流式输出文本，ReAct 循环结束
- 工具调用（`finish_reason: tool_calls`）→ 执行 propose-execution Tool，触发 interrupt
- 达到最大步数（默认 10 步）→ AgentEvent 包含错误，强制退出循环

**歧义处理与意图澄清：**

- 语言不支持 → Tool 返回错误字符串，LLM 告知用户当前支持的语言范围
- 用户提问模糊 → LLM 直接输出澄清问句，不调用任何 Tool
- **意图澄清必须在 propose_execution 之前完成**：System Prompt 指令要求 LLM 在不确定用户意图时先问清楚再 propose，因为一旦进入 HITL interrupt 态，用户发新消息会丢弃当前 proposal 重新开始

---

## 5. 上下文管理

使用 Eino ADK `summarization` 中间件，作为 `ChatModelAgentMiddleware` 注册到 `Handlers`：

```go
summarizationMW, _ := summarization.New(ctx, &summarization.Config{
    Model: claudeModel,
    Trigger: &summarization.TriggerCondition{
        ContextTokens: 8000,
    },
})
```

**行为：**

- 每次 ChatModel 调用前自动检查历史消息 token 数
- 超过 8000 token 时触发压缩：保留最近消息，对更早历史生成摘要替换
- 文章上下文在 Instruction 中，不参与压缩
- 完全由 Eino 框架管理，无需自定义代码

**大 Tool 输出截断**（reduction 中间件）：

```go
reductionMW, _ := reduction.New(ctx, &reduction.Config{
    MaxLengthForTrunc: 50000,
    MaxTokensForClear: 160000,
})
```

- 截断阶段：Tool 输出超过 50000 字符时自动截断
- 清理阶段：总 token 超过 160000 时 offload 历史 Tool 结果

---

## 6. 用户意图澄清

**设计原则：意图澄清在 propose_execution 之前完成，不在 HITL interrupt 之后。**

System Prompt 指令要求 LLM：在用户意图不明确时（如"帮我看看"、"能不能修一下"）先通过对话澄清，确认 **要改什么代码** 和 **为什么改** 后再调用 propose_execution。一旦进入 HITL interrupt 态，用户只有两个选择：确认执行 或 发新消息（丢弃 proposal 重来）。

**触发条件：** 用户提问模���时，LLM 直接输出澄清问句，不调用任何 Tool。

**Agent 循环终止与恢复：**

- LLM 输出纯文本且无 tool_call → ReAct 循环自然结束，AgentEvent 迭代器关闭
- 用户补充信息后发起新的 `/agent/chat` 请求 → 携带同一 `session_id`，从 SessionStore 读取历史后继续推理
- 新 `/agent/chat` 到达时，向旧 SSE 连接发送 interrupted 后替换连接
- **HITL 中新 /chat**：丢弃 pending interrupt，以新消息重新 `Run()`

---

## 7. SSE 事件机制

**核心原则：直接透出 Eino AgentEvent，每个 SSE 对应一个完整 Agent 生命周期。**

**两个 SSE 端点：**

1. **`/agent/chat` SSE**：`agent.Run()` → AgentEvent 流 → interrupt 或正常结束时 SSE 关闭
2. **`/agent/confirm` SSE**：等 callback → `agent.Resume()` → AgentEvent 流 → SSE 关闭

Handler 消费模式一致：

```go
iter := agent.Run(ctx, input)  // 或 agent.Resume(ctx, info)
for {
    event, ok := iter.Next()
    if !ok { break }
    data, _ := json.Marshal(event)
    c.SSEvent("message", string(data))
}
// 迭代器结束 → SSE 关闭 → HTTP 请求完成
```

**AgentEvent 原生字段：**

| 字段 | 含义 |
| --- | --- |
| `AgentName` | Agent 名称 |
| `Content` | LLM 流式文本 delta |
| `ToolCalls` | Tool 调用信息 |
| `ActionType` | 动作类型（如 interrupt） |
| `Action` | 动作详情（含 interrupt data：代码、语言、说明） |
| `Err` | 错误信息 |

**业务补充事件（Handler 层注入，非 AgentEvent）：**

| 事件 | 端点 | 含义 |
| --- | --- | --- |
| `session_created` | `/agent/chat` 首次 | 返回 `session_id` |
| `executing` | `/agent/confirm` 开始 | 告知前端执行已触发 |

所有事件格式为 SSE `data:` JSON 帧。

---

## 8. 关键技术决策

### 决策 1：Agent 嵌入 CodeRunner Server 而非独立微服务

**结论**：Agent 作为 `internal/agent/` 模块嵌入 Server 进程。

**原因**：

- MVP 阶段独立微服务增加运维复杂度（独立端口、独立配置、独立部署），收益不大
- 前端只需对接一套 HTTP API，降低集成复杂度
- 共享 Gin、Logger、Prometheus 等基础设施，避免重复搭建
- 通过 `CodeExecutor` 接口解耦，未来拆分为独立服务时接口层不变

---

### 决策 2：通过 CodeExecutor 接口解耦而非直接调用 gRPC

**结论**：定义 `CodeExecutor` 接口，Server Application Service 实现此接口，Agent 依赖注入。

**原因**：

- Agent 不需要感知 gRPC/WebSocket/Worker 调度等细节
- 进程内调用省去序列化开销和网络跳转
- 接口隔离使未来拆分微服务时只需替换实现（gRPC 客户端），Agent 代码不变

---

### 决策 3：使用 Eino ADK Runner + ChatModelAgent 而非 flow/agent/react

**结论**：使用 `adk.Runner` 包装 `adk.ChatModelAgent` 作为 Agent 核心。

**原因**：

- ChatModelAgent 内建 ReAct 循环、Tool 调用、interrupt/resume，无需自建状态机
- Runner 自动管理 CheckPointStore（interrupt 时保存，Resume 时加载），无需手动处理 checkpoint 序列化
- ADK 中间件生态（summarization、reduction）直接可用
- AgentEvent 流提供标准化事件，SSE 直接透出
- HITL interrupt/resume 天然匹配"提议-确认-执行"流程

---

### 决策 4：三种对话行为通过 Prompt 指令实现，仅 propose-execution 为 Go Tool

**结论**：`explain-code`、`debug-code`、`generate-tests` 不实现为 Go Tool。

**原因**：

- 前三种行为本质上只是"取代码块数据给 LLM"——而博客全文和代码块已在 Instruction 中，LLM 直接可见，tool call 取数据是多余的
- 去掉三个 Tool 大幅简化实现，减少 ReAct 轮次，降低延迟
- `propose-execution` 有真实副作用（HITL interrupt），必须作为 Tool 实现

---

### 决策 5：SSE 直接透出 AgentEvent，不自定义事件体系

**结论**：SSE 帧直接序列化 Eino `AgentEvent`，仅补充 2 个业务事件。

**原因**：

- AgentEvent 已覆盖 LLM 流式输出、tool 调用、interrupt、error 等所有 Agent 生命周期事件
- 自定义事件体系需要在 Agent 和 SSE 之间做映射转换，增加维护负担
- 前端可复用 Eino 生态的 SSE 客户端工具
- 仅 `session_created`（连接建立）和 `interrupted`（连接替换）需要业务层补充

---

### 决策 6：/agent/confirm 返回 SSE 流，对齐 Eino HITL 推荐实践

**结论**：`/agent/confirm` 返回 SSE 流，confirm Handler 阻塞等待 callback → Resume → 流式推送结果 → SSE 关闭。

**原因**：

- Eino 推荐实践：每个 SSE 对应一个完整 Agent 生命周期（Run 或 Resume），interrupt 时 SSE 关闭，Resume 在新请求中启动新 SSE
- 避免跨 HTTP 请求桥接（SessionRegistry / SSEWriter / channel 协调），大幅降低并发复杂度
- callback 桥接只需 `PendingCallbacks`（`proposal_id → chan ExecResult`），一对一 channel，无竞态
- 前端在 confirm 期间持有 SSE 连接，可实时感知执行状态（executing → 结果）

---

## 9. 迭代计划

### Iter 0：最窄端到端通路（~3d）

**目标**：走通从 `/agent/chat` 到 SSE 流式回复的完整链路，不涉及工具调用。

**范围**：

- Agent 模块初始化，注册到 Server 的 Gin 路由
- `adk.Runner` + `ChatModelAgent` 接入，Claude BaseChatModel 流式调用
- SessionStore（JSONL 文件持久化 + TTL）+ CheckPointStore 基础实现
- SSE 推送：`session_created` + AgentEvent 流透出
- Prometheus 基础指标框架（`agent_sessions_active` gauge + `agent_chat_duration_seconds` histogram）

**不做**：工具调用、CodeRunner 集成、HITL 机制

**验证**：发送"帮我解释一下 goroutine 是什么"→ SSE 收到 AgentEvent 流式文本回复，`session_id` 正确返回。

---

### Iter 1：完整工具调用 + HITL（~3d）

**在 Iter 0 基础上新增**：

- `propose-execution` HITL interrupt Tool 注册到 Runner
- 文章上下文（`article_ctx`）及三种对话行为指令注入 Instruction
- summarization 中间件 + reduction 中间件注册
- interrupt 事件通过 AgentEvent 自然推送
- `interrupted` 机制（新 `/agent/chat` 替换旧 SSE 连接）

**不做**：用户确认执行（`/agent/confirm`）、CodeRunner 集成

**验证**：传入包含 Go 代码的文章 → 问"这段代码为什么会 panic？" → Agent 分析并给出修复建议 → 调用 `propose-execution` Tool → SSE 收到 interrupt 事件（含代码和说明）。

---

### Iter 2：执行确认与同步执行模式（~3d）

**在 Iter 1 基础上新增**：

- **CodeRunner 同步执行模式**（前置依赖）：
  - `protocol/message.go` 新增 `execute_sync` / `result` 消息类型
  - Server 端 `websocket/server/client.go` 新增 `SendSync()` 方法
  - Worker 端根据消息类型选择 WebSocket 回传（sync）或 HTTP callback（async）
  - `application/service/server/service.go` 新增 `ExecuteSync()` 方法
- `POST /agent/confirm` SSE 实现（校验 session/proposal/幂等/过期）
- `CodeExecutor` 接口定义 + Server Application Service 实现（调用 `ExecuteSync`）
- confirm Handler 同步执行 → `runner.Resume()` → SSE 推送

**不做**：水平扩展、持久化存储

**验证**：Agent 给出修复代码 → interrupt 事件到达 → chat SSE 关闭 → 前端确认 → `/agent/confirm` 开启新 SSE → 推送 `executing` → Worker 同步执行完成 → Agent Resume → SSE 推送 AgentEvent（含执行结果分析）→ confirm SSE 关闭 → 继续在 `/agent/chat` 追问"为什么这样修能解决问题" → Agent 结合执行结果给出解释。

---

### Iter 3：收尾加固（~2d）

**在 Iter 2 基础上新增**：

- 边界处理：并发 `/agent/chat`、Proposal 过期 410、session 不存在 404、LLM 超步 error 事件
- Prometheus 补充指标（`agent_tool_calls_total`，基础指标已在 Iter 0 埋入）
- System Prompt 调优（根据 Iter 1/2 测试中发现的工具误用场景优化 `Use when` / `NOT for`）
- 单元测试覆盖：SessionStore / CheckPointStore 并发安全、语言标准化全 case

**不做**：新功能

**验证**：运行 `go test ./... -race -count=1` 全部通过；压测并发 `/agent/chat` 无竞态；Prometheus `/metrics` 端点数据正确。

---

| 迭代 | 交付物 | 累计可用能力 | 预估 |
| --- | --- | --- | --- |
| Iter 0 | 服务启动 + SSE 流式回复 | 自然语言对话（无工具） | 3d |
| Iter 1 | propose-execution HITL Tool + 中间件 + Prompt 行为 | 调试/解释/测试建议 + interrupt 事件 | 3d |
| Iter 2 | CodeRunner 同步模式 + /confirm SSE + CodeExecutor + Resume | 端到端代码执行与结果反馈 | 3d |
| Iter 3 | 加固 + 指标 + 测试 | 生产就绪 | 2d |
