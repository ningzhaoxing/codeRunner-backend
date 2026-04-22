package client

import (
	"bytes"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type ReadResult struct {
	Request *proto.ExecuteRequest
	MsgType protocol.MsgType
}

type WebsocketClient interface {
	Dail(TargetServer) error                          // websocket客户端启动
	Read() (*ReadResult, error)                       // 读取消息
	CallBackSend(*proto.ExecuteResponse, error) error // 发送消息post到回调url
	WebsocketSend(any) error                          // 通过websocket发送消息
	Close() error
}

const (
	handshakeTimeout = 10 * time.Second
	pingPeriod       = 30 * time.Second
	pongWait         = 60 * time.Second
	writeWait        = 10 * time.Second

	reconnectInitialBackoff = 3 * time.Second
	reconnectMaxBackoff     = 30 * time.Second
	reconnectBackoffFactor  = 2
)

type WebsocketClientImpl struct {
	targetServer TargetServer

	lifecycleMu   sync.Mutex
	conn          *websocket.Conn
	currentStopCh chan struct{}
	closed        bool

	writeMu sync.Mutex

	sleepFn func(time.Duration)
}

func NewWebsocketClientImpl() *WebsocketClientImpl {
	return &WebsocketClientImpl{
		sleepFn: time.Sleep,
	}
}

// Dail 建立websocket连接
func (i *WebsocketClientImpl) Dail(targetServer TargetServer) error {
	i.targetServer = targetServer
	return i.connect()
}

// Read 读取websocket服务端消息；网络错误自愈，仅在显式 Close 或反序列化失败时返回错误
func (i *WebsocketClientImpl) Read() (*ReadResult, error) {
	for {
		i.lifecycleMu.Lock()
		conn := i.conn
		closed := i.closed
		i.lifecycleMu.Unlock()
		if closed {
			return nil, errors.ClientClosed
		}
		if conn == nil {
			return nil, errors.ClientClosed
		}

		_, m, err := conn.ReadMessage()
		if err != nil {
			zap.S().Warnf("Worker read failed, attempting reconnect: %v", err)
			if reconnectErr := i.reconnect(); reconnectErr != nil {
				return nil, reconnectErr
			}
			continue
		}

		var wsMsg protocol.WsMessage
		if err = json.Unmarshal(m, &wsMsg); err != nil {
			return nil, err
		}

		// Only execute needs ACK; execute_sync uses result message as confirmation
		if wsMsg.Type == protocol.MsgTypeExecute {
			ack, _ := json.Marshal(protocol.WsMessage{
				Type:      protocol.MsgTypeAck,
				RequestID: wsMsg.RequestID,
			})
			i.writeMu.Lock()
			writeErr := conn.WriteMessage(websocket.TextMessage, ack)
			i.writeMu.Unlock()
			if writeErr != nil {
				zap.S().Warn("infrastructure-websocket-client Read() send ACK failed: ", writeErr)
			}
		}

		msg := new(proto.ExecuteRequest)
		if err = json.Unmarshal(wsMsg.Payload, msg); err != nil {
			return nil, err
		}
		return &ReadResult{Request: msg, MsgType: wsMsg.Type}, nil
	}
}

const (
	callbackTimeout    = 10 * time.Second
	callbackMaxRetries = 3
)

var callbackHTTPClient = &http.Client{Timeout: callbackTimeout}

// CallBackSend 将msg通过post发送到回调url，失败时指数退避重试最多3次
func (i *WebsocketClientImpl) CallBackSend(msg *proto.ExecuteResponse, err error) error {
	if err != nil {
		msg.Err = err.Error()
	}

	data, err := json.Marshal(*msg)
	if err != nil {
		zap.S().Error("infrastructure-websocket-client CallBackSend() json.Marshal err=", err)
		return err
	}

	var lastErr error
	for attempt := 0; attempt < callbackMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second) // 1s, 2s 退避
			zap.S().Infof("CallBackSend 第 %d 次重试, requestId=%s", attempt, msg.Id)
		}

		req, err := http.NewRequest("POST", msg.CallBackUrl, bytes.NewBuffer(data))
		if err != nil {
			zap.S().Error("infrastructure-websocket-client CallBackSend() NewRequest err=", err)
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", msg.Id)

		resp, err := callbackHTTPClient.Do(req)
		if err != nil {
			zap.S().Errorf("infrastructure-websocket-client CallBackSend() Do err=%v (attempt %d)", err, attempt+1)
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			zap.S().Errorf("infrastructure-websocket-client CallBackSend() status=%d (attempt %d)", resp.StatusCode, attempt+1)
			lastErr = errors.ResultSendFail
			continue
		}

		return nil
	}

	zap.S().Errorf("infrastructure-websocket-client CallBackSend() 重试耗尽，结果丢失 requestId=%s err=%v", msg.Id, lastErr)
	return lastErr
}

