package ws

import (
	"codeRunner-siwu/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleConn() gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println("interfaces.controller.ws.server.HandleConn().Upgrade err=", err)
			return
		}
		defer conn.Close()

		// 将该服务器注册到负载均衡管理
		err = service.NewHandleServer().RegisterServerLoadBalance(conn)
		if err != nil {
			log.Println("interfaces.controller.ws.server.HandleConn().Upgrade err=", err)
			return
		}

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}
}
