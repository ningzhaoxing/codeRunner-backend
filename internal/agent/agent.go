package agent

import (
	"context"
	"fmt"
	"sync"

	"codeRunner-siwu/internal/agent/ai"
	"codeRunner-siwu/internal/agent/checkpoint"
	"codeRunner-siwu/internal/agent/session"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"go.uber.org/zap"
)

type AgentService struct {
	Cfg             AgentConfig
	Provider        ai.Provider
	SessionStore    session.Store
	CheckpointStore *checkpoint.MemoryCheckPointStore
	Runner          *adk.Runner
	Executor        CodeExecutor

	// InterruptIDs maps sessionID → interrupt context ID (from InterruptCtx.ID).
	// Populated by the chat handler when an interrupt event is observed.
	InterruptIDs sync.Map // sessionID → string

	// Proposals maps sessionID → *tools.ProposalInfo.
	// Populated by the chat handler when a propose_execution interrupt is observed.
	Proposals sync.Map // sessionID → *tools.ProposalInfo
}

func NewAgentService(ctx context.Context, cfg AgentConfig, dataDir string) (*AgentService, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("agent is not enabled")
	}

	// AI Provider
	aiCfg := ai.Config{Provider: cfg.Provider}
	aiCfg.Claude.APIKey = cfg.Claude.APIKey
	aiCfg.Claude.Model = cfg.Claude.Model
	aiCfg.OpenAI.APIKey = cfg.OpenAI.APIKey
	aiCfg.OpenAI.Model = cfg.OpenAI.Model
	aiCfg.Qwen.APIKey = cfg.Qwen.APIKey
	aiCfg.Qwen.Model = cfg.Qwen.Model
	provider, err := ai.NewProvider(ctx, aiCfg)
	if err != nil {
		return nil, fmt.Errorf("create AI provider: %w", err)
	}

	// Session Store (JSONL file implementation)
	sessionStore, err := session.NewFileStore(dataDir+"/agent/sessions", cfg.GetSessionTTL())
	if err != nil {
		return nil, fmt.Errorf("create session store: %w", err)
	}
	sessionStore.StartCleanup(cfg.GetSessionTTL() / 6)

	// Checkpoint Store (memory, for HITL only)
	checkpointStore := checkpoint.NewMemoryCheckPointStore(cfg.GetSessionTTL())
	checkpointStore.StartCleanup(cfg.GetSessionTTL() / 6)

	// Context compression middleware
	summMw, err := summarization.New(ctx, &summarization.Config{
		Model: provider.ChatModel(),
		Trigger: &summarization.TriggerCondition{
			ContextTokens: 100000,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create summarization middleware: %w", err)
	}

	// ChatModelAgent with summarization middleware
	chatAgent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:          "code-learning-agent",
		Model:         provider.ChatModel(),
		MaxIterations: cfg.MaxSteps,
		Handlers:      []adk.ChatModelAgentMiddleware{summMw},
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model agent: %w", err)
	}

	// Runner wraps ChatModelAgent + CheckPointStore
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           chatAgent,
		EnableStreaming:  true,
		CheckPointStore: checkpointStore,
	})

	zap.S().Info("Agent service initialized", "provider", cfg.Provider)

	return &AgentService{
		Cfg:             cfg,
		Provider:        provider,
		SessionStore:    sessionStore,
		CheckpointStore: checkpointStore,
		Runner:          runner,
		// Executor is set externally after construction (requires ServerService).
	}, nil
}
