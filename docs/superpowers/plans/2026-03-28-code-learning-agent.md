# Code Learning Agent Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone Go microservice that lets blog readers chat with an AI agent about any code in the article — debugging, explanation, test generation — with the agent able to propose and (with user confirmation) run modified code via CodeRunner.

**Architecture:** New Go service at `cmd/agent/` in the existing `codeRunner-siwu` module. Blog frontend calls the agent directly via HTTP + SSE (keep-alive, connection stays open past `done`). Agent calls CodeRunner's existing `Execute` gRPC when code needs to run. CodeRunner is untouched.

**Tech Stack:** Go 1.23, Gin, gRPC (existing), `github.com/anthropics/anthropic-sdk-go`, `github.com/openai/openai-go`, `github.com/google/uuid` (existing), prometheus (existing), zap (existing), Viper (existing)

**Spec:** `docs/superpowers/specs/2026-03-28-agent-design.md`

---

## Parallel Tracks

**Dev A — Infrastructure Track**: config, session store, CodeRunner gRPC client, `/confirm` and `/internal/callback` handlers

**Dev B — AI & Agent Track**: AI provider abstraction, Claude + OpenAI implementations, tools, agent ReAct loop

**Coordination point:** Dev B's Task 8 (tools) depends on Dev A's Task 2 (session store). **Both devs do Task 0 first together to agree on shared interfaces, then split.**

---

## File Map

```text
cmd/agent/main.go                          NEW
configs/agent.yaml                         NEW
internal/agent/config/config.go            NEW
internal/agent/config/config_test.go       NEW
internal/agent/session/types.go            NEW  ← Task 0 (both devs)
internal/agent/session/store.go            NEW
internal/agent/session/store_test.go       NEW
internal/agent/coderunner/client.go        NEW
internal/agent/coderunner/client_test.go   NEW
internal/agent/ai/provider.go              NEW  ← Task 0 (both devs)
internal/agent/ai/claude/claude.go         NEW
internal/agent/ai/claude/claude_test.go    NEW
internal/agent/ai/openai/openai.go         NEW
internal/agent/ai/openai/openai_test.go    NEW
internal/agent/tools/tools.go              NEW
internal/agent/tools/tools_test.go         NEW
internal/agent/service/agent.go            NEW
internal/agent/service/agent_test.go       NEW
internal/agent/handler/chat.go             NEW
internal/agent/handler/chat_test.go        NEW
internal/agent/handler/confirm.go          NEW
internal/agent/handler/confirm_test.go     NEW
internal/agent/handler/callback.go         NEW
internal/agent/handler/callback_test.go    NEW
internal/agent/handler/middleware.go       NEW
internal/agent/router/router.go            NEW
```

---

## Pre-Flight: Add Dependencies

Before any task, both devs run once:

```bash
go get github.com/anthropics/anthropic-sdk-go
go get github.com/openai/openai-go
go mod tidy
git add go.mod go.sum
git commit -m "chore: add anthropic and openai SDK dependencies"
```

---

## Task 0: Shared Interface Definitions (Both Devs — Do First)

**Files:**
- Create: `internal/agent/session/types.go`
- Create: `internal/agent/ai/provider.go`

- [ ] **Step 1: Create session types**

```go
// internal/agent/session/types.go
package session

import (
	"sync"
	"time"
)

// SSEEvent is marshalled flat: each event type sets its own top-level keys.
// Use map[string]any so different event types can have different shapes.
type SSEEvent = map[string]any

type ArticleContext struct {
	ArticleID      string      `json:"article_id"`
	ArticleContent string      `json:"article_content"`
	CodeBlocks     []CodeBlock `json:"code_blocks"`
}

type CodeBlock struct {
	BlockID  string `json:"block_id"`
	Language string `json:"language"`
	Code     string `json:"code"`
}

type Proposal struct {
	ID            string
	Code          string
	Language      string
	Description   string
	CallbackToken string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	Confirmed     bool
}

type ExecResult struct {
	ProposalID string
	Result     string
	Err        string
	ReceivedAt time.Time
}

type Message struct {
	Role    string `json:"role"`    // "user" | "assistant" | "tool"
	Content string `json:"content"`
	Name    string `json:"name,omitempty"` // tool name when role=="tool"
}

// AgentSession holds all state for one article reading session.
// Mu is exported so packages outside `session` can lock it directly.
type AgentSession struct {
	Mu               sync.RWMutex
	ID               string
	ArticleID        string
	ArticleCtx       ArticleContext
	Messages         []Message
	ExecutionResults map[string]ExecResult // key: "proposal:{pid}"
	Proposals        map[string]Proposal
	SSEChan          chan SSEEvent // nil when no active connection
	PendingResults   []ExecResult // buffered when SSEChan is nil
	CreatedAt        time.Time
	LastActiveAt     time.Time
	TTL              time.Duration
}
```

- [ ] **Step 2: Create AI provider interface**

```go
// internal/agent/ai/provider.go
package ai

import (
	"context"
	"encoding/json"
)

type ToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ChatChunk struct {
	Content      string
	ToolCall     *ToolCall
	FinishReason string // "stop" | "tool_calls" | "max_steps" | "error"
	Err          error
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type ChatRequest struct {
	Messages []any  // []session.Message cast to any, providers handle conversion
	Tools    []Tool
	MaxSteps int
	System   string
}

// Provider is the interface both Claude and OpenAI adapters implement.
type Provider interface {
	Chat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/agent/session/types.go internal/agent/ai/provider.go
git commit -m "feat(agent): add shared session types and AI provider interface"
```

---

## Track A — Dev A

---

### Task 1: Config

**Files:**
- Create: `configs/agent.yaml`
- Create: `internal/agent/config/config.go`
- Create: `internal/agent/config/config_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/agent/config/config_test.go
package config_test

import (
	"os"
	"testing"

	"codeRunner-siwu/internal/agent/config"
)

func TestLoad(t *testing.T) {
	os.Setenv("AGENT_API_KEY", "test-key")
	os.Setenv("CLAUDE_API_KEY", "claude-key")
	os.Setenv("CODERUNNER_SERVICE_PASSWORD", "pw")
	defer os.Unsetenv("AGENT_API_KEY")
	defer os.Unsetenv("CLAUDE_API_KEY")
	defer os.Unsetenv("CODERUNNER_SERVICE_PASSWORD")

	cfg, err := config.Load("../../../configs/agent.yaml")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 8081 {
		t.Errorf("want port 8081, got %d", cfg.Server.Port)
	}
	if cfg.Server.APIKey != "test-key" {
		t.Errorf("want AGENT_API_KEY from env, got %q", cfg.Server.APIKey)
	}
}
```

