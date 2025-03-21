package controller

import (
	serverAppSrv "codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller/server"
)

type apiGroup struct {
	CodeRunnerSrv server.EndpointCtl
}

var APIs *apiGroup

func InitSrbInject(codeRunnerSrv serverAppSrv.Service) {
	APIs = &apiGroup{
		CodeRunnerSrv: server.EndpointCtl{Srv: codeRunnerSrv},
	}
}
