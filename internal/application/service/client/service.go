package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"fmt"
)

type Service interface {
	Run(config.Config) error
}

type ServiceImpl struct {
	service.InnerServerDomain
	logger.Logger
}

func NewServiceImpl(innerServerDomainTmpl service.InnerServerDomain, logger logger.Logger) *ServiceImpl {
	return &ServiceImpl{InnerServerDomain: innerServerDomainTmpl,
		Logger: logger}
}

func (w *ServiceImpl) Run(c config.Config) error {
	if err := w.dail(c); err != nil {
		w.Logger.Error(fmt.Sprintln("application.client.Run() dail err=", err))
		return err
	}

	for {
		// 读取消息
		msg, err := w.InnerServerDomain.Read()
		if err != nil {
			w.Logger.Error(fmt.Sprintln("websocket客户端已被关闭,请重启服务。application.client.Run() Read err=", err))
			return err
		}
		fmt.Println("读取到消息:", msg)

		// 执行代码
		res, err := w.RunCode(msg)
		if err != nil {
			w.Logger.Error(fmt.Sprintln("application.client.Run() Service err=", err))
			continue
		}
		fmt.Println("处理结果为:", res)

		// 发送结果
		if err = w.send(res, err); err != nil {
			w.Logger.Error(fmt.Sprintln("application.client.Run() send err=", err))
			continue
		}
	}
}

// 向服务端建立连接
func (w *ServiceImpl) dail(c config.Config) error {
	targetServer := client.NewTargetServer(c.Client.Server.Host, c.Client.Server.Port, c.Client.Server.Path, fmt.Sprintf("weight=%d", c.Client.App.Weight))
	// 发起websocket连接
	err := w.InnerServerDomain.Dail(*targetServer)
	if err != nil {
		w.Logger.Error(fmt.Sprintln("application.client.dail() Dail err=\n", err))
		return err
	}
	return nil
}

func (w *ServiceImpl) send(res *proto.ExecuteResponse, err error) error {
	// 发送消息
	if err := w.InnerServerDomain.Send(res, err); err != nil {
		w.Logger.Error(fmt.Sprintln("application.client.send() Send err=\n", err))
		return err
	}
	return nil
}
