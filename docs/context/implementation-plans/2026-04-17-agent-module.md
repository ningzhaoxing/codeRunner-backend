# Code Learning Agent Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 CodeRunner Server 内嵌入基于 Eino ADK Runner + ChatModelAgent 的代码学习 Agent，提供 `/agent/chat` 和 `/agent/confirm` 两个 SSE API，支持文章级多轮对话、HITL 执行确认、执行后 Resume。

**Architecture:** Agent 模块作为 `internal/agent/` 独立子模块嵌入现有 Server 进程。普通多轮对话使用 `SessionStore`（JSONL）管理历史消息并在每次 `runner.Run()` 前显式传入；HITL interrupt/resume 使用 `CheckPointStore` 由 Runner 自动管理。代码执行通过前置依赖的 CodeRunner 同步执行模式（`ExecuteSync`）完成，confirm Handler 同步拿到结果后直接 `runner.ResumeWithParams()`。

**Tech Stack:** Go 1.23, Gin, cloudwego/eino, Eino ADK Runner + ChatModelAgent, JSONL file storage, Prometheus, Zap

**前置依赖:** `docs/context/implementation-plans/2026-04-17-coderunner-sync-mode.md` 必须先完成。

---

## File Structure

**Create / modify the following files:**

### Agent module
- Create: `internal/agent/config.go` — `AgentConfig` 结构体，解析顶层 `agent:` 配置
- Create: `internal/agent/executor.go` — `CodeExecutor` 接口 + CodeRunner `ExecuteSync` 适配
- Create: `internal/agent/agent.go` — `AgentService`，负责创建 model、ChatModelAgent、Runner、stores、handlers
- Create: `internal/agent/session/store.go` — JSONL `SessionStore`
- Create: `internal/agent/checkpoint/store.go` — `CheckPointStore` 实现
- Create: `internal/agent/tools/propose_execution.go` — HITL interrupt Tool + proposal 元数据管理
- Create: `internal/agent/ai/provider.go` — model provider 工厂
- Create: `internal/agent/ai/claude.go` — Claude model.BaseChatModel 初始化
- Create: `internal/agent/handler/middleware.go` — API key 鉴权
- Create: `internal/agent/handler/chat.go` — `/agent/chat` SSE handler
- Create: `internal/agent/handler/confirm.go` — `/agent/confirm` SSE handler

### Server integration
- Modify: `internal/infrastructure/config/initConfig.go` — 扩展 `Config` 加 `Agent` 顶层配置
- Modify: `configs/dev.yaml` — 新增 `agent:` 顶层块
- Modify: `configs/product.yaml` — 新增 `agent:` 顶层块
- Modify: `internal/interfaces/adapter/initialize/app.go` — Server 启动时初始化 Agent 模块
- Modify: `internal/interfaces/adapter/router/router.go` — 注册 `/agent/chat` 和 `/agent/confirm`

### Tests
- Create: `internal/agent/session/store_test.go`
- Create: `internal/agent/checkpoint/store_test.go`
- Create: `internal/agent/tools/propose_execution_test.go`
- Create: `internal/agent/handler/chat_test.go`
- Create: `internal/agent/handler/confirm_test.go`
- Possibly create: `internal/agent/agent_test.go`（如需测试初始化/Prompt 组装）

---

### Task 1: 接入依赖与配置骨架

**Files:**
- Modify: `go.mod`
- Modify: `internal/infrastructure/config/initConfig.go`
- Modify: `configs/dev.yaml`
- Modify: `configs/product.yaml`
- Create: `internal/agent/config.go`

- [ ] **Step 1: 写配置解析失败测试**

```go
// internal/agent/config_test.go
func TestAgentConfig_Defaults(t *testing.T) {
    cfg := DefaultAgentConfig()
    if cfg.MaxSteps != 10 {
        t.Fatalf("MaxSteps = %d, want 10", cfg.MaxSteps)
    }
    if cfg.SessionTTL != time.Hour {
        t.Fatalf("SessionTTL = %v, want 1h", cfg.SessionTTL)
    }
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent/... -run TestAgentConfig_Defaults -v`
Expected: FAIL — `internal/agent/config.go` 不存在

- [ ] **Step 3: 为 go.mod 添加 Eino 依赖**

在 `go.mod` 中新增：

```go
require (
    github.com/cloudwego/eino vX.Y.Z
)
```

并在实现时执行：

