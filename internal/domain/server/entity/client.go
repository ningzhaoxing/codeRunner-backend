package entity

import (
	"codeRunner-siwu/api/proto"
	"encoding/json"

	"github.com/google/uuid"
)

type Client struct {
	id string
	WebsocketClient
}

func NewClient(client WebsocketClient) *Client {
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

func (c *Client) GetId() string {
	return c.id
}

func (c *Client) Close() error {
	return c.WebsocketClient.Close()
}

func (c *Client) IsClosed() bool {
	return c.WebsocketClient.IsClosed()
}

type WebsocketClient interface {
	Send([]byte) error
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}
