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
	payload, err := json.Marshal(*request)
	if err != nil {
		return err
	}
	return c.WebsocketClient.Send(request.Id, payload)
}

func (c *Client) SetAckHandler(fn func(requestID string)) {
	c.WebsocketClient.SetAckHandler(fn)
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
	Send(requestID string, payload []byte) error
	SetAckHandler(fn func(requestID string))
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}
