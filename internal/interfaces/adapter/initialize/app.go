package initialize

import (
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"log"
)

func RunServer() {
	ctx := context.Background()

	// 创建websocket服务端实例(管理websocket客户端)，依赖注入
	websocketServer := server.NewServiceTmpl()

	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 初始化日志
	InitLogger()

	// 启动websocket服务
	go InitEngine(websocketServer, c)

	// 启动grpc
	lis, s, client := InitGrpc(websocketServer, c, ctx)
	defer func(Client *clientv3.Client) {
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
	ctx := context.Background()
	// 初始化配置
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	// 初始化日志
	InitLogger()

	// 自定义权重
	var weight int64 = 1

	client, err := ws.NewInnerServerClient(c, ctx, weight)
	if err != nil {
		panic(fmt.Sprintf("服务启动失败err=%s\n", err))
	}

	if err := client.Run(); err != nil {
		log.Println(fmt.Sprintf("服务启动失败err=%s\n", err))
	}
}
