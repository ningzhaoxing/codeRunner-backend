package initialize

import (
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	docker "codeRunner-siwu/internal/infrastructure/containerBasic"
	client2 "codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/interfaces/controller"
)

func serverServiceRegister() {
	// 依赖注入
	srv := server.NewServiceImpl(service.NewClientManagerDomainTmpl(weightedRRBalance.NewWeightedRR()))

	controller.InitSrbInject(srv)
}

func clientServiceRegister() (*client.ServiceImpl, error) {
	containerTmpl := docker.NewRunCode(docker.NewContainerSrvImpl())

	InnerServerDomainImpl, err := entity.NewInnerServerDomainImpl(containerTmpl, client2.NewWebsocketClientImpl())
	if err != nil {
		return nil, err
	}

	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)

	//controller.InitClientSrvInject(clientSvr)

	return clientSvr, nil
}
