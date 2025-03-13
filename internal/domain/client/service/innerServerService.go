package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
)

type InnerServerDomain interface {
	Dail(client.TargetServer) error
	Read() (*proto.ExecuteRequest, error)
	Send(*proto.ExecuteResponse) error
	RunCode(*proto.ExecuteRequest) (*proto.ExecuteResponse, error)
}
