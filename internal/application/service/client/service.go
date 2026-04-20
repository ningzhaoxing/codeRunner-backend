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
	zap.S().Infof("Worker starting, target server: %s:%s/%s (weight=%d)",
		c.Client.Server.Host, c.Client.Server.Port, c.Client.Server.Path, c.Client.App.Weight)

	if err := w.dail(c); err != nil {
		zap.S().Errorf("Worker failed to connect: %v", err)
		return err
	}

	zap.S().Info("Worker ready, waiting for tasks...")

	for {
		readResult, err := w.InnerServerDomain.Read()
		if err != nil {
			zap.S().Errorf("Worker websocket closed, service stopped: %v", err)
			return err
		}
		zap.S().Infof("Worker received task: requestId=%s language=%s", readResult.Request.Id, readResult.Request.Language)

		res, execErr := w.RunCode(readResult.Request)
		if execErr != nil {
			zap.S().Errorf("Worker code execution failed: requestId=%s err=%v", readResult.Request.Id, execErr)
		} else {
			zap.S().Infof("Worker code execution succeeded: requestId=%s", readResult.Request.Id)
		}

		switch readResult.MsgType {
		case protocol.MsgTypeExecuteSync:
			if err = w.sendResult(res, execErr); err != nil {
				zap.S().Errorf("Worker sendResult failed: requestId=%s err=%v", res.Id, err)
			} else {
				zap.S().Infof("Worker sendResult succeeded: requestId=%s", res.Id)
			}
		default:
			if err = w.send(res, execErr); err != nil {
				zap.S().Errorf("Worker callback failed: requestId=%s err=%v", res.Id, err)
			} else {
				zap.S().Infof("Worker callback succeeded: requestId=%s", res.Id)
			}
		}
	}
}

// 向服务端建立连接
func (w *ServiceImpl) dail(c config.Config) error {
	targetServer := client.NewTargetServer(c.Client.Server.Host, c.Client.Server.Port, c.Client.Server.Path, fmt.Sprintf("weight=%d", c.Client.App.Weight))
	// 发起websocket连接
	err := w.InnerServerDomain.Dail(*targetServer)
	if err != nil {
		zap.S().Errorf("Worker dial failed: %v", err)
		return err
	}
	return nil
}

func (w *ServiceImpl) send(res *proto.ExecuteResponse, err error) error {
	// 发送消息
	if err := w.InnerServerDomain.Send(res, err); err != nil {
		zap.S().Errorf("Worker callback send failed: %v", err)
		return err
	}
	return nil
}

func (w *ServiceImpl) sendResult(res *proto.ExecuteResponse, err error) error {
	if err := w.InnerServerDomain.SendResult(res, err); err != nil {
		zap.S().Errorf("Worker sendResult failed: %v", err)
		return err
	}
	return nil
}
