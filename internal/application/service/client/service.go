package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"fmt"
	"go.uber.org/zap"
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
		zap.S().Error(fmt.Sprintln("application.client.Run() dail err=", err))
		return err
	}

	for {
		readResult, err := w.InnerServerDomain.Read()
		if err != nil {
			zap.S().Error(fmt.Sprintln("websocket客户端已被关闭,请重启服务。application.client.Run() Read err=", err))
			return err
		}
		fmt.Println("读取到消息:", readResult.Request)

		res, execErr := w.RunCode(readResult.Request)
		if execErr != nil {
			zap.S().Error(fmt.Sprintln("application.client.Run() Service err=", execErr))
		}
		fmt.Println("处理结果为:", res)

		switch readResult.MsgType {
		case protocol.MsgTypeExecuteSync:
			if err = w.sendResult(res, execErr); err != nil {
				zap.S().Error(fmt.Sprintln("application.client.Run() sendResult err=", err))
			}
		default:
			if err = w.send(res, execErr); err != nil {
				zap.S().Error(fmt.Sprintln("application.client.Run() send err=", err))
			}
		}
		fmt.Println("结果发送成功")
	}
}

// 向服务端建立连接
func (w *ServiceImpl) dail(c config.Config) error {
	targetServer := client.NewTargetServer(c.Client.Server.Host, c.Client.Server.Port, c.Client.Server.Path, fmt.Sprintf("weight=%d", c.Client.App.Weight))
	// 发起websocket连接
	err := w.InnerServerDomain.Dail(*targetServer)
	if err != nil {
		zap.S().Error(fmt.Sprintln("application.client.dail() Dail err=\n", err))
		return err
	}
	return nil
}

func (w *ServiceImpl) send(res *proto.ExecuteResponse, err error) error {
	// 发送消息
	if err := w.InnerServerDomain.Send(res, err); err != nil {
		zap.S().Error(fmt.Sprintln("application.client.send() CallBackSend err=\n", err))
		return err
	}
	return nil
}

func (w *ServiceImpl) sendResult(res *proto.ExecuteResponse, err error) error {
	if err := w.InnerServerDomain.SendResult(res, err); err != nil {
		zap.S().Error(fmt.Sprintln("application.client.sendResult() err=", err))
		return err
	}
	return nil
}
