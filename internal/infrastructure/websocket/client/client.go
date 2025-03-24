package client

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

type WebsocketClient interface {
	Dail(TargetServer) error              // websocket客户端启动
	Read() (*proto.ExecuteRequest, error) // 读取消息
	Send(*proto.ExecuteResponse) error    // 发送消息post到调用者
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
		pingPeriod:       10 * time.Second,
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

// 读取websocket服务端消息
func (i *WebsocketClientImpl) Read() (*proto.ExecuteRequest, error) {
	_, m, err := i.conn.ReadMessage()
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer的Read()  err=", err)
		return nil, err
	}

	fmt.Println(string(m))

	msg := new(proto.ExecuteRequest)
	if err = json.Unmarshal(m, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// Send 将msg通过post发送到回调url
func (i *WebsocketClientImpl) Send(msg *proto.ExecuteResponse) error {
	// 序列化msg
	data, err := json.Marshal(*msg)
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer Send() 的 json.Marshal err=", err)
		return err
	}

	// 发送msg
	req, err := http.NewRequest("POST", msg.CallBackUrl, bytes.NewBuffer(data))
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer Send() 的 client.NewRequest err=", err)
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer Send() 的 client.Do err=", err)
		return err
	}
	if resp.StatusCode != 200 {
		log.Println("infrastructure-websocket-client innerServer Send() 的 resp.StatusCode 发送失败", resp.StatusCode)
		return errors.ResultSendFail
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
		log.Println("内网服务器客户端发起链接失败 err=", err)
		return err
	}

	i.conn = conn

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
		log.Println("尝试重新连接...")
		err := i.connect()
		if err == nil {
			log.Println("重连成功")
			return nil
		}
		maxAttempts--
		if maxAttempts <= 0 {
			log.Printf("重连失败已达%d次，停止重试", maxAttempts)
			return errors.MaxRetryAttemptsReached
		}
		log.Printf("重连失败: %v, %d秒后重试\n", err, i.reconnectWait/time.Second)
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
			if err := i.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
				log.Println("发送心跳失败:", err)
				if err := i.reconnect(); err != nil {
					log.Println("重连失败，停止心跳检测")
					return
				}
			}
		case <-i.stopPingCh:
			return
		}
	}
}
