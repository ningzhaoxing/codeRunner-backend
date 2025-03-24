package controller

import (
	authAppSrv "codeRunner-siwu/internal/application/service/auth"
	serverAppSrv "codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller/auth"
	"codeRunner-siwu/internal/interfaces/controller/server"
)

type apiGroup struct {
	CodeRunnerSrv server.EndpointCtl
	Auth          auth.EndpointCtl
}

var APIs *apiGroup

func InitSrbInject(codeRunnerSrv serverAppSrv.Service, authSrv authAppSrv.Service) {
	APIs = &apiGroup{
		CodeRunnerSrv: server.EndpointCtl{Srv: codeRunnerSrv},
		Auth:          auth.EndpointCtl{Srv: authSrv},
	}
}
