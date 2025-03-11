package ws

import (
	"codeRunner-siwu/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleServer() gin.HandlerFunc {
	return func(c *gin.Context) {
		weightString := c.Query("weight") // 获取服务器权重
		weight, err := strconv.ParseInt(weightString, 10, 64)

		if err != nil {
			log.Println("权重转换错误 err", err)
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		websocketServer := service.NewWebsocketServer()
		// 将该服务器添加到服务器管理
		websocketServer.Add(conn, weight)

		// 阻塞连接
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}
}
