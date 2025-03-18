package initialize

import (
	"codeRunner-siwu/internal/application/service/server"
	"codeRunner-siwu/internal/infrastructure/config"
	"codeRunner-siwu/internal/interfaces/controller/ws"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

func InitEngine(websocketServer *server.ServiceTmpl, c *config.Config) {
	r := gin.Default()

	r.GET("/ws", ws.HandleServer(websocketServer))

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", c.App.Host, c.App.Port),
		Handler: r,
	}
	err := server.ListenAndServe()
	if err != nil {
		log.Println("server-->", err)
		return
	}
}
