package server

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	pingInterval = 30 * time.Second // 服务端主动 Ping 间隔
	pongWait     = 60 * time.Second // 等待客户端 Pong 的超时时间
)

type WebsocketClientImpl struct {
	conn     *websocket.Conn
	isClosed bool
}

func NewWebsocketClientImpl(conn *websocket.Conn) *WebsocketClientImpl {
	return &WebsocketClientImpl{
		conn: conn,
	}
}

func (c *WebsocketClientImpl) Read() ([]byte, error) {
	_, msg, err := c.conn.ReadMessage()
	return msg, err
}

func (c *WebsocketClientImpl) Send(msg []byte) error {
	return c.conn.WriteMessage(websocket.TextMessage, msg)
}

func (c *WebsocketClientImpl) Close() error {
	if !c.IsClosed() {
		c.isClosed = true
		err := c.conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *WebsocketClientImpl) HeartBeat() error {
	// 设置初始读超时
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	// 收到客户端 Ping 时回复 Pong
	c.conn.SetPingHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if err := c.conn.WriteMessage(websocket.PongMessage, []byte(appData)); err != nil {
			c.Close()
			return err
		}
		return nil
	})

	// 收到客户端 Pong 时刷新读超时
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// 服务端主动发 Ping 探活
	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if c.IsClosed() {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logrus.Warn("heartbeat ping failed, closing connection: ", err)
				c.Close()
				return
			}
		}
	}()

	return nil
}

func (c *WebsocketClientImpl) IsClosed() bool {
	return c.isClosed
}