- [ ] **Step 2: Run test — expect FAIL (package doesn't exist)**

```bash
go test ./internal/agent/config/... -v
```

- [ ] **Step 3: Create config template**

```yaml
# configs/agent.yaml
server:
  port: 8081
  internal_base_url: "http://localhost:8081"
  api_key: ""            # set via AGENT_API_KEY env var

coderunner:
  grpc_addr: "localhost:50011"
  service_name: "agent-service"
  service_password: ""   # set via CODERUNNER_SERVICE_PASSWORD env var
  token_refresh_interval: 82800  # 23 hours in seconds

agent:
  provider: claude         # or "openai"
  max_steps: 10
  context_token_limit: 8000
  session_ttl: 3600        # seconds
  proposal_ttl: 600        # seconds
  claude:
    api_key: ""            # set via CLAUDE_API_KEY env var
    model: claude-opus-4-6
  openai:
    api_key: ""            # set via OPENAI_API_KEY env var
    model: gpt-4o
```

- [ ] **Step 4: Implement config**

```go
// internal/agent/config/config.go
package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type ServerConfig struct {
	Port            int    `mapstructure:"port"`
	InternalBaseURL string `mapstructure:"internal_base_url"`
	APIKey          string `mapstructure:"api_key"`
}

type CodeRunnerConfig struct {
	GRPCAddr             string `mapstructure:"grpc_addr"`
	ServiceName          string `mapstructure:"service_name"`
	ServicePassword      string `mapstructure:"service_password"`
	TokenRefreshInterval int    `mapstructure:"token_refresh_interval"` // stored as seconds
}

type ClaudeConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type OpenAIConfig struct {
	APIKey string `mapstructure:"api_key"`
	Model  string `mapstructure:"model"`
}

type AgentConfig struct {
	Provider          string       `mapstructure:"provider"`
	MaxSteps          int          `mapstructure:"max_steps"`
	ContextTokenLimit int          `mapstructure:"context_token_limit"`
	SessionTTL        int          `mapstructure:"session_ttl"`   // seconds
	ProposalTTL       int          `mapstructure:"proposal_ttl"`  // seconds
	Claude            ClaudeConfig `mapstructure:"claude"`
	OpenAI            OpenAIConfig `mapstructure:"openai"`
}

type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	CodeRunner CodeRunnerConfig `mapstructure:"coderunner"`
	Agent      AgentConfig      `mapstructure:"agent"`
}

// Durations returns TTL values as time.Duration.
func (c *Config) SessionTTL() time.Duration  { return time.Duration(c.Agent.SessionTTL) * time.Second }
func (c *Config) ProposalTTL() time.Duration { return time.Duration(c.Agent.ProposalTTL) * time.Second }
func (c *Config) TokenRefreshInterval() time.Duration {
	return time.Duration(c.CodeRunner.TokenRefreshInterval) * time.Second
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	// Map dotted keys to env vars: server.api_key → SERVER_API_KEY
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Explicit bindings for keys that don't follow the automatic convention
	v.BindEnv("server.api_key", "AGENT_API_KEY")
	v.BindEnv("coderunner.service_password", "CODERUNNER_SERVICE_PASSWORD")
	v.BindEnv("agent.claude.api_key", "CLAUDE_API_KEY")
	v.BindEnv("agent.openai.api_key", "OPENAI_API_KEY")

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
```

- [ ] **Step 5: Run test — expect PASS**

```bash
go test ./internal/agent/config/... -v
```

- [ ] **Step 6: Commit**

```bash
git add configs/agent.yaml internal/agent/config/
git commit -m "feat(agent): add config loading with Viper and explicit env var bindings"
```

---

### Task 2: Session Store

**Files:**
- Create: `internal/agent/session/store.go`
- Create: `internal/agent/session/store_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/session/store_test.go
package session_test

import (
	"testing"
	"time"

	"codeRunner-siwu/internal/agent/session"
)

func TestStore_CreateAndGet(t *testing.T) {
	s := session.NewStore(time.Hour)
	ctx := session.ArticleContext{ArticleID: "art-1", ArticleContent: "hello"}
	sess, err := s.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID == "" {
		t.Error("expected non-empty session ID")
	}
	got, err := s.Get(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ArticleID != "art-1" {
		t.Errorf("want art-1, got %s", got.ArticleID)
	}
}

func TestStore_GetNotFound(t *testing.T) {
	s := session.NewStore(time.Hour)
	_, err := s.Get("nonexistent")
	if err != session.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_TTLEviction(t *testing.T) {
	s := session.NewStore(50 * time.Millisecond)
	ctx := session.ArticleContext{ArticleID: "art-1"}
	sess, _ := s.Create(ctx)
	time.Sleep(100 * time.Millisecond)
	s.EvictExpired()
	_, err := s.Get(sess.ID)
	if err != session.ErrNotFound {
		t.Error("expected session to be evicted after TTL")
	}
}

func TestStore_AddProposal(t *testing.T) {
	s := session.NewStore(time.Hour)
	sess, _ := s.Create(session.ArticleContext{ArticleID: "art-1"})

	p, err := s.AddProposal(sess.ID, "fmt.Println()", "golang", "fix print", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.CallbackToken == "" {
		t.Error("expected proposal ID and callback token")
	}
	if p.Confirmed {
		t.Error("new proposal should not be confirmed")
	}
}

func TestStore_ConfirmProposal_Idempotency(t *testing.T) {
	s := session.NewStore(time.Hour)
	sess, _ := s.Create(session.ArticleContext{ArticleID: "art-1"})
	p, _ := s.AddProposal(sess.ID, "code", "golang", "desc", 10*time.Minute)

	if err := s.ConfirmProposal(sess.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmProposal(sess.ID, p.ID); err != session.ErrAlreadyConfirmed {
		t.Errorf("want ErrAlreadyConfirmed, got %v", err)
	}
}

func TestStore_ConfirmProposal_Expired(t *testing.T) {
	s := session.NewStore(time.Hour)
	sess, _ := s.Create(session.ArticleContext{ArticleID: "art-1"})
	p, _ := s.AddProposal(sess.ID, "code", "golang", "desc", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	if err := s.ConfirmProposal(sess.ID, p.ID); err != session.ErrProposalExpired {
		t.Errorf("want ErrProposalExpired, got %v", err)
	}
}

func TestStore_ReplaceSSEChan_ReturnsOld(t *testing.T) {
	s := session.NewStore(time.Hour)
	sess, _ := s.Create(session.ArticleContext{ArticleID: "art-1"})

	oldCh := make(chan session.SSEEvent, 1)
	s.SetSSEChan(sess.ID, oldCh)

	newCh := make(chan session.SSEEvent, 1)
	returned, err := s.ReplaceSSEChan(sess.ID, newCh)
	if err != nil {
		t.Fatal(err)
	}
	if returned != oldCh {
		t.Error("ReplaceSSEChan should return the old channel")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/session/... -v
```

- [ ] **Step 3: Implement store**

```go
// internal/agent/session/store.go
package session

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound         = errors.New("session not found")
	ErrProposalNotFound = errors.New("proposal not found")
	ErrAlreadyConfirmed = errors.New("proposal already confirmed")
	ErrProposalExpired  = errors.New("proposal expired")
)

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*AgentSession
	ttl      time.Duration
}

func NewStore(ttl time.Duration) *Store {
	s := &Store{sessions: make(map[string]*AgentSession), ttl: ttl}
	go s.gcLoop()
	return s
}

func (s *Store) Create(articleCtx ArticleContext) (*AgentSession, error) {
	sess := &AgentSession{
		ID:               uuid.NewString(),
		ArticleID:        articleCtx.ArticleID,
		ArticleCtx:       articleCtx,
		Messages:         []Message{},
		ExecutionResults: make(map[string]ExecResult),
		Proposals:        make(map[string]Proposal),
		PendingResults:   []ExecResult{},
		CreatedAt:        time.Now(),
		LastActiveAt:     time.Now(),
		TTL:              s.ttl,
	}
	s.mu.Lock()
	s.sessions[sess.ID] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Store) Get(sessionID string) (*AgentSession, error) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, ErrNotFound
	}
	return sess, nil
}

func (s *Store) AddProposal(sessionID, code, language, description string, ttl time.Duration) (Proposal, error) {
	sess, err := s.Get(sessionID)
	if err != nil {
		return Proposal{}, err
	}
	now := time.Now()
	p := Proposal{
		ID:            uuid.NewString(),
		Code:          code,
		Language:      language,
		Description:   description,
		CallbackToken: uuid.NewString(),
		CreatedAt:     now,
		ExpiresAt:     now.Add(ttl),
	}
	sess.Mu.Lock()
	sess.Proposals[p.ID] = p
	sess.Mu.Unlock()
	return p, nil
}

func (s *Store) GetProposal(sessionID, proposalID string) (Proposal, error) {
	sess, err := s.Get(sessionID)
	if err != nil {
		return Proposal{}, err
	}
	sess.Mu.RLock()
	p, ok := sess.Proposals[proposalID]
	sess.Mu.RUnlock()
	if !ok {
		return Proposal{}, ErrProposalNotFound
	}
	return p, nil
}

func (s *Store) ConfirmProposal(sessionID, proposalID string) error {
	sess, err := s.Get(sessionID)
	if err != nil {
		return err
	}
	sess.Mu.Lock()
	defer sess.Mu.Unlock()
	p, ok := sess.Proposals[proposalID]
	if !ok {
		return ErrProposalNotFound
	}
	if time.Now().After(p.ExpiresAt) {
		return ErrProposalExpired
	}
	if p.Confirmed {
		return ErrAlreadyConfirmed
	}
	p.Confirmed = true
	sess.Proposals[proposalID] = p
	return nil
}

func (s *Store) SaveExecResult(sessionID string, result ExecResult) error {
	sess, err := s.Get(sessionID)
	if err != nil {
		return err
	}
	key := "proposal:" + result.ProposalID
	sess.Mu.Lock()
	sess.ExecutionResults[key] = result
	if sess.SSEChan == nil {
		sess.PendingResults = append(sess.PendingResults, result)
	}
	sess.Mu.Unlock()
	return nil
}

func (s *Store) PushSSEEvent(sessionID string, event SSEEvent) bool {
	sess, err := s.Get(sessionID)
	if err != nil {
		return false
	}
	sess.Mu.RLock()
	ch := sess.SSEChan
	sess.Mu.RUnlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- event:
		return true
	default:
		return false
	}
}

// SetSSEChan sets the SSEChan without returning the old one (use ReplaceSSEChan if you need the old).
func (s *Store) SetSSEChan(sessionID string, ch chan SSEEvent) error {
	sess, err := s.Get(sessionID)
	if err != nil {
		return err
	}
	sess.Mu.Lock()
	sess.SSEChan = ch
	sess.Mu.Unlock()
	return nil
}

// ReplaceSSEChan replaces the SSEChan and returns the previous channel (may be nil).
func (s *Store) ReplaceSSEChan(sessionID string, newCh chan SSEEvent) (chan SSEEvent, error) {
	sess, err := s.Get(sessionID)
	if err != nil {
		return nil, err
	}
	sess.Mu.Lock()
	old := sess.SSEChan
	sess.SSEChan = newCh
	sess.Mu.Unlock()
	return old, nil
}

func (s *Store) DrainPendingResults(sessionID string) []ExecResult {
	sess, err := s.Get(sessionID)
	if err != nil {
		return nil
	}
	sess.Mu.Lock()
	pending := sess.PendingResults
	sess.PendingResults = []ExecResult{}
	sess.Mu.Unlock()
	return pending
}

func (s *Store) OverrideArticleContext(sessionID string, ctx ArticleContext) error {
	sess, err := s.Get(sessionID)
	if err != nil {
		return err
	}
	sess.Mu.Lock()
	sess.ArticleID = ctx.ArticleID
	sess.ArticleCtx = ctx
	sess.Messages = []Message{} // clear history on context override
	sess.Mu.Unlock()
	return nil
}

func (s *Store) EvictExpired() {
	s.mu.Lock()
	now := time.Now()
	for id, sess := range s.sessions {
		if now.After(sess.CreatedAt.Add(sess.TTL)) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func (s *Store) gcLoop() {
	ticker := time.NewTicker(s.ttl / 2)
	defer ticker.Stop()
	for range ticker.C {
		s.EvictExpired()
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/session/... -v -race
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/session/store.go internal/agent/session/store_test.go
git commit -m "feat(agent): add session store with TTL eviction and proposal management"
```

---

### Task 3: CodeRunner gRPC Client + Token Manager

**Files:**
- Create: `internal/agent/coderunner/client.go`
- Create: `internal/agent/coderunner/client_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/coderunner/client_test.go
package coderunner_test

import (
	"testing"
	"time"

	"codeRunner-siwu/internal/agent/coderunner"
)

func TestNormalizeLanguage(t *testing.T) {
	cases := []struct {
		input    string
		expected string
		wantErr  bool
	}{
		{"go", "golang", false},
		{"Go", "golang", false},
		{"golang", "golang", false},
		{"python", "python", false},
		{"py", "python", false},
		{"Python", "python", false},
		{"javascript", "javascript", false},
		{"js", "javascript", false},
		{"java", "java", false},
		{"c", "c", false},
		{"C", "c", false},
		{"rust", "", true},
		{"typescript", "", true},
	}
	for _, tc := range cases {
		got, err := coderunner.NormalizeLanguage(tc.input)
		if tc.wantErr && err == nil {
			t.Errorf("NormalizeLanguage(%q): want error", tc.input)
		}
		if !tc.wantErr && got != tc.expected {
			t.Errorf("NormalizeLanguage(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestTokenManager_CachesToken(t *testing.T) {
	callCount := 0
	mockGenerate := func() (string, error) {
		callCount++
		return "mock-token", nil
	}
	tm := coderunner.NewTokenManagerWithGenerator(23*time.Hour, mockGenerate)

	token, err := tm.Token()
	if err != nil {
		t.Fatal(err)
	}
	if token != "mock-token" {
		t.Errorf("want mock-token, got %s", token)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call to generate, got %d", callCount)
	}
	// Second call should reuse cached token
	tm.Token()
	if callCount != 1 {
		t.Errorf("expected still 1 call (cached), got %d", callCount)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/coderunner/... -v
```

- [ ] **Step 3: Implement client**

```go
// internal/agent/coderunner/client.go
package coderunner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"codeRunner-siwu/api/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var languageMap = map[string]string{
	"go": "golang", "golang": "golang",
	"python": "python", "py": "python",
	"javascript": "javascript", "js": "javascript",
	"java": "java",
	"c": "c",
}

func NormalizeLanguage(lang string) (string, error) {
	normalized, ok := languageMap[strings.ToLower(lang)]
	if !ok {
		return "", fmt.Errorf("unsupported language: %s (supported: golang, python, javascript, java, c)", lang)
	}
	return normalized, nil
}

type TokenManager struct {
	mu              sync.RWMutex
	token           string
	expiresAt       time.Time
	refreshInterval time.Duration
	generate        func() (string, error)
}

func NewTokenManagerWithGenerator(refreshInterval time.Duration, generate func() (string, error)) *TokenManager {
	return &TokenManager{refreshInterval: refreshInterval, generate: generate}
}

func (tm *TokenManager) Token() (string, error) {
	tm.mu.RLock()
	if tm.token != "" && time.Now().Before(tm.expiresAt) {
		t := tm.token
		tm.mu.RUnlock()
		return t, nil
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.token != "" && time.Now().Before(tm.expiresAt) {
		return tm.token, nil
	}
	token, err := tm.generate()
	if err != nil {
		if tm.token != "" {
			return tm.token, nil // use stale token on refresh failure
		}
		return "", err
	}
	tm.token = token
	tm.expiresAt = time.Now().Add(tm.refreshInterval)
	return tm.token, nil
}

// Executor is the interface other packages use (enables test mocking).
type Executor interface {
	Execute(ctx context.Context, req *proto.ExecuteRequest) error
}

type Client struct {
	conn         *grpc.ClientConn
	execClient   proto.CodeRunnerClient
	tokenManager *TokenManager
}

func NewClient(grpcAddr, serviceName, servicePassword string, refreshInterval time.Duration) (*Client, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	execClient := proto.NewCodeRunnerClient(conn)
	tokenClient := proto.NewTokenIssuerClient(conn)

	generate := func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := tokenClient.GenerateToken(ctx, &proto.GenerateTokenRequest{
			Name:     serviceName,
			Password: servicePassword,
		})
		if err != nil {
			return "", err
		}
		return resp.Token, nil
	}

	c := &Client{conn: conn, execClient: execClient,
		tokenManager: NewTokenManagerWithGenerator(refreshInterval, generate)}
	if _, err := c.tokenManager.Token(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to acquire initial token: %w", err)
	}
	return c, nil
}

func (c *Client) Execute(ctx context.Context, req *proto.ExecuteRequest) error {
	token, err := c.tokenManager.Token()
	if err != nil {
		return fmt.Errorf("no valid token: %w", err)
	}
	md := metadata.Pairs("token", token)
	ctx = metadata.NewOutgoingContext(ctx, md)
	_, err = c.execClient.Execute(ctx, req)
	return err
}

func (c *Client) Close() error { return c.conn.Close() }
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/coderunner/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/coderunner/
git commit -m "feat(agent): add CodeRunner gRPC client with token manager and language normalizer"
```

---

### Task 4: /internal/callback Handler

**Files:**
- Create: `internal/agent/handler/callback.go`
- Create: `internal/agent/handler/callback_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/handler/callback_test.go
package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/agent/session"
)

func setupCallbackTest(t *testing.T) (*handler.CallbackHandler, *session.Store, string, string, string) {
	store := session.NewStore(time.Hour)
	sess, _ := store.Create(session.ArticleContext{ArticleID: "a1"})
	p, _ := store.AddProposal(sess.ID, "code", "golang", "desc", 10*time.Minute)
	h := handler.NewCallbackHandler(store)
	return h, store, sess.ID, p.ID, p.CallbackToken
}

func TestCallback_Valid(t *testing.T) {
	h, store, sessID, propID, token := setupCallbackTest(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/callback", h.Handle)

	body, _ := json.Marshal(proto.ExecuteResponse{
		Id: propID, Result: "Hello World", Err: "",
	})
	url := "/internal/callback?session_id=" + sessID + "&proposal_id=" + propID + "&token=" + token
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}

	pending := store.DrainPendingResults(sessID)
	if len(pending) != 1 {
		t.Errorf("want 1 pending result, got %d", len(pending))
	}
	if pending[0].Result != "Hello World" {
		t.Errorf("want result 'Hello World', got %q", pending[0].Result)
	}
}

func TestCallback_InvalidToken(t *testing.T) {
	h, _, sessID, propID, _ := setupCallbackTest(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/internal/callback", h.Handle)

	body, _ := json.Marshal(proto.ExecuteResponse{Id: propID})
	url := "/internal/callback?session_id=" + sessID + "&proposal_id=" + propID + "&token=wrong-token"
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/handler/... -run TestCallback -v
```

- [ ] **Step 3: Implement callback handler**

```go
// internal/agent/handler/callback.go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/agent/session"
)

type CallbackHandler struct {
	store *session.Store
}

func NewCallbackHandler(store *session.Store) *CallbackHandler {
	return &CallbackHandler{store: store}
}

func (h *CallbackHandler) Handle(c *gin.Context) {
	sessID := c.Query("session_id")
	propID := c.Query("proposal_id")
	token := c.Query("token")

	if sessID == "" || propID == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing query params"})
		return
	}

	p, err := h.store.GetProposal(sessID, propID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if p.CallbackToken != token {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid token"})
		return
	}

	var resp proto.ExecuteResponse
	if err := json.NewDecoder(c.Request.Body).Decode(&resp); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	result := session.ExecResult{
		ProposalID: propID,
		Result:     resp.Result,
		Err:        resp.Err,
		ReceivedAt: time.Now(),
	}
	h.store.SaveExecResult(sessID, result)

	h.store.PushSSEEvent(sessID, session.SSEEvent{
		"type":        "execution_result",
		"proposal_id": propID,
		"result":      result.Result,
		"err":         result.Err,
	})

	c.Status(http.StatusOK)
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/handler/... -run TestCallback -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/handler/callback.go internal/agent/handler/callback_test.go
git commit -m "feat(agent): add internal callback handler with token validation"
```

---

### Task 5: /confirm Handler

**Files:**
- Create: `internal/agent/handler/confirm.go`
- Create: `internal/agent/handler/confirm_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/handler/confirm_test.go
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/agent/session"
)

type mockCRClient struct {
	called    bool
	shouldErr bool
}

func (m *mockCRClient) Execute(_ context.Context, _ *proto.ExecuteRequest) error {
	m.called = true
	if m.shouldErr {
		return errors.New("grpc error")
	}
	return nil
}

func TestConfirm_Success(t *testing.T) {
	store := session.NewStore(time.Hour)
	sess, _ := store.Create(session.ArticleContext{ArticleID: "a1"})
	p, _ := store.AddProposal(sess.ID, "fmt.Println()", "golang", "fix", 10*time.Minute)

	mock := &mockCRClient{}
	h := handler.NewConfirmHandler(store, mock, "http://localhost:8081", 10*time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/confirm", h.Handle)

	body, _ := json.Marshal(map[string]string{"session_id": sess.ID, "proposal_id": p.ID})
	req := httptest.NewRequest(http.MethodPost, "/confirm", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfirm_AlreadyConfirmed(t *testing.T) {
	store := session.NewStore(time.Hour)
	sess, _ := store.Create(session.ArticleContext{ArticleID: "a1"})
	p, _ := store.AddProposal(sess.ID, "code", "golang", "desc", 10*time.Minute)
	store.ConfirmProposal(sess.ID, p.ID)

	mock := &mockCRClient{}
	h := handler.NewConfirmHandler(store, mock, "http://localhost:8081", 10*time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/confirm", h.Handle)

	body, _ := json.Marshal(map[string]string{"session_id": sess.ID, "proposal_id": p.ID})
	req := httptest.NewRequest(http.MethodPost, "/confirm", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("want 409, got %d", w.Code)
	}
}

func TestConfirm_Expired(t *testing.T) {
	store := session.NewStore(time.Hour)
	sess, _ := store.Create(session.ArticleContext{ArticleID: "a1"})
	p, _ := store.AddProposal(sess.ID, "code", "golang", "desc", 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	mock := &mockCRClient{}
	h := handler.NewConfirmHandler(store, mock, "http://localhost:8081", 10*time.Minute)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/confirm", h.Handle)

	body, _ := json.Marshal(map[string]string{"session_id": sess.ID, "proposal_id": p.ID})
	req := httptest.NewRequest(http.MethodPost, "/confirm", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("want 410, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/handler/... -run TestConfirm -v
```

- [ ] **Step 3: Implement confirm handler**

```go
// internal/agent/handler/confirm.go
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/agent/session"
)

// CodeRunnerExecutor is implemented by coderunner.Client.
type CodeRunnerExecutor interface {
	Execute(ctx context.Context, req *proto.ExecuteRequest) error
}

type ConfirmHandler struct {
	store           *session.Store
	crClient        CodeRunnerExecutor
	internalBaseURL string
	proposalTTL     time.Duration
}

func NewConfirmHandler(store *session.Store, crClient CodeRunnerExecutor, internalBaseURL string, proposalTTL time.Duration) *ConfirmHandler {
	return &ConfirmHandler{store: store, crClient: crClient, internalBaseURL: internalBaseURL, proposalTTL: proposalTTL}
}

type confirmRequest struct {
	SessionID  string `json:"session_id" binding:"required"`
	ProposalID string `json:"proposal_id" binding:"required"`
}

func (h *ConfirmHandler) Handle(c *gin.Context) {
	var req confirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.store.ConfirmProposal(req.SessionID, req.ProposalID); err != nil {
		switch {
		case errors.Is(err, session.ErrNotFound), errors.Is(err, session.ErrProposalNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		case errors.Is(err, session.ErrProposalExpired):
			c.JSON(http.StatusGone, gin.H{"error": err.Error()})
		case errors.Is(err, session.ErrAlreadyConfirmed):
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	p, err := h.store.GetProposal(req.SessionID, req.ProposalID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	callbackURL := fmt.Sprintf("%s/internal/callback?session_id=%s&proposal_id=%s&token=%s",
		h.internalBaseURL, req.SessionID, req.ProposalID, p.CallbackToken)

	// Return accepted immediately — Execute runs in background
	c.JSON(http.StatusOK, gin.H{"request_id": req.ProposalID, "status": "accepted"})

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		execReq := &proto.ExecuteRequest{
			Id:          req.ProposalID,
			Uid:         0,
			Language:    p.Language,
			CodeBlock:   p.Code,
			CallBackUrl: callbackURL,
		}
		if err := h.crClient.Execute(ctx, execReq); err != nil {
			h.store.PushSSEEvent(req.SessionID, session.SSEEvent{
				"type":    "error",
				"message": fmt.Sprintf("execution failed: %s", err.Error()),
			})
		}
	}()
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/handler/... -run TestConfirm -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/handler/confirm.go internal/agent/handler/confirm_test.go
git commit -m "feat(agent): add /confirm handler with async gRPC execute"
```

---

## Track B — Dev B

*(Tasks 6–7 start immediately; Task 8 needs session store from Dev A Task 2)*

---

### Task 6: Claude AI Provider

**Files:**
- Create: `internal/agent/ai/claude/claude.go`
- Create: `internal/agent/ai/claude/claude_test.go`

- [ ] **Step 1: Write failing test**

```go
// internal/agent/ai/claude/claude_test.go
package claude_test

import (
	"testing"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/ai/claude"
)

// Compile-time check that Provider satisfies ai.Provider interface.
func TestClaude_ImplementsProvider(t *testing.T) {
	var _ ai.Provider = (*claude.Provider)(nil)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/ai/claude/... -v
```

- [ ] **Step 3: Implement Claude provider**

```go
// internal/agent/ai/claude/claude.go
package claude

import (
	"context"
	"encoding/json"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/session"
)

type Provider struct {
	client *anthropic.Client
	model  string
}

func New(apiKey, model string) *Provider {
	c := anthropic.NewClient(anthropic.WithAPIKey(apiKey))
	return &Provider{client: &c, model: model}
}

func (p *Provider) Chat(ctx context.Context, req ai.ChatRequest) (<-chan ai.ChatChunk, error) {
	ch := make(chan ai.ChatChunk, 16)

	messages := make([]anthropic.MessageParam, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := m.(session.Message)
		switch msg.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
		case "assistant":
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
		// tool results are folded into the next user turn as context
		}
	}

	tools := make([]anthropic.ToolParam, 0, len(req.Tools))
	for _, t := range req.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		tools = append(tools, anthropic.ToolParam{
			Name:        anthropic.String(t.Name),
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.Raw[anthropic.ToolInputSchemaParam](schema),
		})
	}

	go func() {
		defer close(ch)

		params := anthropic.MessageStreamParams{
			Model:     anthropic.Model(p.model),
			Messages:  messages,
			MaxTokens: anthropic.Int(4096),
		}
		if req.System != "" {
			params.System = []anthropic.TextBlockParam{{Text: anthropic.String(req.System)}}
		}
		if len(tools) > 0 {
			params.Tools = tools
		}

		stream := p.client.Messages.Stream(ctx, params)
		for stream.Next() {
			event := stream.Current()
			switch e := event.AsUnion().(type) {
			case anthropic.ContentBlockDeltaEvent:
				if delta, ok := e.Delta.AsUnion().(anthropic.TextDelta); ok {
					ch <- ai.ChatChunk{Content: delta.Text}
				}
			case anthropic.ContentBlockStopEvent:
				msg := stream.Message()
				if int(e.Index) < len(msg.Content) {
					if tc, ok := msg.Content[e.Index].AsUnion().(anthropic.ToolUseBlock); ok {
						input, _ := json.Marshal(tc.Input)
						ch <- ai.ChatChunk{ToolCall: &ai.ToolCall{Name: tc.Name, Input: input}}
					}
				}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- ai.ChatChunk{Err: fmt.Errorf("claude: %w", err), FinishReason: "error"}
			return
		}
		if stream.Message().StopReason == "tool_use" {
			ch <- ai.ChatChunk{FinishReason: "tool_calls"}
		} else {
			ch <- ai.ChatChunk{FinishReason: "stop"}
		}
	}()

	return ch, nil
}
```

- [ ] **Step 4: Run — expect PASS (compile check)**

```bash
go test ./internal/agent/ai/claude/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/ai/claude/
git commit -m "feat(agent): add Claude AI provider"
```

---

### Task 7: OpenAI AI Provider

**Files:**
- Create: `internal/agent/ai/openai/openai.go`
- Create: `internal/agent/ai/openai/openai_test.go`

- [ ] **Step 1: Write test**

```go
// internal/agent/ai/openai/openai_test.go
package openai_test

import (
	"testing"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/ai/openai"
)

func TestOpenAI_ImplementsProvider(t *testing.T) {
	var _ ai.Provider = (*openai.Provider)(nil)
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/ai/openai/... -v
```

- [ ] **Step 3: Implement OpenAI provider**

```go
// internal/agent/ai/openai/openai.go
package openai

import (
	"context"
	"encoding/json"
	"fmt"

	openaiSDK "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/session"
)

type Provider struct {
	client *openaiSDK.Client
	model  string
}

func New(apiKey, model string) *Provider {
	c := openaiSDK.NewClient(option.WithAPIKey(apiKey))
	return &Provider{client: &c, model: model}
}

func (p *Provider) Chat(ctx context.Context, req ai.ChatRequest) (<-chan ai.ChatChunk, error) {
	ch := make(chan ai.ChatChunk, 16)

	messages := make([]openaiSDK.ChatCompletionMessageParamUnion, 0)
	if req.System != "" {
		messages = append(messages, openaiSDK.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		msg := m.(session.Message)
		switch msg.Role {
		case "user":
			messages = append(messages, openaiSDK.UserMessage(msg.Content))
		case "assistant":
			messages = append(messages, openaiSDK.AssistantMessage(msg.Content))
		}
	}

	tools := make([]openaiSDK.ChatCompletionToolParam, 0, len(req.Tools))
	for _, t := range req.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		tools = append(tools, openaiSDK.ChatCompletionToolParam{
			Type: "function",
			Function: openaiSDK.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openaiSDK.String(t.Description),
				Parameters:  openaiSDK.FunctionParameters(json.RawMessage(schema)),
			},
		})
	}

	go func() {
		defer close(ch)

		params := openaiSDK.ChatCompletionNewParams{
			Model:    openaiSDK.ChatModel(p.model),
			Messages: messages,
		}
		if len(tools) > 0 {
			params.Tools = tools
			params.ParallelToolCalls = openaiSDK.Bool(false)
		}

		stream := p.client.Chat.Completions.NewStreaming(ctx, params)
		acc := openaiSDK.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					ch <- ai.ChatChunk{Content: choice.Delta.Content}
				}
			}
		}
		if err := stream.Err(); err != nil {
			ch <- ai.ChatChunk{Err: fmt.Errorf("openai: %w", err), FinishReason: "error"}
			return
		}
		if len(acc.Choices) == 0 {
			ch <- ai.ChatChunk{FinishReason: "stop"}
			return
		}
		if len(acc.Choices[0].Message.ToolCalls) > 0 {
			for _, tc := range acc.Choices[0].Message.ToolCalls {
				ch <- ai.ChatChunk{ToolCall: &ai.ToolCall{
					Name:  tc.Function.Name,
					Input: json.RawMessage(tc.Function.Arguments),
				}}
			}
			ch <- ai.ChatChunk{FinishReason: "tool_calls"}
		} else {
			ch <- ai.ChatChunk{FinishReason: "stop"}
		}
	}()

	return ch, nil
}
```

- [ ] **Step 4: Run — expect PASS**

```bash
go test ./internal/agent/ai/openai/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/ai/openai/
git commit -m "feat(agent): add OpenAI provider"
```

---

### Task 8: Tool Implementations

*Requires Dev A Task 2 (session store) to be merged.*

**Files:**
- Create: `internal/agent/tools/tools.go`
- Create: `internal/agent/tools/tools_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/tools/tools_test.go
package tools_test

import (
	"encoding/json"
	"testing"
	"time"

	"codeRunner-siwu/internal/agent/session"
	"codeRunner-siwu/internal/agent/tools"
)

func makeSession(t *testing.T) (*session.Store, *session.AgentSession) {
	t.Helper()
	store := session.NewStore(time.Hour)
	ctx := session.ArticleContext{
		ArticleID: "art-1",
		CodeBlocks: []session.CodeBlock{
			{BlockID: "b1", Language: "golang", Code: `fmt.Println("hello")`},
		},
	}
	sess, err := store.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return store, sess
}

func TestExplainCode_ValidBlock(t *testing.T) {
	store, sess := makeSession(t)
	registry := tools.NewRegistry(store, 10*time.Minute)
	input, _ := json.Marshal(map[string]string{"block_id": "b1"})
	result, err := registry.Execute(sess.ID, "explain_code", input)
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestExplainCode_InvalidBlock(t *testing.T) {
	store, sess := makeSession(t)
	registry := tools.NewRegistry(store, 10*time.Minute)
	input, _ := json.Marshal(map[string]string{"block_id": "nonexistent"})
	_, err := registry.Execute(sess.ID, "explain_code", input)
	if err == nil {
		t.Error("expected error for invalid block_id")
	}
}

func TestProposeExecution_UnsupportedLanguage(t *testing.T) {
	store, sess := makeSession(t)
	registry := tools.NewRegistry(store, 10*time.Minute)
	input, _ := json.Marshal(map[string]string{
		"new_code": "fn main() {}", "language": "rust", "description": "fix",
	})
	_, err := registry.Execute(sess.ID, "propose_execution", input)
	if err == nil {
		t.Error("expected error for unsupported language 'rust'")
	}
}

func TestProposeExecution_Success(t *testing.T) {
	store, sess := makeSession(t)
	registry := tools.NewRegistry(store, 10*time.Minute)
	input, _ := json.Marshal(map[string]string{
		"new_code": `fmt.Println("fixed")`, "language": "go", "description": "fix print",
	})
	result, err := registry.Execute(sess.ID, "propose_execution", input)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("result not JSON: %v", err)
	}
	if out["proposal_id"] == "" {
		t.Error("expected proposal_id in result")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/tools/... -v
```

- [ ] **Step 3: Implement tools**

```go
// internal/agent/tools/tools.go
package tools

import (
	"encoding/json"
	"fmt"
	"time"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/coderunner"
	"codeRunner-siwu/internal/agent/session"
)

type Registry struct {
	store       *session.Store
	proposalTTL time.Duration
}

func NewRegistry(store *session.Store, proposalTTL time.Duration) *Registry {
	return &Registry{store: store, proposalTTL: proposalTTL}
}

// Definitions returns the tool schemas for inclusion in the AI request.
func (r *Registry) Definitions() []ai.Tool {
	return []ai.Tool{
		{
			Name:        "explain_code",
			Description: "Use when user asks what code does or how it works. NOT for: fixing bugs, generating new code.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"block_id": map[string]string{"type": "string"}},
				"required":   []string{"block_id"},
			},
		},
		{
			Name:        "debug_code",
			Description: "Use when user reports an error or unexpected output. NOT for: explaining working code.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"block_id":      map[string]string{"type": "string"},
					"error_message": map[string]string{"type": "string"},
				},
				"required": []string{"block_id"},
			},
		},
		{
			Name:        "generate_tests",
			Description: "Use when user asks for test cases or edge case coverage. NOT for: running tests.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"block_id": map[string]string{"type": "string"}},
				"required":   []string{"block_id"},
			},
		},
		{
			Name:        "propose_execution",
			Description: "Use when you have concrete, working code ready to run. Language must be one of: golang, python, javascript, java, c. NOT for: speculative code.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"new_code":    map[string]string{"type": "string"},
					"language":   map[string]string{"type": "string"},
					"description": map[string]string{"type": "string", "description": "Brief explanation of changes made"},
				},
				"required": []string{"new_code", "language", "description"},
			},
		},
	}
}

// Execute dispatches a tool call and returns the result string to be fed back to the AI.
func (r *Registry) Execute(sessionID, toolName string, rawInput json.RawMessage) (string, error) {
	sess, err := r.store.Get(sessionID)
	if err != nil {
		return "", err
	}
	switch toolName {
	case "explain_code":
		return explainCode(sess, rawInput)
	case "debug_code":
		return debugCode(sess, rawInput)
	case "generate_tests":
		return generateTests(sess, rawInput)
	case "propose_execution":
		return r.proposeExecution(sessionID, sess, rawInput)
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}
}

func explainCode(sess *session.AgentSession, raw json.RawMessage) (string, error) {
	var input struct {
		BlockID string `json:"block_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", err
	}
	block, err := findBlock(sess, input.BlockID)
	if err != nil {
		return "", err
	}
	result := fmt.Sprintf("Language: %s\nCode:\n%s", block.Language, block.Code)
	if er, ok := latestExecResult(sess); ok {
		result += fmt.Sprintf("\nLast execution output: %s", er.Result)
		if er.Err != "" {
			result += fmt.Sprintf("\nExecution error: %s", er.Err)
		}
	}
	return result, nil
}

