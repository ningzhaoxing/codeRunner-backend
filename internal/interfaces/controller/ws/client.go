package ws

import (
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"context"
)

type InnerServerClient struct {
	service.RunCode
}

func NewInnerServerClient(c *config.Config, ctx context.Context) (*InnerServerClient, error) {
	client, err := service.NewWebsocketClient(c, ctx)
	if err != nil {
		return nil, err
	}
	return &InnerServerClient{RunCode: client}, nil
}

func (i *InnerServerClient) Run() error {
	if err := i.RunCode.Run(); err != nil {
		return err
	}
	return nil
}
