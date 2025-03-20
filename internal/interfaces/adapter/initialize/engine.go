package initialize

import (
	"codeRunner-siwu/internal/interfaces/adapter/router"
	"github.com/gin-gonic/gin"
)

func routeEngine() *gin.Engine {
	r := gin.New()

	r.Use(gin.Recovery(), gin.Logger())

	router.ApiRouter(r)

	return r
}
