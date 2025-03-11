package ws

import (
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"context"
)

type InnerServerClient struct {
	weight int64 // 服务器权重
	service.RunCode
}

func NewInnerServerClient(c *config.Config, ctx context.Context, weight int64) (*InnerServerClient, error) {
	client, err := service.NewWebsocketClient(c, ctx)
	if err != nil {
		return nil, err
	}
	return &InnerServerClient{RunCode: client, weight: weight}, nil
}

func (i *InnerServerClient) Run() error {
	if err := i.RunCode.Run(i.weight); err != nil {
		return err
	}
	return nil
}
