package innerServer

import (
	"bytes"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"net/http"
	"time"
)

type ServerClient interface {
	Dail(TargetServer) error
	Read() (*proto.ExecuteRequest, error)
	SendToServer(*proto.ExecuteResponse) error
}

type InnerServerClient struct {
	conn *websocket.Conn
}

func NewInnerServerClient() *InnerServerClient {
	return &InnerServerClient{}
}

func (i *InnerServerClient) Dail(targetServer TargetServer) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	url := fmt.Sprintf("ws://%s:%s/%s/%s", targetServer.host, targetServer.port, targetServer.path, targetServer.rowQuery)
	fmt.Println(url)
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		log.Println("内网服务器客户端发起链接失败 err=", err)
		return err
	}

	i.conn = conn
	return nil
}

func (i *InnerServerClient) Read() (*proto.ExecuteRequest, error) {
	_, m, err := i.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg *proto.ExecuteRequest

	if err = json.Unmarshal(m, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func (i *InnerServerClient) SendToServer(msg *proto.ExecuteResponse) error {
	data, err := json.Marshal(*msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", msg.CallBackUrl, bytes.NewBuffer(data))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		return errors.ResultSendFail
	}
	return nil
}
