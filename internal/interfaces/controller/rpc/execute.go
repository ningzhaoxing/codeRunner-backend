package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service/server"
	"context"
	"fmt"
	"log"
)

type Server struct {
	server.Service
}

func NewServer(websocketServer *server.ServiceTmpl) *Server {
	return &Server{Service: websocketServer}
}

func (s *Server) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	fmt.Println("执行execute")
	err := s.Service.Execute(in)
	if err != nil {
		log.Println("interfaces-controller-rpc-execute Execute的 s.Service.Execute err=", err)
		return nil, err
	}
	return nil, err
}