Run: `cd codeRunner-backend && go mod tidy`
Expected: 成功拉取依赖，`go.sum` 更新

- [ ] **Step 4: 扩展全局配置结构**

在 `internal/infrastructure/config/initConfig.go` 中新增：

```go
type Config struct {
    Server ServerConfig `yaml:"server"`
    Client ClientConfig `yaml:"client"`
    Logger LoggerConfig `yaml:"log"`
    Agent  AgentConfig  `yaml:"agent"`
}

type AgentConfig struct {
    Enabled    bool   `yaml:"enabled"`
    APIKey     string `yaml:"api_key"`
    Provider   string `yaml:"provider"`
    MaxSteps   int    `yaml:"max_steps"`
    SessionTTL int    `yaml:"session_ttl"`
    ProposalTTL int   `yaml:"proposal_ttl"`
    Summarization struct {
        TriggerTokens int `yaml:"trigger_tokens"`
    } `yaml:"summarization"`
    Reduction struct {
        MaxLengthForTrunc int `yaml:"max_length_for_trunc"`
        MaxTokensForClear int `yaml:"max_tokens_for_clear"`
    } `yaml:"reduction"`
    Claude struct {
        APIKey string `yaml:"api_key"`
        Model  string `yaml:"model"`
    } `yaml:"claude"`
    OpenAI struct {
        APIKey string `yaml:"api_key"`
        Model  string `yaml:"model"`
    } `yaml:"openai"`
}
```

- [ ] **Step 5: 在 `internal/agent/config.go` 封装默认值和便捷访问**

```go
func DefaultAgentConfig() config.AgentConfig {
    return config.AgentConfig{
        Enabled:     false,
        Provider:    "claude",
        MaxSteps:    10,
        SessionTTL:  3600,
        ProposalTTL: 600,
        Summarization: struct{ TriggerTokens int `yaml:"trigger_tokens"` }{TriggerTokens: 8000},
        Reduction: struct {
            MaxLengthForTrunc int `yaml:"max_length_for_trunc"`
            MaxTokensForClear int `yaml:"max_tokens_for_clear"`
        }{MaxLengthForTrunc: 50000, MaxTokensForClear: 160000},
    }
}
```

- [ ] **Step 6: 更新 `configs/dev.yaml` 和 `configs/product.yaml`**

追加：

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

- [ ] **Step 7: 运行配置测试与编译检查**

Run: `cd codeRunner-backend && go test ./internal/agent/... -run TestAgentConfig_Defaults -v && go build ./...`
Expected: PASS，编译通过

- [ ] **Step 8: Commit**

```bash
cd codeRunner-backend
git add go.mod go.sum internal/infrastructure/config/initConfig.go configs/dev.yaml configs/product.yaml internal/agent/config.go internal/agent/config_test.go
git commit -m "feat: add agent config and Eino dependencies"
```

---

### Task 2: 实现 JSONL SessionStore 与 CheckPointStore

**Files:**
- Create: `internal/agent/session/store.go`
- Create: `internal/agent/session/store_test.go`
- Create: `internal/agent/checkpoint/store.go`
- Create: `internal/agent/checkpoint/store_test.go`

- [ ] **Step 1: 写 SessionStore JSONL 追加/读取失败测试**

```go
func TestSessionStore_AppendAndReadMessages(t *testing.T) {
    dir := t.TempDir()
    store := NewSessionStore(dir, time.Hour)

    sid := "session-1"
    err := store.Create(sid, "system prompt")
    if err != nil { t.Fatal(err) }

    err = store.Append(sid,
        schema.UserMessage("为什么 panic？"),
        schema.AssistantMessage("因为越界", nil),
    )
    if err != nil { t.Fatal(err) }

    msgs, err := store.GetMessages(sid)
    if err != nil { t.Fatal(err) }
    if len(msgs) != 2 { t.Fatalf("len(msgs) = %d, want 2", len(msgs)) }
}
```

- [ ] **Step 2: 写 CheckPointStore put/get 失败测试**

```go
func TestCheckPointStore_SetGet(t *testing.T) {
    store := NewMemoryCheckPointStore(time.Hour)
    data := []byte("checkpoint-data")
    if err := store.Set(context.Background(), "sid-1", data); err != nil { t.Fatal(err) }
    got, ok, err := store.Get(context.Background(), "sid-1")
    if err != nil { t.Fatal(err) }
    if !ok { t.Fatal("expected checkpoint to exist") }
    if string(got) != string(data) { t.Fatalf("got %q, want %q", got, data) }
}
```

