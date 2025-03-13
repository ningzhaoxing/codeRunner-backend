package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"context"
)

type Server struct {
}

func (s *Server) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	err := ws.WebsocketServer.Execute(in)
	if err != nil {
		return nil, err
	}
	return nil, err
}
