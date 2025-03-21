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
}

type WebsocketClientImpl struct {
	conn *websocket.Conn
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
	return c.conn.Close()
}

func (c *WebsocketClientImpl) HeartBeat() error {
	err := c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		return err
	}

	c.conn.SetPingHandler(func(string) error {
		err := c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		if err != nil {
			return err
		}
		return nil
	})
	return nil
}
