package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	grpc2 "codeRunner-siwu/internal/infrastructure/grpc"
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
	s := grpc2.Register()
	return lis, s, etcdClient.Client
}
