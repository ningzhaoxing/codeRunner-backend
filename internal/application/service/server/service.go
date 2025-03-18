package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/domain/server/service"
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
)

type Service interface {
	Add(*websocket.Conn, int64) string
	Remove(string) error
	Execute(in *proto.ExecuteRequest) error
}

type ServiceTmpl struct {
	service.ClientManagerDomain
}

func NewServiceTmpl() *ServiceTmpl {
	return &ServiceTmpl{
		ClientManagerDomain: service.NewClientManager(),
	}
}

func (w *ServiceTmpl) Add(conn *websocket.Conn, weight int64) string {
	client := entity.NewClient(conn)
	w.ClientManagerDomain.Add(client, weight)
	return client.GetId()
}

func (w *ServiceTmpl) Remove(id string) error {
	return w.ClientManagerDomain.Remove(id)
}

func (w *ServiceTmpl) Send(conn *websocket.Conn, in *proto.ExecuteRequest) error {
	msg, err := json.Marshal(*in)
	if err != nil {
		log.Println("application.service.Send() Marshal err=", err)
		return err
	}

	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		log.Println("application.service.Send() WriteMessage err=", err)
		return err
	}
	return nil
}

func (w *ServiceTmpl) Execute(in *proto.ExecuteRequest) error {
	// 通过负载均衡获取服务器conn
	server, err := w.ClientManagerDomain.GetServerByBalance()
	if err != nil {
		log.Println("application.service.Send() Execute err=", err)
		return err
	}

	// 将请求数据发送给内网服务器
	err = w.Send(server.GetConn(), in)
	if err != nil {
		log.Println("application.service.Send() Send err=", err)
		return err
	}
	return nil
}
