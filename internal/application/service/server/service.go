package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/websocket/server"
	"log"
)

type Service interface {
	Execute(in *proto.ExecuteRequest) error
	Run(cli server.WebsocketClient, weight int64) error
}

type ServiceImpl struct {
	service.ClientManagerDomain
}

func NewServiceImpl(clientManagerDomain service.ClientManagerDomain) *ServiceImpl {
	return &ServiceImpl{
		ClientManagerDomain: clientManagerDomain,
	}
}

func (w *ServiceImpl) Execute(in *proto.ExecuteRequest) error {
	// 通过负载均衡获取客户端
	client, err := w.ClientManagerDomain.GetClientByBalance()
	if err != nil {
		log.Println("application.service.Send() Execute err=", err)
		return err
	}

	// 将请求数据发送给内网服务器
	err = client.Send(in)
	if err != nil {
		log.Println("application.service.Send() Send err=", err)
		return err
	}
	return nil
}

func (w *ServiceImpl) Run(cli server.WebsocketClient, weight int64) error {
	// 将http请求的内网服务器客户端加入到服务端的 clientManager
	client := entity.NewClient(cli)
	w.ClientManagerDomain.AddClient(client, weight)

	// 启动心跳检测
	if err := client.HeartBeat(); err != nil {
		return err
	}

	// 维持连接
	for {
		if _, err := client.Read(); err != nil {
			log.Println("application.service.server.Run() Read() err=", err)
			continue
		}
	}
}