- [ ] **Step 3: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent/session ./internal/agent/checkpoint -v`
Expected: FAIL — 包和文件不存在

- [ ] **Step 4: 实现 JSONL SessionStore**

核心结构：

```go
type SessionStore struct {
    baseDir string
    ttl     time.Duration
    mu      sync.Map // sessionID -> *sync.Mutex
}

type SessionMeta struct {
    ID           string    `json:"id"`
    Instruction  string    `json:"instruction"`
    LastActiveAt time.Time `json:"last_active_at"`
}
```

关键点：
- `Create(sessionID, instruction)` 创建 `{baseDir}/{sid}.jsonl.meta` 和空 `{sid}.jsonl`
- `Append()` 以 append 模式逐行写 `schema.Message` JSON
- `GetMessages()` 逐行 scan 并 `json.Unmarshal`
- 每个 session 用 `*sync.Mutex` 做文件级串行化
- 清理 goroutine 定期删除超时 `.jsonl` + `.meta`

- [ ] **Step 5: 实现 CheckPointStore**

```go
type MemoryCheckPointStore struct {
    ttl time.Duration
    data sync.Map // sessionID -> item{payload []byte, updatedAt time.Time}
    stop chan struct{}
}
```

关键点：
- 实现 `core.CheckPointStore` 接口的 `Get` 和 `Set`（注意：方法名是 `Set` 不是 `Put`）
- 后台 ticker 清理 TTL 过期条目
- 提供 `Close()` 供优雅退出

- [ ] **Step 6: 运行单元测试**

Run: `cd codeRunner-backend && go test ./internal/agent/session ./internal/agent/checkpoint -v -race`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd codeRunner-backend
git add internal/agent/session/store.go internal/agent/session/store_test.go internal/agent/checkpoint/store.go internal/agent/checkpoint/store_test.go
git commit -m "feat: add JSONL session store and checkpoint store"
```

---

### Task 3: 实现 AI Provider、ChatModelAgent 与 Runner 装配

**Files:**
- Create: `internal/agent/ai/provider.go`
- Create: `internal/agent/ai/claude.go`
- Create: `internal/agent/agent.go`
- Test: `internal/agent/agent_test.go`

- [ ] **Step 1: 写 AgentService 初始化失败测试**

```go
func TestNewAgentService_RequiresProviderAPIKey(t *testing.T) {
    cfg := config.AgentConfig{Enabled: true, Provider: "claude"}
    _, err := NewAgentService(context.Background(), cfg, nil)
    if err == nil {
        t.Fatal("expected error when Claude API key missing")
    }
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent -run TestNewAgentService_RequiresProviderAPIKey -v`
Expected: FAIL — `NewAgentService` 未定义

- [ ] **Step 3: 实现 AI provider 工厂**

`internal/agent/ai/provider.go`:

```go
type Provider interface {
    ChatModel() model.BaseChatModel
}

func NewProvider(ctx context.Context, cfg config.AgentConfig) (Provider, error) {
    switch cfg.Provider {
    case "claude":
        return NewClaudeProvider(ctx, cfg)
    default:
        return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
    }
}
```

- [ ] **Step 4: 实现 Claude provider**

`internal/agent/ai/claude.go` 负责：
- 检查 `cfg.Claude.APIKey`
- 创建 Claude chat model（按 Eino 对应 package 实现）
- 返回 `model.BaseChatModel`

代码按 Eino 实际 SDK 写，不要猜 API。如果需要，先 grep `eino-examples` 中 Claude model 初始化模式再实现。

- [ ] **Step 5: 实现 AgentService**

`internal/agent/agent.go` 中定义：

```go
type AgentService struct {
    cfg             config.AgentConfig
    executor        CodeExecutor
    sessionStore    *session.SessionStore
    checkpointStore *checkpoint.MemoryCheckPointStore
    provider        ai.Provider
    runner          *adk.Runner
    chatAgent       *adk.ChatModelAgent
}
```

装配流程：
- provider := ai.NewProvider(...)
- create `proposeExecutionTool`
- create summarization middleware（**必须传 Model 参数**，否则 `Config.check()` 报错）：
  ```go
  summarizationMW, _ := summarization.New(ctx, &summarization.Config{
      Model: provider.ChatModel(),  // 同一个 BaseChatModel
      Trigger: &summarization.TriggerCondition{ContextTokens: cfg.Summarization.TriggerTokens},
  })
  ```
