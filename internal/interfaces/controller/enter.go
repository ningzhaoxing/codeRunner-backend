package controller

import (
	authAppSrv "codeRunner-siwu/internal/application/service/auth"
	serverAppSrv "codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/interfaces/controller/auth"
	ctrlFeedback "codeRunner-siwu/internal/interfaces/controller/feedback"
	"codeRunner-siwu/internal/interfaces/controller/server"
)

type apiGroup struct {
	CodeRunnerSrv server.EndpointCtl
	Auth          auth.EndpointCtl
	FeedbackSvc   ctrlFeedback.FeedbackService
}

var APIs *apiGroup

func InitSrbInject(codeRunnerSrv serverAppSrv.ServerService, authSrv authAppSrv.Service, feedbackSvc ctrlFeedback.FeedbackService) {
	APIs = &apiGroup{
		CodeRunnerSrv: server.EndpointCtl{Srv: codeRunnerSrv},
		Auth:          auth.EndpointCtl{Srv: authSrv},
		FeedbackSvc:   feedbackSvc,
	}
}
