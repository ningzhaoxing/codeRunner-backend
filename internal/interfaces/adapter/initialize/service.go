package initialize

import (
	"codeRunner-siwu/internal/application/service/auth"
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/balanceStrategy/p2cBalance"
	"codeRunner-siwu/internal/infrastructure/common/token"
	"codeRunner-siwu/internal/infrastructure/config"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	client2 "codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/interfaces/controller"
)

func serverServiceRegister() {
	/*
		依赖注入
	*/

	// token实现
	tokenImpl := token.NewToken()
	// token验证
	tokenSrv := auth.NewService(tokenImpl)
	// 负载均衡策略
	balanceStrategy := p2cBalance.NewP2CBalancer()
	// 客户端manager
	clientManagerDomain := service.NewClientManagerDomainTmpl(balanceStrategy)
	// 服务
	srv := server.NewServiceImpl(clientManagerDomain)
	// 注入
	controller.InitSrbInject(srv, tokenSrv)
}

func clientServiceRegister(poolCfg config.ContainerPoolConfig) (*client.ServiceImpl, error) {
	/*
		依赖注入
	*/

	// docker客户端
	dockerClient := docker.NewDockerClient(poolCfg)
	// docker容器
	containerTmpl := docker.NewRunCode(dockerClient)
	// websocket客户端
	websocketClientImpl := client2.NewWebsocketClientImpl()
	// 内网服务器领域
	InnerServerDomainImpl := entity.NewInnerServerDomainImpl(containerTmpl, websocketClientImpl)
	// 服务
	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)

	return clientSvr, nil
}
