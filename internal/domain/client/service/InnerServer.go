package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/infrastructure/websocket/innerServer"
	"context"
	"github.com/gorilla/websocket"
	"log"
)

type InnerServerClient interface {
	Dail(innerServer.TargetServer) error
	Read() (*proto.ExecuteRequest, error)
	SendToServer(*proto.ExecuteResponse) error
	RunCode(*proto.ExecuteRequest) (*proto.ExecuteResponse, error)
}

type InnerServer struct {
	id   string
	conn *websocket.Conn
	entity.DockerContainerDomain
	innerServer.ServerClient
}

func NewInnerServer(ctx context.Context) (*InnerServer, error) {
	// 创建docker对象
	dockerContainer, err := entity.NewDockerClient(ctx)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return &InnerServer{ServerClient: innerServer.NewInnerServerClient(), DockerContainerDomain: dockerContainer}, nil
}

func (i *InnerServer) Dail(targetServer innerServer.TargetServer) error {
	// 初始化websocket连接
	if err := innerServer.NewInnerServerClient().Dail(targetServer); err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func (i *InnerServer) RunCode(request *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	response, err := i.DockerContainerDomain.RunCode(request)
	if err != nil {
		return nil, err
	}
	return &response, err
}

func (i *InnerServer) Read() (*proto.ExecuteRequest, error) {
	msg, err := i.ServerClient.Read()
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (i *InnerServer) SendToServer(response *proto.ExecuteResponse) error {
	if err := i.ServerClient.SendToServer(response); err != nil {
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
