package initialize

import (
	"codeRunner-siwu/internal/interfaces/controller/rpc"
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func RunServer() {
	ctx := context.Background()

	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 启动路由
	InitEngine()

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
	s := rpc.Register()

	// 优雅关机
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		<-c
		s.GracefulStop()
	}()

	fmt.Println("server running...")
	if err := s.Serve(lis); err != nil {
		log.Println("server-->" + err.Error())
	}
}

func RunClient() {
	ctx := context.Background()

	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	client, err := ws.NewInnerServerClient(c, ctx)
	if err != nil {
		panic(fmt.Sprintf("服务启动失败err=%s", err))
	}
	if err := client.Run(); err != nil {
		panic(fmt.Sprintf("服务启动失败err=%s", err))
	}
}
