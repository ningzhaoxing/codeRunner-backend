package initialize

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeRunner-siwu/internal/agent"
	"codeRunner-siwu/internal/infrastructure/metrics"
	"codeRunner-siwu/internal/interfaces/adapter/router"
	etcd "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
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
	err = InitLogger(c)
	if err != nil {
		panic("日志文件解析错误" + err.Error())
		return
	}

	// 服务注册
	serverSvc := serverServiceRegister(c)

	go func() {
		url := fmt.Sprintf("%s:%s", c.Server.App.Host, c.Server.App.Port)
		fmt.Println(url)
		r := routeEngine()
		router.ApiRouter(r, serverSvc)
		if c.Agent.Enabled {
			agentSvc, err := agent.NewAgentService(ctx, c.Agent, ".")
			if err != nil {
				zap.S().Warnf("agent service init failed, agent routes disabled: %v", err)
			} else {
				agentSvc.Executor = agent.NewCodeExecutor(serverSvc, 30*time.Second)
				router.AgentRouter(r, agentSvc)
			}
		}
		if err := r.Run(url); err != nil {
			panic("http服务启动失败" + err.Error())
		}
	}()

	// 启动grpc
	lis, s, client := InitGrpc(c, ctx)
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
	err = InitLogger(c)
	if err != nil {
		panic("日志文件解析错误" + err.Error())
		return
	}
	srv, pool, err := clientServiceRegister(c.Client.ContainerPool)
	if err != nil {
		panic("服务注册失败,原因:" + err.Error())
	}

	// 启动 metrics push（worker 在内网，云端 Prometheus 抓不到，改为 push 到 pushgateway）
	hostname, _ := os.Hostname()
	stopPush := metrics.StartPusher(c.Client.Metrics.PushgatewayURL, c.Client.Metrics.JobName, hostname, c.Client.Metrics.PushInterval)

	// 优雅关停：监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		zap.S().Info("收到关停信号，正在关闭容器池...")
		if stopPush != nil {
			stopPush()
		}
		pool.Close()
		os.Exit(0)
	}()

	if err := srv.Run(*c); err != nil {
		log.Println(fmt.Sprintf("服务启动失败err=%s\n", err))
	}
}
