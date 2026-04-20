package client

import (
	"bytes"
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"net/http"
	"time"
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

type WebsocketClientImpl struct {
	conn             *websocket.Conn // websocket连接
	handshakeTimeout time.Duration   // 连接时间限制
	pingPeriod       time.Duration   // 心跳检测时间间隔
	pongWait         time.Duration   // 心跳响应等待时间
	reconnectWait    time.Duration   // 重连等待时间
	stopPingCh       chan struct{}   // 用于通知心跳检测结束
	targetServer     TargetServer    // 需要连接的目标服务器
}

func NewWebsocketClientImpl() *WebsocketClientImpl {
	return &WebsocketClientImpl{
		handshakeTimeout: 10 * time.Second,
		pingPeriod:       2 * time.Second,
		pongWait:         60 * time.Second,
		stopPingCh:       make(chan struct{}),
		reconnectWait:    10 * time.Second,
	}
}

// Dail 建立websocket连接
func (i *WebsocketClientImpl) Dail(targetServer TargetServer) error {
	i.targetServer = targetServer
	// 建立连接
	if err := i.connect(); err != nil {
		return err
	}
	return nil
}

// 读取websocket服务端消息，execute 消息立即回 ACK，execute_sync 使用 result 消息作为确认
func (i *WebsocketClientImpl) Read() (*ReadResult, error) {
	_, m, err := i.conn.ReadMessage()
	if err != nil {
		zap.S().Error("infrastructure-websocket-client Read() err=", err)
		return nil, err
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
		if writeErr := i.conn.WriteMessage(websocket.TextMessage, ack); writeErr != nil {
			zap.S().Warn("infrastructure-websocket-client Read() send ACK failed: ", writeErr)
		}
	}

	msg := new(proto.ExecuteRequest)
	if err = json.Unmarshal(wsMsg.Payload, msg); err != nil {
		return nil, err
	}
	return &ReadResult{Request: msg, MsgType: wsMsg.Type}, nil
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
			// URL 格式有误，重试无意义
			zap.S().Error("infrastructure-websocket-client CallBackSend() NewRequest err=", err)
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-ID", msg.Id) // 调用方可用于幂等去重

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
		zap.S().Error("infrastructure-websocket-client innerServer WebsocketSend() 的 json.Marshal err=", err)
		return err
	}

	err = i.conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		zap.S().Error("infrastructure-websocket-client innerServer WebsocketSend() 的 conn.WriteMessage err=", err)
		return err
	}
	return nil
}

// Close 关闭websocket客户端
func (i *WebsocketClientImpl) Close() error {
	// 停止心跳检测
	close(i.stopPingCh)
	return i.conn.Close()
}

// websocket客户端建立连接
func (i *WebsocketClientImpl) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: i.handshakeTimeout,
	}

	// 建立连接
	//url := "ws://192.168.23.31:7979/ws?weight=1"
	url := fmt.Sprintf("ws://%s:%s/%s?%s", i.targetServer.host, i.targetServer.port, i.targetServer.path, i.targetServer.rowQuery)

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		zap.S().Errorf("Worker failed to connect to server %s: %v", url, err)
		return err
	}

	i.conn = conn
	zap.S().Infof("Worker connected to server: %s", url)

	// 启动心跳
	go i.heartBeat()
	return nil
}

// 重连方法
func (i *WebsocketClientImpl) reconnect() error {
	// 关闭客户端
	if err := i.Close(); err != nil {
		return err
	}

	// 开始重连
	maxAttempts := 3

	for {
		zap.S().Infof("Worker reconnecting to server %s:%s/%s (remaining attempts: %d)", i.targetServer.host, i.targetServer.port, i.targetServer.path, maxAttempts)
		err := i.connect()
		if err == nil {
			zap.S().Info("Worker reconnect succeeded")
			return nil
		}
		maxAttempts--
		if maxAttempts <= 0 {
			zap.S().Error("Worker reconnect failed after max attempts")
			return errors.MaxRetryAttemptsReached
		}
		zap.S().Errorf("Worker reconnect failed: %v, retrying in %ds", err, int(i.reconnectWait/time.Second))
		time.Sleep(i.reconnectWait)
	}
}

// 心跳检测
func (i *WebsocketClientImpl) heartBeat() {
	// 初始化心跳检测定时器
	ticker := time.NewTicker(i.pingPeriod)
	defer ticker.Stop()

	// 进行心跳检测
	for {
		select {
		case <-ticker.C:
			zap.S().Debug("Worker sending heartbeat ping")
			if err := i.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(3*time.Second)); err != nil {
				zap.S().Errorf("Worker heartbeat failed: %v", err)
				if err := i.reconnect(); err != nil {
					zap.S().Error("Worker reconnect failed, stopping heartbeat")
					return
				}
			}
		case <-i.stopPingCh:
			zap.S().Info("Worker heartbeat stopped")
			return
		}
	}
}
