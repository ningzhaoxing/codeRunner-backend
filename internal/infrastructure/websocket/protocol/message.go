package protocol

// MsgType 定义 WebSocket 消息类型
type MsgType string

const (
	MsgTypeExecute MsgType = "execute" // Server → Client：执行请求
	MsgTypeAck     MsgType = "ack"     // Client → Server：收到确认
)

// WsMessage 是 WebSocket 通信的统一消息封装
// 在原始 JSON payload 外增加 type 和 request_id 字段，用于协议层路由和 ACK 匹配
type WsMessage struct {
	Type      MsgType `json:"type"`
	RequestID string  `json:"request_id"`
	Payload   []byte  `json:"payload,omitempty"`
}
