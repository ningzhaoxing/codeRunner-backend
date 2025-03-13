package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"context"
)

type Server struct {
	service.RunServer
}

func NewServer(websocketServer *service.WebsocketServer) *Server {
	return &Server{RunServer: websocketServer}
}

func (s *Server) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	err := s.RunServer.Execute(in)
	if err != nil {
		return nil, err
	}
	return nil, err
}
