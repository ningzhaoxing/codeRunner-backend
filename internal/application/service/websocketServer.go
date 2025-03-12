package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"encoding/json"
	"github.com/gorilla/websocket"
)

type RunServer interface {
	Add(*websocket.Conn, int64) string
	Remove(string)
	Execute(in *proto.ExecuteRequest) error
}

type WebsocketServer struct {
	service.ClientManagerDomain
}

func NewWebsocketServer() *WebsocketServer {
	return &WebsocketServer{
		ClientManagerDomain: service.NewClientManager(),
	}
}

func (w *WebsocketServer) Add(conn *websocket.Conn, weight int64) string {
	client := entity.NewClient(conn)
	w.ClientManagerDomain.Add(client, weight)
	return client.GetId()
}

func (w *WebsocketServer) Remove(id string) {
	w.ClientManagerDomain.Remove(id)
}

func (w *WebsocketServer) Send(conn *websocket.Conn, in *proto.ExecuteRequest) error {
	msg, err := json.Marshal(*in)
	if err != nil {
		return err
	}

	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		return err
	}
	return nil
}

func (w *WebsocketServer) Execute(in *proto.ExecuteRequest) error {
	// 通过负载均衡获取服务器conn
	server, err := w.ClientManagerDomain.GetServerByBalance()
	if err != nil {
		return err
	}

	// 将请求数据发送给内网服务器
	err = w.Send(server.GetConn(), in)
	if err != nil {
		return err
	}
	return nil
}
