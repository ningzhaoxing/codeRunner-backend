package ai

import (
	"context"
	"fmt"

	oai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type openaiProvider struct {
	chatModel model.BaseChatModel
}

func NewOpenAIProvider(ctx context.Context, cfg Config) (*openaiProvider, error) {
	if cfg.OpenAI.APIKey == "" {
		return nil, fmt.Errorf("openai API key is required")
	}
	modelName := cfg.OpenAI.Model
	if modelName == "" {
		modelName = "gpt-4o"
	}

	cm, err := oai.NewChatModel(ctx, &oai.ChatModelConfig{
		APIKey: cfg.OpenAI.APIKey,
		Model:  modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model: %w", err)
	}
	return &openaiProvider{chatModel: cm}, nil
}

func (p *openaiProvider) ChatModel() model.BaseChatModel {
	return p.chatModel
}
