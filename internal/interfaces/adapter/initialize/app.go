package initialize

import (
	"context"
	"fmt"
	etcd "go.etcd.io/etcd/client/v3"
	"log"
)

func RunServer() {
	ctx := context.Background()
	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 初始化日志
	InitLogger()

	// 服务注册
	srv := serverServiceRegister()

	go func() {
		url := fmt.Sprintf("%s:%s", c.Server.App.Host, c.Server.App.Port)
		fmt.Println(url)
		if err := routeEngine().Run(url); err != nil {
			panic("http服务启动失败" + err.Error())
		}
	}()

	// 启动grpc
	lis, s, client := InitGrpc(srv, c, ctx)
	defer func(Client *etcd.Client) {
		err := Client.Close()
		if err != nil {
			log.Println("服务关闭失败" + err.Error())
		}
	}(client)

	fmt.Println("server running...")
	if err := s.Serve(lis); err != nil {
		log.Println("server-->" + err.Error())
	}
}

func RunClient() {
	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 初始化日志
	InitLogger()

	srv, err := clientServiceRegister()
	if err != nil {
		panic("服务注册失败,原因:" + err.Error())
	}

	if err := srv.Run(*c); err != nil {
		log.Println(fmt.Sprintf("服务启动失败err=%s\n", err))
	}
}
