package initialize

import (
	"codeRunner-siwu/internal/application/service/auth"
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/common/token"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	client2 "codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/interfaces/controller"
)

func serverServiceRegister() {
	/*
		依赖注入
	*/

	// 日志
	log := logger.NewLoggerTmpl()
	// token实现
	tokenImpl := token.NewToken()
	// token验证
	tokenSrv := auth.NewService(tokenImpl, log)
	// 负载均衡策略
	balanceStrategy := weightedRRBalance.NewWeightedRR()
	// 客户端manager
	clientManagerDomain := service.NewClientManagerDomainTmpl(balanceStrategy)
	// 服务
	srv := server.NewServiceImpl(clientManagerDomain, log)
	// 注入
	controller.InitSrbInject(srv, tokenSrv)
}

func clientServiceRegister() (*client.ServiceImpl, error) {
	/*
		依赖注入
	*/

	// 日志
	log := logger.NewLoggerTmpl()
	// docker客户端
	dockerClient := docker.NewContainerSrvImpl()
	// docker容器
	containerTmpl := docker.NewRunCode(dockerClient)
	// websocket客户端
	websocketClientImpl := client2.NewWebsocketClientImpl()
	// 内网服务器领域
	InnerServerDomainImpl := entity.NewInnerServerDomainImpl(containerTmpl, websocketClientImpl)
	// 服务
	clientSvr := client.NewServiceImpl(InnerServerDomainImpl, log)

	return clientSvr, nil
}