- create reduction middleware（**必须传 Backend 参数**，否则 `New()` 报错）：
  ```go
  reductionMW, _ := reduction.New(ctx, &reduction.Config{
      Backend:           reduction.NewInMemoryBackend(),  // 或 filesystem backend
      MaxLengthForTrunc: cfg.Reduction.MaxLengthForTrunc,
      MaxTokensForClear: int64(cfg.Reduction.MaxTokensForClear),
  })
  ```
- 注意：两个 middleware 都返回 `adk.ChatModelAgentMiddleware`（接口），必须放在 `ChatModelAgentConfig.Handlers`（不是 `Middlewares`，`Middlewares` 是 struct-based legacy API）
- create `chatAgent := adk.NewChatModelAgent(...)`
- create `runner := adk.NewRunner(..., CheckPointStore: checkpointStore)`

- [ ] **Step 6: 运行测试和编译检查**

Run: `cd codeRunner-backend && go test ./internal/agent -run TestNewAgentService_RequiresProviderAPIKey -v && go build ./...`
Expected: PASS，编译通过

- [ ] **Step 7: Commit**

```bash
cd codeRunner-backend
git add internal/agent/ai/provider.go internal/agent/ai/claude.go internal/agent/agent.go internal/agent/agent_test.go
git commit -m "feat: wire ChatModelAgent runner and Claude provider"
```

---

### Task 4: 实现 propose_execution Tool

**Files:**
- Create: `internal/agent/tools/propose_execution.go`
- Create: `internal/agent/tools/propose_execution_test.go`
- Modify: `internal/agent/session/store.go`

- [ ] **Step 1: 写语言标准化失败测试**

```go
func TestNormalizeLanguage(t *testing.T) {
    cases := map[string]string{
        "go": "golang",
        "Go": "golang",
        "python": "python",
        "js": "javascript",
        "Java": "java",
        "c": "c",
    }
    for in, want := range cases {
        got, err := normalizeLanguage(in)
        if err != nil { t.Fatalf("normalizeLanguage(%q): %v", in, err) }
        if got != want { t.Fatalf("got %q, want %q", got, want) }
    }
}

func TestNormalizeLanguage_Unsupported(t *testing.T) {
    _, err := normalizeLanguage("cpp")
    if err == nil { t.Fatal("expected error for cpp") }
}
```

- [ ] **Step 2: 写 interrupt tool 输出测试**

```go
func TestProposeExecutionTool_Info(t *testing.T) {
    tool := NewProposeExecutionTool(nil, 10*time.Minute)
    info, err := tool.Info(context.Background())
    if err != nil { t.Fatal(err) }
    if info.Name != "propose_execution" {
        t.Fatalf("tool name = %q, want propose_execution", info.Name)
    }
}
```

- [ ] **Step 3: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent/tools -v`
Expected: FAIL — tool 文件不存在

- [ ] **Step 4: 扩展 SessionStore 保存 proposal 元数据**

在 `session/store.go` 新增 `.meta` 中 proposal 字段，或单独 `{sid}.proposal.json` 存储：

```go
type ProposalMeta struct {
    ProposalID  string    `json:"proposal_id"`
    Code        string    `json:"code"`
    Language    string    `json:"language"`
    Description string    `json:"description"`
    ExpiresAt   time.Time `json:"expires_at"`
    Confirmed   bool      `json:"confirmed"`
}
```

需要支持：
- SaveProposal(sessionID, meta)
- GetProposal(sessionID, proposalID)
- MarkProposalConfirmed(sessionID, proposalID)

- [ ] **Step 5: 实现 Tool**

核心：

```go
func (t *ProposeExecutionTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
    var input Input
    if err := sonic.UnmarshalString(argumentsInJSON, &input); err != nil { return "", err }

    normalized, err := normalizeLanguage(input.Language)
    if err != nil { return "", err }

    proposalID := uuid.NewString()
    // 保存 proposal 元数据到 SessionStore
    // sessionID 从 ctx 里的 session values 取出

    proposalInfo := &ProposalInfo{
        ProposalID:  proposalID,
        Code:        input.NewCode,
        Language:    normalized,
        Description: input.Description,
    }
    // info = 用户可见的 interrupt 信息（前端展示用）
    // state = 内部状态（gob 序列化，Resume 时通过 GetInterruptState 恢复）
    return "", tool.StatefulInterrupt(ctx, proposalInfo, proposalInfo)
}
```

- [ ] **Step 6: 运行测试**

Run: `cd codeRunner-backend && go test ./internal/agent/tools ./internal/agent/session -v -race`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd codeRunner-backend
git add internal/agent/tools/propose_execution.go internal/agent/tools/propose_execution_test.go internal/agent/session/store.go
git commit -m "feat: add propose_execution interrupt tool"
```

