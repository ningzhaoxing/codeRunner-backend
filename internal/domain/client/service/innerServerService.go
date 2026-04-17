package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
)

type InnerServerDomain interface {
	Dail(client.TargetServer) error
	Read() (*client.ReadResult, error)
	Send(*proto.ExecuteResponse, error) error
	SendResult(*proto.ExecuteResponse, error) error
	RunCode(*proto.ExecuteRequest) (*proto.ExecuteResponse, error)
}
