package ai

import (
	"context"
	"fmt"

	oai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type qwenProvider struct {
	chatModel model.BaseChatModel
}

func NewQwenProvider(ctx context.Context, cfg Config) (*qwenProvider, error) {
	if cfg.Qwen.APIKey == "" {
		return nil, fmt.Errorf("qwen API key is required")
	}
	modelName := cfg.Qwen.Model
	if modelName == "" {
		modelName = "qwen-plus"
	}

	cm, err := oai.NewChatModel(ctx, &oai.ChatModelConfig{
		APIKey:  cfg.Qwen.APIKey,
		Model:   modelName,
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
	})
	if err != nil {
		return nil, fmt.Errorf("create qwen model: %w", err)
	}
	return &qwenProvider{chatModel: cm}, nil
}

func (p *qwenProvider) ChatModel() model.BaseChatModel {
	return p.chatModel
}
