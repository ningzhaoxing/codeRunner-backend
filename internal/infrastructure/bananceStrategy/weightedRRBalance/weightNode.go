package weightedRRBalance

import "github.com/gorilla/websocket"

type WeightNode struct {
	serverId  string // 服务器id
	conn      *websocket.Conn
	curWeight int64 // 当前权重
	weight    int64 // 初始权重
}

func NewWeightNode(serverId string, conn *websocket.Conn, weight int64) *WeightNode {
	return &WeightNode{
		serverId: serverId,
		conn:     conn,
		weight:   weight,
	}
}

func (w *WeightNode) GetConn() *websocket.Conn {
	return w.conn
}
