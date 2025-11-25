package entity

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/events"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	"codeRunner-siwu/internal/infrastructure/websocket/client"

	"github.com/sirupsen/logrus"
)

type InnerServerDomainImpl struct {
	docker.Container
	client.WebsocketClient
}

func NewInnerServerDomainImpl(container docker.Container, websocketClient client.WebsocketClient) *InnerServerDomainImpl {
	return &InnerServerDomainImpl{WebsocketClient: websocketClient, Container: container}
}

func (i *InnerServerDomainImpl) Dail(targetServer client.TargetServer) error {
	if err := i.WebsocketClient.Dail(targetServer); err != nil {
		logrus.Error("domain.client.entity.Dail() Dail err=", err)
		return err
	}
	return nil
}

func (i *InnerServerDomainImpl) RunCode(request *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	// 执行代码
	duration, response, err := i.Container.RunCode(request)
	if err != nil {
		logrus.Error("domain.client.entity.Service() RunCode err=", err)
		return nil, err
	}

	// 将响应时间发送给服务端
	if err := i.sendResponseDuration(float64(duration)); err != nil {
		logrus.Error("domain.client.entity.Service() sendResponseDuration err=", err)
		return nil, err
	}
	return &response, err
}

// 将服务响应时间通过websocket发送给服务端
func (i *InnerServerDomainImpl) sendResponseDuration(duration float64) error {
	data := events.NewMsgOfResponseDuration(duration)
	return i.WebsocketSend(data)
}

func (i *InnerServerDomainImpl) Read() (*proto.ExecuteRequest, error) {
	msg, err := i.WebsocketClient.Read()
	if err != nil {
		logrus.Error("domain.client.entity.Read() WebsocketClient.Read err=", err)
		return nil, err
	}
	return msg, nil
}

func (i *InnerServerDomainImpl) Send(response *proto.ExecuteResponse, err error) error {
	if err := i.WebsocketClient.CallBackSend(response, err); err != nil {
		logrus.Error("domain.client.entity.CallBackSend() WebsocketClient.CallBackSend err=", err)
		return err
	}
	return nil
}
