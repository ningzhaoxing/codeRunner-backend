package ws

import (
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"context"
	"log"
)

type InnerServerClient struct {
	weight int64 // 服务器权重
	service.RunCode
}

func NewInnerServerClient(c *config.Config, ctx context.Context, weight int64) (*InnerServerClient, error) {
	client, err := service.NewWebsocketClient(c, ctx)
	if err != nil {
		log.Println("interfaces-controller-ws NewInnerServerClient的service.NewWebsocketClient err=", err)
		return nil, err
	}
	return &InnerServerClient{RunCode: client, weight: weight}, nil
}

func (i *InnerServerClient) Run() error {
	if err := i.RunCode.Run(i.weight); err != nil {
		log.Println("interfaces-controller-ws NewInnerServerClient的i.RunCode.Run err=", err)
		return err
	}
	return nil
}
