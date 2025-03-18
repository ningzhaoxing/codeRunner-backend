package ws

import (
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/infrastructure/config"
	"context"
	"log"
)

type InnerServerClient struct {
	weight int64 // 服务器权重
	client.Service
}

func NewInnerServerClient(c *config.Config, ctx context.Context, weight int64) (*InnerServerClient, error) {
	client, err := client.NewServiceImpl(c, ctx)
	if err != nil {
		log.Println("interfaces-controller-ws NewInnerServerClient的service.NewServiceImpl err=", err)
		return nil, err
	}
	return &InnerServerClient{Service: client, weight: weight}, nil
}

func (i *InnerServerClient) Run() error {
	if err := i.Service.Run(i.weight); err != nil {
		log.Println("interfaces-controller-ws NewInnerServerClient的i.Service.Run err=", err)
		return err
	}
	return nil
}
