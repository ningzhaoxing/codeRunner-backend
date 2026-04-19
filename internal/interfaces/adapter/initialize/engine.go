package initialize

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func routeEngine() *gin.Engine {
	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Agent-API-Key"},
		ExposeHeaders:    []string{"Content-Length"},
	}))

	return r
}
