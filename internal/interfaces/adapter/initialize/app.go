package initialize

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func Run() {
	ctx := context.Background()

	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 启动路由
	InitEngine()

	// 注册etcd服务
	etcdClient, err := InitEtcd(ctx, c)
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

	// 创建grpc服务
	s := grpc.NewServer()

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
