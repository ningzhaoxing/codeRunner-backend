package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"fmt"
	"log"
)

type Service interface {
	Run(config.Config) error
}

type ServiceImpl struct {
	service.InnerServerDomain
}

func NewServiceImpl(innerServerDomainTmpl service.InnerServerDomain) *ServiceImpl {
	return &ServiceImpl{InnerServerDomain: innerServerDomainTmpl}
}

func (w *ServiceImpl) Run(c config.Config) error {
	if err := w.dail(c); err != nil {
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
		fmt.Println("读取到消息:", msg)
		// 执行代码
		res, err := w.RunCode(msg)
		if err != nil {
			log.Println("application.service.Run() Service err=", err)
			continue
		}
		fmt.Println("处理结果为:", res)
		// 发送结果
		if err = w.send(res); err != nil {
			log.Println("application.service.Run() send err=", err)
			continue
		}
	}
}

// 向服务端建立连接
func (w *ServiceImpl) dail(c config.Config) error {
	targetServer := client.NewTargetServer(c.Client.Server.Host, c.Client.Server.Port, c.Client.Server.Path, fmt.Sprintf("weight=%d", c.Client.App.Weight))

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
