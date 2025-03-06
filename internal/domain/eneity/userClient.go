package eneity

import (
	"codeRunner-siwu/internal/infrastructure/common/utils"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
)

type UserClient struct {
	id   string
	conn *websocket.Conn
}

func NewUserClient(conn *websocket.Conn) *UserClient {
	uuid, _ := utils.GetUuid()
	return &UserClient{
		id:   uuid,
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
//func (u *UserClient) Send() {
//	u.conn.WriteMessage()
//}
