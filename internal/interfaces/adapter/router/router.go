package router

import (
	"codeRunner-siwu/internal/interfaces/controller"
	"github.com/gin-gonic/gin"
)

func ApiRouter(r *gin.Engine) {
	r.GET("/ws", controller.APIs.Server.HandleServer())
}
