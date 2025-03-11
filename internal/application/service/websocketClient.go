package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/innerServer"
	"context"
	"fmt"
)

type RunCode interface {
	Run(int64) error
}

type WebsocketClient struct {
	config *config.Config
	service.InnerServerClient
}

func NewWebsocketClient(config *config.Config, ctx context.Context) (*WebsocketClient, error) {
	client, err := service.NewInnerServer(ctx)
	if err != nil {
		return nil, err
	}
	return &WebsocketClient{config: config, InnerServerClient: client}, nil
}

func (w *WebsocketClient) dail(weight int64) error {
	targetServer := innerServer.NewTargetServer(w.config.Grpc.Host, w.config.Grpc.Port, fmt.Sprintf("weight=%d", weight), "/ws")

	err := w.InnerServerClient.Dail(*targetServer)
	if err != nil {
		return err
	}
	return nil
}

func (w *WebsocketClient) send(res *proto.ExecuteResponse) error {
	if err := w.InnerServerClient.SendToServer(res); err != nil {
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
		msg, err := w.InnerServerClient.Read()
		if err != nil {
			return err
		}
		// 执行代码
		res, err := w.RunCode(msg)

		// 发送结果
		if err = w.send(res); err != nil {
			return err
		}
	}
}
