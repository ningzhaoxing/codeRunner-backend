package eneity

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
)

type UserClient struct {
	id   string
	conn *websocket.Conn
}

func NewUserClient(conn *websocket.Conn, id string) *UserClient {
	return &UserClient{
		id:   id,
		conn: conn,
	}
}

// 读取消息
func (u *UserClient) Read() error {
	_, msg, err := u.conn.ReadMessage()
	if err != nil {
		log.Println("userClient消息读取失败", err)
		return err
	}
	fmt.Println(msg)
	return nil
}

// Send 发送消息
func (u *UserClient) Send(msg string) error {
	err := u.conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		log.Println("userClient消息读取失败", err)
		return err
	}
	return nil
}
