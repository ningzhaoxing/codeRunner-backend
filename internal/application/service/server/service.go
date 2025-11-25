package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"fmt"

	"github.com/sirupsen/logrus"
)

type ServerService interface {
	Execute(in *proto.ExecuteRequest) error
	Run(cli WebsocketClient, weight int64) error
}

type ServiceImpl struct {
	ClientManagerDomain
}

func NewServiceImpl(clientManagerDomain ClientManagerDomain) *ServiceImpl {
	return &ServiceImpl{
		ClientManagerDomain: clientManagerDomain,
	}
}

func (w *ServiceImpl) Execute(in *proto.ExecuteRequest) error {
	// 通过负载均衡获取客户端
	client, err := w.ClientManagerDomain.GetClientByBalance()
	if err != nil {
		logrus.Error(fmt.Sprintln("application.server.CallBackSend() Execute err=\n", err))
		return err
	}

	// 将请求数据发送给内网服务器
	err = client.Send(in)
	if err != nil {
		logrus.Error(fmt.Sprintln("application.server.CallBackSend() CallBackSend err=\n", err))
		return err
	}
	return nil
}

func (w *ServiceImpl) Run(cli WebsocketClient, weight int64) error {
	// 将http请求的内网服务器客户端加入到服务端的 clientManager
	client := entity.NewClient(cli)
	w.ClientManagerDomain.AddClient(client, weight)

	// 启动心跳检测
	if err := client.HeartBeat(); err != nil {
		logrus.Error(fmt.Sprintln("application.server.Run() HeartBeat err=\n", err))
		return err
	}

	// 读取客户端消息
	for {
		if _, err := client.Read(); err != nil {
			logrus.Error(fmt.Sprintln("application.server.server.Run() Read() err=\n", err))
			return err
		}
	}
}

type ClientManagerDomain interface {
	AddClient(*entity.Client, int64)
	RemoveClient(string) error
	GetClientByBalance() (*entity.Client, error)
	GetClientById(id string) (*entity.Client, error)
}

type WebsocketClient interface {
	Send([]byte) error
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}