func (i *WebsocketClientImpl) WebsocketSend(data any) error {
	msg, err := json.Marshal(data)
	if err != nil {
		zap.S().Error("infrastructure-websocket-client WebsocketSend() json.Marshal err=", err)
		return err
	}

	i.lifecycleMu.Lock()
	conn := i.conn
	i.lifecycleMu.Unlock()
	if conn == nil {
		return errors.ClientClosed
	}

	i.writeMu.Lock()
	defer i.writeMu.Unlock()
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		zap.S().Error("infrastructure-websocket-client WebsocketSend() conn.WriteMessage err=", err)
		return err
	}
	return nil
}

// Close 幂等地关闭客户端，停止心跳并断开连接
func (i *WebsocketClientImpl) Close() error {
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()
	if i.closed {
		return nil
	}
	i.closed = true
	if i.currentStopCh != nil {
		close(i.currentStopCh)
		i.currentStopCh = nil
	}
	if i.conn != nil {
		err := i.conn.Close()
		i.conn = nil
		return err
	}
	return nil
}

// connect 建立连接并装配 deadline / handler / heartbeat
func (i *WebsocketClientImpl) connect() error {
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	url := fmt.Sprintf("ws://%s:%s/%s?%s", i.targetServer.host, i.targetServer.port, i.targetServer.path, i.targetServer.rowQuery)

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		zap.S().Errorf("Worker failed to connect to server %s: %v", url, err)
		return err
	}

	i.lifecycleMu.Lock()
	if i.closed {
		i.lifecycleMu.Unlock()
		_ = conn.Close()
		return errors.ClientClosed
	}
	if i.currentStopCh != nil {
		close(i.currentStopCh)
	}
	stopCh := make(chan struct{})
	i.currentStopCh = stopCh
	i.conn = conn
	i.lifecycleMu.Unlock()

	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		i.writeMu.Lock()
		defer i.writeMu.Unlock()
		err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
		if err == websocket.ErrCloseSent {
			return nil
		}
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return nil
		}
		return err
	})

	zap.S().Infof("Worker connected to server: %s", url)
	go i.heartBeat(conn, stopCh)
	return nil
}

// heartBeat 周期性发送 ping；失败直接退出，由 Read() 自愈触发重连
func (i *WebsocketClientImpl) heartBeat(conn *websocket.Conn, stopCh chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.writeMu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait))
			i.writeMu.Unlock()
			if err != nil {
				zap.S().Warnf("Worker heartbeat ping failed, exiting heartbeat goroutine: %v", err)
				return
			}
			zap.S().Debug("Worker sent heartbeat ping")
		case <-stopCh:
			zap.S().Debug("Worker heartbeat stopped")
			return
		}
	}
}

// reconnect 关闭旧连接并指数退避无限重试，仅在显式 Close 时退出
func (i *WebsocketClientImpl) reconnect() error {
	i.lifecycleMu.Lock()
	if i.closed {
		i.lifecycleMu.Unlock()
		return errors.ClientClosed
	}
	if i.currentStopCh != nil {
		close(i.currentStopCh)
		i.currentStopCh = nil
	}
	if i.conn != nil {
		_ = i.conn.Close()
		i.conn = nil
	}
	i.lifecycleMu.Unlock()

	backoff := reconnectInitialBackoff
	for attempt := 1; ; attempt++ {
		i.lifecycleMu.Lock()
		closed := i.closed
		i.lifecycleMu.Unlock()
		if closed {
			return errors.ClientClosed
		}

		zap.S().Infof("Worker reconnecting (attempt %d, backoff=%s) to %s:%s/%s",
			attempt, backoff, i.targetServer.host, i.targetServer.port, i.targetServer.path)
		if err := i.connect(); err == nil {
			zap.S().Info("Worker reconnect succeeded")
			return nil
		} else {
			zap.S().Warnf("Worker reconnect attempt %d failed: %v", attempt, err)
		}

		i.sleepFn(backoff)
		if backoff < reconnectMaxBackoff {
			backoff *= reconnectBackoffFactor
			if backoff > reconnectMaxBackoff {
				backoff = reconnectMaxBackoff
			}
		}
	}
}
