package server

import (
	"github.com/gorilla/websocket"
	"time"
)

type WebsocketClient interface {
	Send([]byte) error
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}

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
	c.conn.SetPingHandler(func(string) error {
		err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err != nil {
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
