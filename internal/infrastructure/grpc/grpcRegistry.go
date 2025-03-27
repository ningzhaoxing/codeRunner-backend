package grpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/interfaces/adapter/middleware"
	"codeRunner-siwu/internal/interfaces/controller"
	"google.golang.org/grpc"
)

// Register grpc服务注册
func Register() *grpc.Server {
	// token中间件注册
	u := grpc.UnaryInterceptor(middleware.UnaryInterceptor())
	s := grpc.NewServer(u)

	// token签发服务注册
	token := controller.APIs.Auth
	proto.RegisterTokenIssuerServer(s, &token)

	// 代码运行服务注册
	serve := controller.APIs.CodeRunnerSrv
	proto.RegisterCodeRunnerServer(s, &serve)

	return s
}
