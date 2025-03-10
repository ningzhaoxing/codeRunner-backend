package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity/serverManage"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/innerServer"
	"context"
)

type RunCode interface {
	Run() error
}

type WebsocketClient struct {
	config *config.Config
	serverManage.InnerServerClient
}

func NewWebsocketClient(config *config.Config, ctx context.Context) (*WebsocketClient, error) {
	client, err := serverManage.New(ctx)
	if err != nil {
		return nil, err
	}
	return &WebsocketClient{config: config, InnerServerClient: client}, nil
}

func (w *WebsocketClient) dail() error {
	targetServer := innerServer.NewTargetServer(w.config.Grpc.Host, w.config.Grpc.Port, "/ws")

	err := w.InnerServerClient.Dail(*targetServer)
	if err != nil {
		return err
	}
	return nil
}

func (w *WebsocketClient) send(res *proto.ExecuteResponse) error {
	if err := w.InnerServerClient.Send(res); err != nil {
		return err
	}
	return nil
}

func (w *WebsocketClient) Run() error {
	if err := w.dail(); err != nil {
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
