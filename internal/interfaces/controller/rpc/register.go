package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"google.golang.org/grpc"
)

// Register grpc服务注册
func Register() *grpc.Server {
	s := grpc.NewServer()

	serve := service.CodeRunner{}
	proto.RegisterCodeRunnerServer(s, &serve)

	return s
}
