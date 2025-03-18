package ws

import (
	"codeRunner-siwu/internal/application/service/server"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"strconv"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func HandleServer(websocketServer *server.ServiceTmpl) gin.HandlerFunc {
	return func(c *gin.Context) {
		weightString := c.Query("weight") // 获取服务器权重
		weight, err := strconv.ParseInt(weightString, 10, 64)
		if err != nil {
			log.Println("权重转换错误 err", err)
			return
		}

		// 升级为websocket
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		// 将该服务器添加到服务器管理
		clientId := websocketServer.Add(conn, weight)
		defer func(websocketServer *server.ServiceTmpl, id string) {
			err := websocketServer.Remove(id)
			if err != nil {
				if err != nil {
					log.Println("interfaces.controller.ws.server HandleServer() Remove err=", err)
					return
				}
			}
		}(websocketServer, clientId)

		// 启动心跳检测
		err = heartHandler(conn)
		if err != nil {
			log.Println("interfaces.controller.ws.server HandleServer() heartHandler err=", err)
			return
		}

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				log.Println("interfaces.controller.ws.server HandleServer() ReadMessage err=", err)
				return
			}
		}
	}
}

func heartHandler(conn *websocket.Conn) error {
	err := conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		return err
	}
	conn.SetPingHandler(func(string) error {
		err := conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		if err != nil {
			return err
		}
		return nil
	})
	return nil
}