---

### Task 5: 实现 `/agent/chat` SSE handler

**Files:**
- Create: `internal/agent/handler/chat.go`
- Create: `internal/agent/handler/middleware.go`
- Create: `internal/agent/handler/chat_test.go`
- Modify: `internal/interfaces/adapter/router/router.go`
- Modify: `internal/interfaces/adapter/initialize/app.go`

- [ ] **Step 1: 写 API key 鉴权失败测试**

```go
func TestAgentAPIKeyMiddleware_Unauthorized(t *testing.T) {
    r := gin.New()
    r.POST("/agent/chat", AgentAPIKeyMiddleware("secret"), func(c *gin.Context) {
        c.Status(http.StatusOK)
    })

    req := httptest.NewRequest(http.MethodPost, "/agent/chat", strings.NewReader(`{}`))
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusUnauthorized {
        t.Fatalf("status = %d, want 401", w.Code)
    }
}
```

- [ ] **Step 2: 写 `/agent/chat` 首次创建 session 测试**

```go
func TestChatHandler_CreatesSessionAndStreams(t *testing.T) {
    // 构造 fake AgentService，其 runner.Run 返回一条 assistant 内容后结束
    // 发 POST /agent/chat，body 中 session_id 为空，article_ctx 非空
    // 断言 SSE 中含 session_created 事件
}
```

