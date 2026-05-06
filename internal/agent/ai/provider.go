package ai

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
)

// Config holds the AI provider configuration, decoupled from the agent package.
type Config struct {
	Provider string
	Claude   struct {
		APIKey  string
		Model   string
		BaseURL string
	}
	OpenAI struct {
		APIKey  string
		Model   string
		BaseURL string
	}
	Qwen struct {
		APIKey  string
		Model   string
		BaseURL string
	}
}

type Provider interface {
	ChatModel() model.BaseChatModel
}

func NewProvider(ctx context.Context, cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "claude":
		return NewClaudeProvider(ctx, cfg)
	case "openai":
		return NewOpenAIProvider(ctx, cfg)
	case "qwen":
		return NewQwenProvider(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", cfg.Provider)
	}
}
