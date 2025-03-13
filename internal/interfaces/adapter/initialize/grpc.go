package initialize

import (
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/interfaces/controller/rpc"
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"net"
)

func InitGrpc(websocketServer *service.WebsocketServer, c *config.Config, ctx context.Context) (net.Listener, *grpc.Server) {
	// 将grpc服务注册到etcd
	etcdClient, err := EtcdRegister(ctx, c)
	if err != nil {
		panic("服务注册失败" + err.Error())
	}

	defer func(Client *clientv3.Client) {
		err := Client.Close()
		if err != nil {
			fmt.Println("服务注册失败" + err.Error())
		}
	}(etcdClient.Client)

	// 启动grpc服务
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", c.Grpc.Host, c.Grpc.Port))
	if err != nil {
		panic("grpc服务启动失败" + err.Error())
	}

	// 注册grpc服务
	s := rpc.Register(websocketServer)
	return lis, s
}
