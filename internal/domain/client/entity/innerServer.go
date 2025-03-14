package entity

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"context"
	"github.com/gorilla/websocket"
	"log"
)

type InnerServer struct {
	id   string
	conn *websocket.Conn
	DockerContainer
	client.WebsocketClient
}

func NewInnerServer(ctx context.Context) (*InnerServer, error) {
	// 创建docker对象
	dockerContainer, err := NewDockerClient(ctx)
	if err != nil {
		log.Println("domain.client.entity.NewInnerServer() NewDockerClient err=", err)
		return nil, err
	}
	return &InnerServer{WebsocketClient: client.NewInnerServerClient(), DockerContainer: dockerContainer}, nil
}

func (i *InnerServer) Dail(targetServer client.TargetServer) error {
	// 初始化websocket连接
	if err := i.WebsocketClient.Dail(targetServer); err != nil {
		log.Println("domain.client.entity.Dail() Dail err=", err)
		return err
	}
	return nil
}

func (i *InnerServer) RunCode(request *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	response, err := i.DockerContainer.RunCode(request)
	if err != nil {
		log.Println("domain.client.entity.RunCode() RunCode err=", err)
		return nil, err
	}
	return &response, err
}

func (i *InnerServer) Read() (*proto.ExecuteRequest, error) {
	msg, err := i.WebsocketClient.Read()
	if err != nil {
		log.Println("domain.client.entity.Read() WebsocketClient.Read err=", err)
		return nil, err
	}
	return msg, nil
}

func (i *InnerServer) Send(response *proto.ExecuteResponse) error {
	if err := i.WebsocketClient.Send(response); err != nil {
		log.Println("domain.client.entity.Send() WebsocketClient.Send err=", err)
		return err
	}
	return nil
}

func (i *InnerServer) GetId() string {
	return i.id
}

func (i *InnerServer) GetConn() *websocket.Conn {
	return i.conn
}
