package rpc

import (
	"codeRunner-siwu/api/proto"
	"google.golang.org/grpc"
)

// Register grpc服务注册
func Register() *grpc.Server {
	// token中间件注册
	u := grpc.UnaryInterceptor(UnaryInterceptor())
	s := grpc.NewServer(u)

	// token签发服务注册
	token := TokenServer{}
	proto.RegisterTokenIssuerServer(s, &token)

	// 代码运行服务注册
	serve := Server{}
	proto.RegisterCodeRunnerServer(s, &serve)

	return s
}
