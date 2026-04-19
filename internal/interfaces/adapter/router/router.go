package router

import (
	"codeRunner-siwu/internal/agent"
	agenthandler "codeRunner-siwu/internal/agent/handler"
	serverService "codeRunner-siwu/internal/application/service/server"
	serverHandler "codeRunner-siwu/internal/interfaces/controller/server"
	"codeRunner-siwu/internal/interfaces/controller"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func ApiRouter(r *gin.Engine, svc serverService.ServerService) {
	r.GET("/ws", controller.APIs.CodeRunnerSrv.HandleServer())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.POST("/execute", serverHandler.ExecuteHandler(svc, 30*time.Second))
}

func AgentRouter(r *gin.Engine, svc *agent.AgentService) {
	g := r.Group("/agent", agenthandler.AgentAPIKeyMiddleware(svc.Cfg.APIKey))
	g.POST("/chat", agenthandler.ChatHandler(svc))
	g.POST("/confirm", agenthandler.ConfirmHandler(svc))
	g.GET("/sessions", agenthandler.ListSessionsHandler(svc))
	g.GET("/sessions/:id/messages", agenthandler.GetSessionMessagesHandler(svc))
}