- [ ] **Step 3: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent/handler -v`
Expected: FAIL — handler 文件不存在

- [ ] **Step 4: 实现 API key middleware**

```go
func AgentAPIKeyMiddleware(expected string) gin.HandlerFunc {
    return func(c *gin.Context) {
        if c.GetHeader("X-Agent-API-Key") != expected {
            c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

- [ ] **Step 5: 实现 chat handler**

核心逻辑：
- 解析 body：`session_id`, `user_message`, `article_ctx`
- 按三种模式处理（创建 / 继续 / 重置）
- 从 SessionStore 读历史，拼接 `schema.UserMessage(userMessage)`
- 调 `runner.Run(ctx, allMessages, adk.WithCheckPointID(sessionID))`
- SSE 先写 `session_created`（仅首次）
- 遍历 `iter.Next()`，逐帧 `c.SSEvent("message", ...)`
- 收集 assistant 最终消息并 `SessionStore.Append(sessionID, userMsg, assistantMsg)`

- [ ] **Step 6: 在 router.go 注册路由，在 initialize/app.go 初始化 Agent 模块**

`router.go`：

```go
r.POST("/agent/chat", controller.APIs.Agent.Chat())
r.POST("/agent/confirm", controller.APIs.Agent.Confirm())
```

`initialize/app.go`：
- 在 `RunServer()` 中，读取 agent 配置
- `if c.Agent.Enabled { ... }` 初始化 AgentService 并注册到 controller container

- [ ] **Step 7: 运行 handler 测试与编译检查**

Run: `cd codeRunner-backend && go test ./internal/agent/handler -v && go build ./...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
cd codeRunner-backend
git add internal/agent/handler/chat.go internal/agent/handler/middleware.go internal/agent/handler/chat_test.go internal/interfaces/adapter/router/router.go internal/interfaces/adapter/initialize/app.go
git commit -m "feat: add agent chat SSE endpoint"
```

---

### Task 6: 实现 `/agent/confirm` SSE handler 与 Resume 流程

**Files:**
- Create: `internal/agent/handler/confirm.go`
- Create: `internal/agent/handler/confirm_test.go`
- Create: `internal/agent/executor.go`
- Modify: `internal/agent/agent.go`

- [ ] **Step 1: 写 proposal 校验失败测试**

```go
func TestConfirmHandler_ProposalExpired(t *testing.T) {
    // SessionStore 中预置一个 expires_at 已过期的 proposal
    // POST /agent/confirm
    // 断言 410 Gone
}
```

- [ ] **Step 2: 写 confirm SSE happy path 测试**

```go
func TestConfirmHandler_StreamExecutingThenResume(t *testing.T) {
    // fake executor 同步返回 ExecResult{Result: "Hello\n", Err: ""}
    // fake runner.ResumeWithParams 返回一条 assistant 分析消息
    // 断言 SSE 先有 executing，再有 AgentEvent content
}
```

- [ ] **Step 3: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/agent/handler -run TestConfirmHandler -v`
Expected: FAIL — confirm handler 未实现

- [ ] **Step 4: 实现 CodeExecutor 适配**

`internal/agent/executor.go`:

```go
type CodeExecutor interface {
    Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error)
}

type ExecuteRequest struct {
    ProposalID string
    Code       string
    Language   string
}

type ExecResult struct {
    Result string
    Err    string
}

type CodeRunnerExecutor struct {
    serverService server.ServerService
    timeout       time.Duration
}

func (e *CodeRunnerExecutor) Execute(ctx context.Context, req ExecuteRequest) (ExecResult, error) {
    resp, err := e.serverService.ExecuteSync(ctx, &proto.ExecuteRequest{
        Id:        req.ProposalID,
        Uid:       0,
        Language:  req.Language,
        CodeBlock: req.Code,
    }, e.timeout)
    if err != nil {
        return ExecResult{}, err
    }
    return ExecResult{Result: resp.Result, Err: resp.Err}, nil
}
```

- [ ] **Step 5: 实现 confirm handler**

核心逻辑：
- 从 SessionStore 读取 proposal 元数据
- 校验不存在 / 已确认 / 已过期
- 先 SSE 写 `{"type":"executing"}`
- `executor.Execute(...)`
- 构造 resume targets / params
- `iter, err := runner.ResumeWithParams(ctx, sessionID, params)`
- 遍历 iter 推 SSE
- 标记 proposal confirmed

- [ ] **Step 6: 运行测试与编译检查**

Run: `cd codeRunner-backend && go test ./internal/agent/handler -run TestConfirmHandler -v && go build ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd codeRunner-backend
git add internal/agent/handler/confirm.go internal/agent/handler/confirm_test.go internal/agent/executor.go internal/agent/agent.go
git commit -m "feat: add agent confirm SSE endpoint and resume flow"
```

---

### Task 7: 加固、指标与全量验证

**Files:**
- Modify: `internal/agent/agent.go`
- Modify: `internal/infrastructure/metrics/metrics.go`
- Possibly modify: `internal/agent/handler/chat.go`
- Possibly modify: `internal/agent/handler/confirm.go`

- [ ] **Step 1: 添加基础 Agent 指标**

在 `internal/infrastructure/metrics/metrics.go` 注册：

```go
var (
    AgentChatDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{Name: "agent_chat_duration_seconds", Help: "Agent chat duration"},
        []string{"status"},
    )
    AgentToolCalls = prometheus.NewCounterVec(
        prometheus.CounterOpts{Name: "agent_tool_calls_total", Help: "Agent tool calls"},
        []string{"tool_name", "status"},
    )
    AgentSessionsActive = prometheus.NewGauge(
        prometheus.GaugeOpts{Name: "agent_sessions_active", Help: "Active agent sessions"},
    )
)
```

- [ ] **Step 2: 在 chat/confirm handler 中打点**

- `chat_duration_seconds{status=success|error}`
- `tool_calls_total{tool_name="propose_execution", status="success|error"}`
- session 创建 / 清理时增减 `agent_sessions_active`

- [ ] **Step 3: 跑全量测试与 race 检查**

Run: `cd codeRunner-backend && go test ./... -race -count=1`
Expected: 全部通过

- [ ] **Step 4: 跑 vet / build**

Run: `cd codeRunner-backend && go vet ./... && go build ./...`
Expected: 无错误

- [ ] **Step 5: 手动验证 golden path**

1. 启动 Server + Worker
2. 准备一段包含 panic 的 Go 代码文章上下文
3. `POST /agent/chat` → 收到分析 + interrupt
4. `POST /agent/confirm` → 收到 `executing` → 收到 Resume 分析
5. 继续 `POST /agent/chat` 追问 → 能读到历史上下文

- [ ] **Step 6: Commit**

```bash
cd codeRunner-backend
git add internal/agent/ internal/infrastructure/metrics/metrics.go internal/interfaces/adapter/router/router.go internal/interfaces/adapter/initialize/app.go internal/infrastructure/config/initConfig.go configs/dev.yaml configs/product.yaml
git commit -m "feat: complete embedded code learning agent module"
```
