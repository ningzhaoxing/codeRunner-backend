package initialize

import (
	"github.com/gin-gonic/gin"
)

func InitEngine() *gin.Engine {
	r := gin.Default()

	//router.BuildWsConnRouter(r)
	return r
}
