package rpc

import (
	"google.golang.org/grpc"
)

// Register grpc服务注册
func Register() *grpc.Server {
	s := grpc.NewServer()

	//serve := CodeRunner{}
	//proto.RegisterCodeRunnerServer(s, &serve)

	return s
}
