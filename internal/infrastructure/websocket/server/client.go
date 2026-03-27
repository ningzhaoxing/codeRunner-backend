package server

import (
	"time"

	"github.com/gorilla/websocket"
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
	c.conn.SetPingHandler(func(appData string) error {
		err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err != nil {
			c.Close()
			return err
		}
		// 回复 Pong，否则客户端认为服务端无响应
		if err := c.conn.WriteMessage(websocket.PongMessage, []byte(appData)); err != nil {
			c.Close()
			return err
		}
		return nil
	})
	return nil
}

func (c *WebsocketClientImpl) IsClosed() bool {
	return c.isClosed
}
