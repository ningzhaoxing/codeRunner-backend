package initialize

import (
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"github.com/gin-gonic/gin"
)

func InitEngine() *gin.Engine {
	r := gin.Default()

	r.POST("/ws", ws.HandleServer())
	return r
}
