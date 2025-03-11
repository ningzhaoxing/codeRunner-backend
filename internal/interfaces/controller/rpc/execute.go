package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"context"
)

type Server struct {
}

func (s *Server) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	server := service.NewWebsocketServer()
	err := server.Execute(in)
	if err != nil {
		return nil, err
	}
	return nil, err
}
