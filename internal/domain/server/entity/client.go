package entity

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/server"
	"encoding/json"
	"github.com/google/uuid"
)

type Client struct {
	id string
	server.WebsocketClient
}

func NewClient(client server.WebsocketClient) *Client {
	id := uuid.NewString()
	return &Client{
		id:              id,
		WebsocketClient: client,
	}
}

func (c *Client) Send(request *proto.ExecuteRequest) error {
	msg, err := json.Marshal(*request)
	if err != nil {
		return err
	}

	err = c.WebsocketClient.Send(msg)
	if err != nil {
		return err
	}
	return nil
}

//func (c *Client) HeartBeat() error {
//	return c.WebsocketClient.HeartBeat()
//}

func (c *Client) GetId() string {
	return c.id
}

// 根据响应时间调整负载均衡权重

func (c *Client) Close() error {
	return c.WebsocketClient.Close()
}
