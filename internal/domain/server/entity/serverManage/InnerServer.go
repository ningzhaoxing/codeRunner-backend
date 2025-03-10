package serverManage

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity/containerManage"
	"codeRunner-siwu/internal/infrastructure/websocket/innerServer"
	"context"
	"github.com/google/uuid"
	"log"
)

type InnerServerClient interface {
	Dail(innerServer.TargetServer) error
	Read() (*proto.ExecuteRequest, error)
	Send(*proto.ExecuteResponse) error
	RunCode(*proto.ExecuteRequest) (*proto.ExecuteResponse, error)
}

type InnerServer struct {
	id string
	containerManage.DockerContainerDomain
	innerServer.ServerClient
}

// New 内网服务器使用的对象
func New(ctx context.Context) (*InnerServer, error) {
	// 创建docker对象
	dockerContainer, err := containerManage.NewDockerClient(ctx)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return &InnerServer{ServerClient: innerServer.NewInnerServerClient(), DockerContainerDomain: dockerContainer}, nil
}

// NewInnerServer 服务端使用的对象
func NewInnerServer() *InnerServer {
	// 签发uuid
	id := uuid.NewString()
	return &InnerServer{
		id: id,
	}
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

func (i *InnerServer) Send(response *proto.ExecuteResponse) error {
	if err := i.ServerClient.Send(response); err != nil {
		return err
	}
	return nil
}

func (i *InnerServer) GetId() string {
	return i.id
}
