package initialize

import (
	"github.com/gin-gonic/gin"
)

func routeEngine() *gin.Engine {
	r := gin.New()

	r.Use(gin.Recovery())

	return r
}