func debugCode(sess *session.AgentSession, raw json.RawMessage) (string, error) {
	var input struct {
		BlockID      string `json:"block_id"`
		ErrorMessage string `json:"error_message"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", err
	}
	block, err := findBlock(sess, input.BlockID)
	if err != nil {
		return "", err
	}
	result := fmt.Sprintf("Language: %s\nCode:\n%s\nError reported: %s", block.Language, block.Code, input.ErrorMessage)
	if er, ok := latestExecResult(sess); ok {
		result += fmt.Sprintf("\nLast execution output: %s\nExecution error: %s", er.Result, er.Err)
	}
	return result, nil
}

func generateTests(sess *session.AgentSession, raw json.RawMessage) (string, error) {
	var input struct {
		BlockID string `json:"block_id"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", err
	}
	block, err := findBlock(sess, input.BlockID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Language: %s\nCode:\n%s", block.Language, block.Code), nil
}

func (r *Registry) proposeExecution(sessionID string, sess *session.AgentSession, raw json.RawMessage) (string, error) {
	var input struct {
		NewCode     string `json:"new_code"`
		Language    string `json:"language"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", err
	}
	normalizedLang, err := coderunner.NormalizeLanguage(input.Language)
	if err != nil {
		return "", err
	}
	_ = sess // already fetched, used for validation context
	p, err := r.store.AddProposal(sessionID, input.NewCode, normalizedLang, input.Description, r.proposalTTL)
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]string{
		"proposal_id": p.ID,
		"description": p.Description,
		"language":    normalizedLang,
	})
	return string(out), nil
}

func findBlock(sess *session.AgentSession, blockID string) (session.CodeBlock, error) {
	sess.Mu.RLock()
	defer sess.Mu.RUnlock()
	for _, b := range sess.ArticleCtx.CodeBlocks {
		if b.BlockID == blockID {
			return b, nil
		}
	}
	return session.CodeBlock{}, fmt.Errorf("block_id %q not found in this article", blockID)
}

func latestExecResult(sess *session.AgentSession) (session.ExecResult, bool) {
	sess.Mu.RLock()
	defer sess.Mu.RUnlock()
	var latest session.ExecResult
	var found bool
	for _, r := range sess.ExecutionResults {
		if !found || r.ReceivedAt.After(latest.ReceivedAt) {
			latest = r
			found = true
		}
	}
	return latest, found
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/tools/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/tools/
git commit -m "feat(agent): add 4 tool implementations with language normalization"
```

---

### Task 9: Agent Service (ReAct Loop)

**Files:**
- Create: `internal/agent/service/agent.go`
- Create: `internal/agent/service/agent_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/service/agent_test.go
package service_test

import (
	"context"
	"testing"
	"time"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/service"
	"codeRunner-siwu/internal/agent/session"
	"codeRunner-siwu/internal/agent/tools"
)

// staticProvider always returns the same sequence of chunks.
type staticProvider struct {
	chunks []ai.ChatChunk
}

func (p *staticProvider) Chat(_ context.Context, _ ai.ChatRequest) (<-chan ai.ChatChunk, error) {
	ch := make(chan ai.ChatChunk, len(p.chunks))
	for _, c := range p.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

// loopProvider returns tool_calls on every call, to test max-steps enforcement.
type loopProvider struct {
	callCount int
}

func (p *loopProvider) Chat(_ context.Context, _ ai.ChatRequest) (<-chan ai.ChatChunk, error) {
	p.callCount++
	ch := make(chan ai.ChatChunk, 2)
	ch <- ai.ChatChunk{ToolCall: &ai.ToolCall{Name: "explain_code", Input: []byte(`{"block_id":"b1"}`)}}
	ch <- ai.ChatChunk{FinishReason: "tool_calls"}
	close(ch)
	return ch, nil
}

func newTestStore(t *testing.T) (*session.Store, string) {
	t.Helper()
	store := session.NewStore(time.Hour)
	sess, err := store.Create(session.ArticleContext{
		ArticleID:      "art-1",
		ArticleContent: "test article",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, sess.ID
}

func TestAgentService_SimpleTextResponse(t *testing.T) {
	store, sessID := newTestStore(t)
	provider := &staticProvider{chunks: []ai.ChatChunk{
		{Content: "Hello "},
		{Content: "World"},
		{FinishReason: "stop"},
	}}
	registry := tools.NewRegistry(store, 10*time.Minute)
	svc := service.NewAgentService(provider, registry, store, 10)

	var received []string
	sseWriter := func(event session.SSEEvent) {
		if event["type"] == "chunk" {
			received = append(received, event["content"].(string))
		}
	}

	if err := svc.Chat(context.Background(), sessID, "explain this", sseWriter); err != nil {
		t.Fatal(err)
	}
	if len(received) == 0 {
		t.Error("expected SSE chunk events")
	}
}

func TestAgentService_MaxStepsHalt(t *testing.T) {
	store, sessID := newTestStore(t)
	provider := &loopProvider{}
	registry := tools.NewRegistry(store, 10*time.Minute)
	svc := service.NewAgentService(provider, registry, store, 3) // maxSteps=3

	svc.Chat(context.Background(), sessID, "loop forever", func(session.SSEEvent) {})
	if provider.callCount > 3 {
		t.Errorf("expected at most 3 LLM calls, got %d", provider.callCount)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/service/... -v
```

- [ ] **Step 3: Implement agent service**

```go
// internal/agent/service/agent.go
package service

import (
	"context"
	"fmt"
	"strings"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/session"
	"codeRunner-siwu/internal/agent/tools"
)

// SSEWriter is called for each SSE event produced during a chat turn.
type SSEWriter func(event session.SSEEvent)

// AgentService is the interface implemented by the agent service.
// It is defined here so handler and router packages can depend on this interface.
type AgentService interface {
	Chat(ctx context.Context, sessionID, message string, write SSEWriter) error
}

const systemPromptTemplate = `You are a code learning assistant helping a user understand code in a blog article.

Article content:
---
%s
---

Code blocks in this article:
%s

Answer concisely. Use tools when they provide better context than reasoning alone.`

type agentService struct {
	provider ai.Provider
	tools    *tools.Registry
	store    *session.Store
	maxSteps int
}

func NewAgentService(provider ai.Provider, registry *tools.Registry, store *session.Store, maxSteps int) AgentService {
	return &agentService{provider: provider, tools: registry, store: store, maxSteps: maxSteps}
}

func (s *agentService) Chat(ctx context.Context, sessionID, userMessage string, write SSEWriter) error {
	sess, err := s.store.Get(sessionID)
	if err != nil {
		return err
	}

	// Inject any pending execution results as context prefix
	pending := s.store.DrainPendingResults(sessionID)
	if len(pending) > 0 {
		var prefixes []string
		for _, r := range pending {
			prefixes = append(prefixes, fmt.Sprintf("[Execution result for proposal %s: output=%q err=%q]",
				r.ProposalID, r.Result, r.Err))
		}
		userMessage = strings.Join(prefixes, "\n") + "\n\n" + userMessage
	}

	if err := s.maybeCompress(ctx, sess); err != nil {
		return err
	}

	sess.Mu.Lock()
	sess.Messages = append(sess.Messages, session.Message{Role: "user", Content: userMessage})
	sess.Mu.Unlock()

	systemPrompt := buildSystemPrompt(sess)

	for step := 0; step < s.maxSteps; step++ {
		sess.Mu.RLock()
		msgs := make([]any, len(sess.Messages))
		for i, m := range sess.Messages {
			msgs[i] = m
		}
		sess.Mu.RUnlock()

		ch, err := s.provider.Chat(ctx, ai.ChatRequest{
			Messages: msgs,
			Tools:    s.tools.Definitions(),
			System:   systemPrompt,
		})
		if err != nil {
			return err
		}

		var textBuf strings.Builder
		var toolCall *ai.ToolCall
		var finishReason string

		for chunk := range ch {
			if chunk.Err != nil {
				write(session.SSEEvent{"type": "error", "message": chunk.Err.Error()})
				return chunk.Err
			}
			if chunk.Content != "" {
				textBuf.WriteString(chunk.Content)
				write(session.SSEEvent{"type": "chunk", "content": chunk.Content})
			}
			if chunk.ToolCall != nil {
				toolCall = chunk.ToolCall
			}
			if chunk.FinishReason != "" {
				finishReason = chunk.FinishReason
			}
		}

		if textBuf.Len() > 0 {
			sess.Mu.Lock()
			sess.Messages = append(sess.Messages, session.Message{Role: "assistant", Content: textBuf.String()})
			sess.Mu.Unlock()
		}

		if finishReason == "stop" || toolCall == nil {
			write(session.SSEEvent{"type": "done"})
			return nil
		}

		// Execute tool and append result
		toolResult, toolErr := s.tools.Execute(sessionID, toolCall.Name, toolCall.Input)
		if toolErr != nil {
			toolResult = fmt.Sprintf("Tool error: %s", toolErr.Error())
		}

		sess.Mu.Lock()
		sess.Messages = append(sess.Messages, session.Message{
			Role:    "tool",
			Content: toolResult,
			Name:    toolCall.Name,
		})
		sess.Mu.Unlock()

		// Notify frontend about pending proposal (without blocking — user must call /confirm)
		if toolCall.Name == "propose_execution" && toolErr == nil {
			write(session.SSEEvent{"type": "proposal_tool_result", "raw": toolResult})
		}
	}

	write(session.SSEEvent{"type": "error", "message": "max steps reached"})
	return fmt.Errorf("max steps reached")
}

func (s *agentService) maybeCompress(ctx context.Context, sess *session.AgentSession) error {
	const threshold = 8000
	const keepRecent = 3

	sess.Mu.RLock()
	tokens := estimateTokens(sess.Messages)
	total := len(sess.Messages)
	sess.Mu.RUnlock()

	if tokens <= threshold || total <= keepRecent {
		return nil
	}

	sess.Mu.RLock()
	toSummarize := append([]session.Message(nil), sess.Messages[:total-keepRecent]...)
	keep := append([]session.Message(nil), sess.Messages[total-keepRecent:]...)
	sess.Mu.RUnlock()

	ch, err := s.provider.Chat(ctx, ai.ChatRequest{
		Messages: []any{session.Message{
			Role:    "user",
			Content: fmt.Sprintf("Summarize in 2-3 sentences, keeping key technical decisions:\n\n%s", formatMessages(toSummarize)),
		}},
	})
	if err != nil {
		return nil // skip compression on error
	}
	var sb strings.Builder
	for chunk := range ch {
		sb.WriteString(chunk.Content)
	}
	if sb.Len() == 0 {
		return nil
	}
	sess.Mu.Lock()
	sess.Messages = append([]session.Message{{Role: "assistant", Content: "[Conversation summary]: " + sb.String()}}, keep...)
	sess.Mu.Unlock()
	return nil
}

func estimateTokens(msgs []session.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content) / 4
	}
	return n
}

func formatMessages(msgs []session.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&sb, "%s: %s\n", m.Role, m.Content)
	}
	return sb.String()
}

func buildSystemPrompt(sess *session.AgentSession) string {
	sess.Mu.RLock()
	defer sess.Mu.RUnlock()
	var blocks strings.Builder
	for _, b := range sess.ArticleCtx.CodeBlocks {
		fmt.Fprintf(&blocks, "- block_id=%q language=%s\n  %s\n", b.BlockID, b.Language, b.Code)
	}
	return fmt.Sprintf(systemPromptTemplate, sess.ArticleCtx.ArticleContent, blocks.String())
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test ./internal/agent/service/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/agent/service/
git commit -m "feat(agent): add agent service with ReAct loop and context compression"
```

---

## Integration Phase (Both Devs Together)

---

### Task 10: /chat SSE Handler + Auth Middleware

**Files:**
- Create: `internal/agent/handler/chat.go`
- Create: `internal/agent/handler/chat_test.go`
- Create: `internal/agent/handler/middleware.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/handler/chat_test.go
package handler_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/agent/service"
	"codeRunner-siwu/internal/agent/session"
)

type mockAgentService struct{}

func (m *mockAgentService) Chat(_ context.Context, _ string, _ string, write service.SSEWriter) error {
	write(session.SSEEvent{"type": "chunk", "content": "hello"})
	write(session.SSEEvent{"type": "done"})
	return nil
}

func TestChat_FirstRequest_CreatesSession(t *testing.T) {
	store := session.NewStore(time.Hour)
	h := handler.NewChatHandler(store, &mockAgentService{})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/chat", h.Handle)

	body, _ := json.Marshal(map[string]any{
		"session_id":   "",
		"user_message": "hello",
		"article_ctx": map[string]any{
			"article_id":      "a1",
			"article_content": "...",
			"code_blocks":     []any{},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/chat", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("want SSE content-type, got %s", w.Header().Get("Content-Type"))
	}

	var events []map[string]any
	scanner := bufio.NewScanner(w.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var ev map[string]any
			json.Unmarshal([]byte(line[6:]), &ev)
			events = append(events, ev)
		}
	}
	if len(events) == 0 {
		t.Fatal("no SSE events received")
	}
	if events[0]["type"] != "session" {
		t.Errorf("first event should be type=session, got %v", events[0]["type"])
	}
	if events[0]["session_id"] == "" || events[0]["session_id"] == nil {
		t.Error("session_id should be set in session event")
	}
}

func TestAuth_MissingAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.APIKeyMiddleware("secret"))
	r.POST("/chat", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodPost, "/chat", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestAuth_ValidAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.APIKeyMiddleware("secret"))
	r.POST("/chat", func(c *gin.Context) { c.Status(200) })

	req := httptest.NewRequest(http.MethodPost, "/chat", nil)
	req.Header.Set("X-Agent-API-Key", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

```bash
go test ./internal/agent/handler/... -run "TestChat|TestAuth" -v
```

- [ ] **Step 3: Implement middleware**

```go
// internal/agent/handler/middleware.go
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func APIKeyMiddleware(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("X-Agent-API-Key") != apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}
```

- [ ] **Step 4: Implement chat handler**

```go
// internal/agent/handler/chat.go
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"codeRunner-siwu/internal/agent/service"
	"codeRunner-siwu/internal/agent/session"
)

type ChatHandler struct {
	store   *session.Store
	service service.AgentService
}

func NewChatHandler(store *session.Store, svc service.AgentService) *ChatHandler {
	return &ChatHandler{store: store, service: svc}
}

type chatRequest struct {
	SessionID   string                  `json:"session_id"`
	UserMessage string                  `json:"user_message" binding:"required"`
	ArticleCtx  *session.ArticleContext `json:"article_ctx"`
}

func (h *ChatHandler) Handle(c *gin.Context) {
	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var sessionID string

	if req.SessionID == "" {
		if req.ArticleCtx == nil || req.ArticleCtx.ArticleID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "article_ctx.article_id required for new session"})
			return
		}
		sess, err := h.store.Create(*req.ArticleCtx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		sessionID = sess.ID
	} else {
		sessionID = req.SessionID
		if _, err := h.store.Get(sessionID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}
		if req.ArticleCtx != nil && req.ArticleCtx.ArticleID != "" {
			h.store.OverrideArticleContext(sessionID, *req.ArticleCtx)
		}
	}

	// Send "interrupted" to the existing SSE connection before replacing it
	sseCh := make(chan session.SSEEvent, 32)
	if oldCh, err := h.store.ReplaceSSEChan(sessionID, sseCh); err == nil && oldCh != nil {
		select {
		case oldCh <- session.SSEEvent{"type": "interrupted"}:
		default:
		}
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	// First event: session_id (for new sessions and as reconnect confirmation)
	if req.SessionID == "" {
		writeSSE(c, session.SSEEvent{"type": "session", "session_id": sessionID})
	}

	ctx := c.Request.Context()
	sseWriter := func(event session.SSEEvent) {
		select {
		case sseCh <- event:
		case <-ctx.Done():
		}
	}

	agentDone := make(chan struct{})
	go func() {
		defer close(agentDone)
		h.service.Chat(ctx, sessionID, req.UserMessage, sseWriter)
	}()

	// Stream SSE events — keep connection alive past "done" for async execution results.
	// Only close on "interrupted", client disconnect, or agent goroutine exit.
	c.Stream(func(w http.ResponseWriter) bool {
		select {
		case event, ok := <-sseCh:
			if !ok {
				return false
			}
			writeSSE(c, event)
			// "interrupted" closes this connection (client should reconnect with new /chat)
			return event["type"] != "interrupted"
		case <-agentDone:
			// Agent finished but keep connection alive for execution_result events
			// Drain any remaining events
			for {
				select {
				case event, ok := <-sseCh:
					if !ok {
						return false
					}
					writeSSE(c, event)
					if event["type"] == "interrupted" {
						return false
					}
				case <-ctx.Done():
					h.store.SetSSEChan(sessionID, nil)
					return false
				}
			}
		case <-ctx.Done():
			h.store.SetSSEChan(sessionID, nil)
			return false
		}
	})
}

func writeSSE(c *gin.Context, event session.SSEEvent) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(c.Writer, "data: %s\n\n", data)
	c.Writer.Flush()
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./internal/agent/handler/... -v -race
```

- [ ] **Step 6: Commit**

```bash
git add internal/agent/handler/chat.go internal/agent/handler/chat_test.go internal/agent/handler/middleware.go
git commit -m "feat(agent): add /chat SSE handler with keep-alive and auth middleware"
```

---

### Task 11: Router

**Files:**
- Create: `internal/agent/router/router.go`

- [ ] **Step 1: Implement**

```go
// internal/agent/router/router.go
package router

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"codeRunner-siwu/internal/agent/config"
	"codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/agent/service"
	"codeRunner-siwu/internal/agent/session"
)

