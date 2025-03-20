package initialize

import (
	"codeRunner-siwu/internal/application/service/client"
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/domain/client/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/docker"
	client2 "codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/interfaces/controller"
	"context"
)

func serverServiceRegister() *server.ServiceImpl {

	srv := server.NewServiceImpl(service.NewClientManagerDomainTmpl(weightedRRBalance.NewWeightedRR()))
	controller.InitSrbInject(srv)

	return srv
}

func clientServiceRegister(ctx context.Context) (*client.ServiceImpl, error) {
	containerTmpl, err := docker.NewContainerTmpl(ctx)
	if err != nil {
		return nil, err
	}

	InnerServerDomainImpl, err := entity.NewInnerServerDomainImpl(containerTmpl, client2.NewWebsocketClientImpl())
	if err != nil {
		return nil, err
	}

	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)
	return clientSvr, nil
}
