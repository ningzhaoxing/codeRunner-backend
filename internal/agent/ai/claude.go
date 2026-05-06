package ai

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type claudeProvider struct {
	chatModel model.BaseChatModel
}

func NewClaudeProvider(ctx context.Context, cfg Config) (*claudeProvider, error) {
	if cfg.Claude.APIKey == "" {
		return nil, fmt.Errorf("claude API key is required")
	}
	modelName := cfg.Claude.Model
	if modelName == "" {
		modelName = "claude-opus-4-6"
	}
	baseURL := cfg.Claude.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Claude.APIKey,
		Model:   modelName,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create claude model: %w", err)
	}
	return &claudeProvider{chatModel: cm}, nil
}

func (p *claudeProvider) ChatModel() model.BaseChatModel {
	return p.chatModel
}
