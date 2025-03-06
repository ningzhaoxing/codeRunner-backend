package eneity

import (
	"codeRunner-siwu/internal/infrastructure/common/errors"
	errors2 "errors"
	"fmt"
	"github.com/gorilla/websocket"
	"sync"
)

type Server struct {
	userClients       sync.Map
	innerServerClient *websocket.Conn
}

// NewServer 创建服务端
func NewServer(conn *websocket.Conn) *Server {
	return &Server{
		userClients:       sync.Map{},
		innerServerClient: conn,
	}
}

func (s *Server) LoadUserClient(id string) (*websocket.Conn, error) {
	con, ok := s.userClients.Load(id)
	var err error
	if !ok {
		err = errors.UserClientNotExist
	}

	conn := con.(*websocket.Conn)
	return conn, err
}

func (s *Server) StoreUserClient(id string, conn *websocket.Conn) error {
	_, err := s.LoadUserClient(id)

	if errors2.Is(err, errors.UserClientNotExist) {
		s.userClients.Store(id, conn)
		return nil
	}
	return err
}

func (s *Server) DeleteUserClient(id string) error {
	_, err := s.LoadUserClient(id)
	if err != nil {
		return err
	}

	s.userClients.Delete(id)
	return nil
}

func (s *Server) Read() error {
	_, msg, err := s.innerServerClient.ReadMessage()
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func (s *Server) Send(id, msg string) error {
	conn, err := s.LoadUserClient(id)
	if err != nil {
		return err
	}

	err = conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		return err
	}
	return nil
}
