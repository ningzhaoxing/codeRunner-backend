package entity

import (
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	id   string
	conn *websocket.Conn
}

func NewClient(conn *websocket.Conn) *Client {
	id := uuid.NewString()
	return &Client{
		id:   id,
		conn: conn,
	}
}

func (c *Client) GetId() string {
	return c.id
}

func (c *Client) GetConn() *websocket.Conn {
	return c.conn
}

func (c *Client) Close() error {
	return c.conn.Close()
}
