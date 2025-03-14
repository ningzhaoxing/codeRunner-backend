package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"context"
	"fmt"
)

// RunCode 主要任务是执行代码，并将结果post到调用者
type RunCode interface {
	Run(int64) error
}

// WebsocketClient websocket 客户端 -- 内网服务器
type WebsocketClient struct {
	config *config.Config
	service.InnerServerDomain
}

func NewWebsocketClient(config *config.Config, ctx context.Context) (*WebsocketClient, error) {
	client, err := entity.NewInnerServer(ctx)
	if err != nil {
		return nil, err
	}
	return &WebsocketClient{config: config, InnerServerDomain: client}, nil
}

// 向服务端建立连接
func (w *WebsocketClient) dail(weight int64) error {
	targetServer := client.NewTargetServer("8.154.36.180", "7979", fmt.Sprintf("weight=%d", weight), "/ws")

	err := w.InnerServerDomain.Dail(*targetServer)
	if err != nil {
		return err
	}
	return nil
}

// 向
func (w *WebsocketClient) send(res *proto.ExecuteResponse) error {
	if err := w.InnerServerDomain.Send(res); err != nil {
		return err
	}
	return nil
}

func (w *WebsocketClient) Run(weight int64) error {
	if err := w.dail(weight); err != nil {
		return err
	}

	for {
		// 读取消息
		msg, err := w.InnerServerDomain.Read()
		if err != nil {
			return err
		}
		// 执行代码
		res, err := w.RunCode(msg)
		if err != nil {
			return err
		}

		// 发送结果
		if err = w.send(res); err != nil {
			return err
		}
	}
}
