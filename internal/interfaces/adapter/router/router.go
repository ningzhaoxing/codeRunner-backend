package router

import (
	"codeRunner-siwu/internal/agent"
	agenthandler "codeRunner-siwu/internal/agent/handler"
	"codeRunner-siwu/internal/interfaces/controller"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func ApiRouter(r *gin.Engine) {
	r.GET("/ws", controller.APIs.CodeRunnerSrv.HandleServer())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

func AgentRouter(r *gin.Engine, svc *agent.AgentService) {
	g := r.Group("/agent", agenthandler.AgentAPIKeyMiddleware(svc.Cfg.APIKey))
	g.POST("/chat", agenthandler.ChatHandler(svc))
	g.POST("/confirm", agenthandler.ConfirmHandler(svc))
}
