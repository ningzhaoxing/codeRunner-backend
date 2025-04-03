package entity

import (
	"codeRunner-siwu/api/proto"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"log"
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
		log.Println("domain.client.entity.Dail() Dail err=", err)
		return err
	}
	return nil
}

func (i *InnerServerDomainImpl) RunCode(request *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	_, response, err := i.Container.RunCode(request)
	if err != nil {
		log.Println("domain.client.entity.Service() Service err=", err)
		return nil, err
	}
	return &response, err
}

func (i *InnerServerDomainImpl) Read() (*proto.ExecuteRequest, error) {
	msg, err := i.WebsocketClient.Read()
	if err != nil {
		log.Println("domain.client.entity.Read() WebsocketClient.Read err=", err)
		return nil, err
	}
	return msg, nil
}

func (i *InnerServerDomainImpl) Send(response *proto.ExecuteResponse, err error) error {
	if err := i.WebsocketClient.Send(response, err); err != nil {
		log.Println("domain.client.entity.Send() WebsocketClient.Send err=", err)
		return err
	}
	return nil
}
