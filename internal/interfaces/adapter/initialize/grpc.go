package initialize

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/interfaces/adapter/middleware"
	"codeRunner-siwu/internal/interfaces/controller"
	"codeRunner-siwu/internal/interfaces/controller/auth"
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"net"
)

func InitGrpc(c *config.Config, ctx context.Context) (net.Listener, *grpc.Server, *clientv3.Client) {
	// 将grpc服务注册到etcd
	etcdClient, err := EtcdRegister(ctx, c)
	if err != nil {
		panic("服务注册失败" + err.Error())
	}

	// 启动grpc服务
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", "0.0.0.0", c.Server.Grpc.Port))
	if err != nil {
		panic("grpc服务启动失败" + err.Error())
	}

	// 注册grpc服务
	s := register()
	return lis, s, etcdClient.Client
}

// register grpc服务注册
func register() *grpc.Server {
	// token中间件注册
	u := grpc.UnaryInterceptor(middleware.UnaryInterceptor())
	s := grpc.NewServer(u)

	// token签发服务注册
	token := auth.TokenServer{}
	proto.RegisterTokenIssuerServer(s, &token)

	// 代码运行服务注册
	serve := controller.APIs.CodeRunnerSrv
	proto.RegisterCodeRunnerServer(s, &serve)

	return s
}