func New(
	cfg *config.Config,
	store *session.Store,
	agentSvc service.AgentService,
	confirmHandler *handler.ConfirmHandler,
	callbackHandler *handler.CallbackHandler,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.POST("/internal/callback", callbackHandler.Handle)

	authed := r.Group("/", handler.APIKeyMiddleware(cfg.Server.APIKey))
	authed.POST("/chat", handler.NewChatHandler(store, agentSvc).Handle)
	authed.POST("/confirm", confirmHandler.Handle)

	return r
}
```

- [ ] **Step 2: Build check**

```bash
go build ./internal/agent/router/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/agent/router/
git commit -m "feat(agent): add Gin router"
```

---

### Task 12: main.go

**Files:**
- Create: `cmd/agent/main.go`

- [ ] **Step 1: Implement**

```go
// cmd/agent/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	agentai "codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/ai/claude"
	"codeRunner-siwu/internal/agent/ai/openai"
	"codeRunner-siwu/internal/agent/config"
	"codeRunner-siwu/internal/agent/coderunner"
	"codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/agent/router"
	"codeRunner-siwu/internal/agent/service"
	"codeRunner-siwu/internal/agent/session"
	"codeRunner-siwu/internal/agent/tools"
)

func main() {
	logger, _ := zap.NewProduction()
	zap.ReplaceGlobals(logger)

	cfg, err := config.Load("configs/agent.yaml")
	if err != nil {
		zap.S().Fatalf("config: %v", err)
	}

	store := session.NewStore(cfg.SessionTTL())

	crClient, err := coderunner.NewClient(
		cfg.CodeRunner.GRPCAddr,
		cfg.CodeRunner.ServiceName,
		cfg.CodeRunner.ServicePassword,
		cfg.TokenRefreshInterval(),
	)
	if err != nil {
		zap.S().Fatalf("coderunner client: %v", err)
	}
	defer crClient.Close()

	var aiProvider agentai.Provider
	switch cfg.Agent.Provider {
	case "openai":
		aiProvider = openai.New(cfg.Agent.OpenAI.APIKey, cfg.Agent.OpenAI.Model)
	default:
		aiProvider = claude.New(cfg.Agent.Claude.APIKey, cfg.Agent.Claude.Model)
	}

	toolRegistry := tools.NewRegistry(store, cfg.ProposalTTL())
	agentSvc := service.NewAgentService(aiProvider, toolRegistry, store, cfg.Agent.MaxSteps)

	confirmHandler := handler.NewConfirmHandler(store, crClient, cfg.Server.InternalBaseURL, cfg.ProposalTTL())
	callbackHandler := handler.NewCallbackHandler(store)

	r := router.New(cfg, store, agentSvc, confirmHandler, callbackHandler)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.Server.Port), Handler: r}

	go func() {
		zap.S().Infof("agent service on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.S().Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	zap.S().Info("stopped")
}
```

- [ ] **Step 2: Build to verify compilation**

```bash
go build ./cmd/agent/...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/agent/main.go
git commit -m "feat(agent): add main entry point with graceful shutdown"
```

---

### Task 13: End-to-End Smoke Test

- [ ] **Step 1: Run all unit tests**

```bash
go test ./internal/agent/... -v -race -count=1
```

Expected: all PASS

- [ ] **Step 2: Start CodeRunner server** (separate terminal)

```bash
export JWT_SECRET="test-secret"
export AUTH_PASSWORD="test-password"
go run cmd/api/main.go
```

- [ ] **Step 3: Start Agent service** (separate terminal)

```bash
export AGENT_API_KEY="test-agent-key"
export CLAUDE_API_KEY="your-key"
export CODERUNNER_SERVICE_PASSWORD="test-password"
go run cmd/agent/main.go
```

- [ ] **Step 4: Test auth rejection**

```bash
curl -s -o /dev/null -w "%{http_code}" \
  -X POST http://localhost:8081/chat \
  -H "Content-Type: application/json" \
  -d '{"user_message":"hi"}'
# Expected: 401
```

- [ ] **Step 5: Test session creation via SSE**

```bash
curl -X POST http://localhost:8081/chat \
  -H "Content-Type: application/json" \
  -H "X-Agent-API-Key: test-agent-key" \
  -N \
  -d '{
    "session_id": "",
    "user_message": "What does this code do?",
    "article_ctx": {
      "article_id": "test-1",
      "article_content": "A Go hello world example.",
      "code_blocks": [{"block_id": "b1", "language": "go", "code": "fmt.Println(\"Hello World\")"}]
    }
  }'
# Expected: SSE stream starting with data: {"type":"session","session_id":"..."}
```

- [ ] **Step 6: Test /confirm 404 for unknown session**

```bash
curl -s -X POST http://localhost:8081/confirm \
  -H "Content-Type: application/json" \
  -H "X-Agent-API-Key: test-agent-key" \
  -d '{"session_id":"fake","proposal_id":"fake"}'
# Expected: {"error":"session not found"}  HTTP 404
```

- [ ] **Step 7: Final commit**

```bash
git add .
git commit -m "feat(agent): complete code learning agent microservice v1"
```
