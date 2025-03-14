package initialize

import (
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func RunServer() {
	ctx := context.Background()

	// 创建websocket服务端实例(管理websocket客户端)，依赖注入
	websocketServer := service.NewWebsocketServer()

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
	lis, s := InitGrpc(websocketServer, c, ctx)

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		panic(fmt.Sprintf("服务启动失败err=%s\n", err))
	}
}
