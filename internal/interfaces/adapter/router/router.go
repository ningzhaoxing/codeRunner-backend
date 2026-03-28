package router

import (
	"codeRunner-siwu/internal/interfaces/controller"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func ApiRouter(r *gin.Engine) {
	r.GET("/ws", controller.APIs.CodeRunnerSrv.HandleServer())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
