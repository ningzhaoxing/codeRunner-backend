package server

import (
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	pingInterval = 30 * time.Second // 服务端主动 Ping 间隔
	pongWait     = 60 * time.Second // 等待客户端 Pong 的超时时间
)

type WebsocketClientImpl struct {
	conn       *websocket.Conn
	isClosed   bool
	ackHandler func(requestID string) // 收到 ACK 时的回调
}

func NewWebsocketClientImpl(conn *websocket.Conn) *WebsocketClientImpl {
	return &WebsocketClientImpl{
		conn: conn,
	}
}

func (c *WebsocketClientImpl) Read() ([]byte, error) {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return nil, err
		}

		var msg protocol.WsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			// 无法解析为 WsMessage，原样返回（兼容旧格式）
			return data, nil
		}

		if msg.Type == protocol.MsgTypeAck {
			// ACK 消息：触发回调，继续等下一条消息
			if c.ackHandler != nil {
				c.ackHandler(msg.RequestID)
			}
			continue
		}

		return msg.Payload, nil
	}
}

// Send 将 payload 封装为 WsMessage 后发送，携带 requestID 用于 ACK 匹配
func (c *WebsocketClientImpl) Send(requestID string, payload []byte) error {
	msg := protocol.WsMessage{
		Type:      protocol.MsgTypeExecute,
		RequestID: requestID,
		Payload:   payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// SetAckHandler 设置收到 ACK 时的回调函数
func (c *WebsocketClientImpl) SetAckHandler(fn func(requestID string)) {
	c.ackHandler = fn
}

func (c *WebsocketClientImpl) Close() error {
	if !c.IsClosed() {
		c.isClosed = true
		err := c.conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *WebsocketClientImpl) HeartBeat() error {
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	c.conn.SetPingHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if err := c.conn.WriteMessage(websocket.PongMessage, []byte(appData)); err != nil {
			c.Close()
			return err
		}
		return nil
	})

	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if c.IsClosed() {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				logrus.Warn("heartbeat ping failed, closing connection: ", err)
				c.Close()
				return
			}
		}
	}()

	return nil
}

func (c *WebsocketClientImpl) IsClosed() bool {
	return c.isClosed
}
