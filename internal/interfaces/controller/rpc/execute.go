package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"context"
	"log"
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
		log.Println("interfaces-controller-rpc-execute Execute的 s.RunServer.Execute err=", err)
		return nil, err
	}
	return nil, err
}
