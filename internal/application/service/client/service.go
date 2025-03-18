package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"context"
	"fmt"
	"log"
)

// Service 主要任务是执行代码，并将结果post到调用者
type Service interface {
	Run(int64) error
}

// ServiceImpl websocket 客户端 -- 内网服务器
type ServiceImpl struct {
	config *config.Config
	service.InnerServerDomain
}

func NewServiceImpl(config *config.Config, ctx context.Context) (*ServiceImpl, error) {
	client, err := entity.NewInnerServer(ctx)
	if err != nil {
		log.Println("application.service.NewServiceImpl() NewInnerServer err=", err)
		return nil, err
	}
	return &ServiceImpl{config: config, InnerServerDomain: client}, nil
}

func (w *ServiceImpl) Run(weight int64) error {
	if err := w.dail(weight); err != nil {
		log.Println("application.service.Run() dail err=", err)
		return err
	}

	for {
		// 读取消息
		msg, err := w.InnerServerDomain.Read()
		if err != nil {
			log.Println("application.service.Run() Read err=", err)
			continue
		}
		fmt.Println(msg)

		// 执行代码
		res, err := w.RunCode(msg)
		if err != nil {
			log.Println("application.service.Run() Service err=", err)
			continue
		}

		// 发送结果
		if err = w.send(res); err != nil {
			log.Println("application.service.Run() send err=", err)
			continue
		}
	}
}

// 向服务端建立连接
func (w *ServiceImpl) dail(weight int64) error {
	targetServer := client.NewTargetServer("8.154.36.180", "7979", "ws", fmt.Sprintf("weight=%d", weight))

	err := w.InnerServerDomain.Dail(*targetServer)
	if err != nil {
		log.Println("application.service.dail() Dail err=", err)
		return err
	}
	return nil
}

func (w *ServiceImpl) send(res *proto.ExecuteResponse) error {
	if err := w.InnerServerDomain.Send(res); err != nil {
		log.Println("application.service.send() Send err=", err)
		return err
	}
	return nil
}
