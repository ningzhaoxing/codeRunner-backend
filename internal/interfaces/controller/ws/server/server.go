package server

import (
	"codeRunner-siwu/internal/infrastructure/websocket/server"
	"fmt"
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

func (ctl *EndpointCtl) HandleServer() gin.HandlerFunc {
	fmt.Println("进来了")
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

		if err := ctl.Srv.Run(server.NewWebsocketClientImpl(conn), weight); err != nil {
			return
		}
	}
}
