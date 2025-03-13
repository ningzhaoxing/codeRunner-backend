package ws

import (
	"codeRunner-siwu/internal/application/service"
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

var WebsocketServer *service.WebsocketServer

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

		WebsocketServer = service.NewWebsocketServer()
		// 将该服务器添加到服务器管理
		clientId := WebsocketServer.Add(conn, weight)

		// 启动心跳检测
		heartBeat(conn, clientId, WebsocketServer)
	}
}

func heartBeat(conn *websocket.Conn, clientId string, websocketServer *service.WebsocketServer) {
	// 设置心跳检测参数
	pingPeriod := 30 * time.Second
	pongWait := 60 * time.Second

	// 设置读取超时时间
	err := conn.SetReadDeadline(time.Now().Add(pongWait))
	if err != nil {
		log.Println("controller.ws.HandleServer() SetReadDeadline err=", err)
		return
	}

	// 设置 pong 处理器
	conn.SetPongHandler(func(string) error {
		// 收到 pong 后延长读取超时时间
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// 启动心跳检测
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 发送 ping 消息
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					// 心跳检测失败时，移除连接
					websocketServer.Remove(clientId)
					log.Println("发送心跳失败:", err)
					return
				}
			}
		}
	}()
}
