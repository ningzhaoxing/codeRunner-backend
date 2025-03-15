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
}

type Client struct {
	conn          *websocket.Conn // websocket连接
	pingPeriod    time.Duration   // 心跳检测时间间隔
	pongWait      time.Duration   // 心跳响应等待时间
	stopPingCh    chan struct{}   // 用于通知心跳检测结束
	reconnectWait time.Duration   // 重连等待时间
	targetServer  TargetServer    // 目标服务器
}

func NewClient() *Client {
	return &Client{
		// 设置默认心跳参数
		pingPeriod: 10 * time.Second,
		pongWait:   60 * time.Second,
		stopPingCh: make(chan struct{}),
		// 设置重连等待时间
		reconnectWait: 5 * time.Second,
	}
}

// Dail 建立websocket连接
func (i *Client) Dail(targetServer TargetServer) error {
	i.targetServer = targetServer
	return i.connect()
}

// 连接方法
func (i *Client) connect() error {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// 建立连接
	url := "ws://localhost:7979/ws?weight=1"
	//url := fmt.Sprintf("ws://%s:%s/%s?%s", i.targetServer.host, i.targetServer.port, i.targetServer.path, i.targetServer.rowQuery)
	conn, _, err := dialer.Dial(url, nil)

	if err != nil {
		log.Println("内网服务器客户端发起链接失败 err=", err)
		return err
	}
	i.conn = conn

	// 启动心跳检测
	go i.startPing()

	return nil
}

// 重连方法
func (i *Client) reconnect() error {
	err := i.conn.Close()
	if err != nil {
		log.Println("鏈接關閉失敗")
		return err
	}
	log.Println("鏈接已被關閉")

	for {
		log.Println("尝试重新连接...")
		err := i.connect()
		if err == nil {
			log.Println("重连成功")
			return nil
		}
		log.Printf("重连失败: %v, %d秒后重试\n", err, i.reconnectWait/time.Second)
		time.Sleep(i.reconnectWait)
	}
}

// 心跳检测方法
func (i *Client) startPing() {
	// 初始化心跳检测定时器
	ticker := time.NewTicker(i.pingPeriod)
	defer ticker.Stop()

	if i.conn != nil {
		fmt.Println("------------")
	}
	// 进行心跳检测
	for {
		select {
		// 定时发送
		case <-ticker.C:
			if err := i.sendPing(); err != nil {
				log.Println("发送心跳失败:", err)
				// 心跳失败时尝试重连
				if err := i.reconnect(); err != nil {
					log.Println("重连失败，停止心跳检测")
					return
				}
			}
			fmt.Println("心跳發送成功！")
		// 结束心跳检测
		case <-i.stopPingCh:
			return
		}
	}
}

// 发送ping
func (i *Client) sendPing() error {
	return i.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(4*time.Second))
}

// 读取websocket服务端消息
func (i *Client) Read() (*proto.ExecuteRequest, error) {
	_, m, err := i.conn.ReadMessage()
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer的Read()  err=", err)
		return nil, err
	}

	var msg *proto.ExecuteRequest

	if err = json.Unmarshal(m, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

// Send 将msg通过post发送到回调url
func (i *Client) Send(msg *proto.ExecuteResponse) error {
	// 序列化msg
	data, err := json.Marshal(*msg)
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer Send() 的 json.Marshal err=", err)
		return err
	}
	fmt.Println(bytes.NewBuffer(data))
	// 发送msg
	req, err := http.NewRequest("POST", msg.CallBackUrl, bytes.NewBuffer(data))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("infrastructure-websocket-client innerServer Send() 的 client.Do err=", err)
		return err
	}

	if resp.StatusCode != 200 {
		return errors.ResultSendFail
	}
	return nil
}

// Close 关闭websocket客户端
func (i *Client) Close() error {
	// 停止心跳检测
	close(i.stopPingCh)
	return i.conn.Close()
}
