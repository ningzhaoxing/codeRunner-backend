package controller

import (
	serverAppSrv "codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller/ws/server"
)

type apiGroup struct {
	Server server.EndpointCtl
}

var APIs *apiGroup

func InitSrbInject(serverSrv serverAppSrv.Service) {
	APIs = &apiGroup{Server: server.EndpointCtl{Srv: serverSrv}}
}
