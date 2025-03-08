package bananceStrategy

import (
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"github.com/gorilla/websocket"
)

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

type WeightedRR struct {
	nodes       []*WeightNode // 所有的服务实例
	totalWeight int64
}

func NewWeightedRR() *WeightedRR {
	return &WeightedRR{
		nodes:       nil,
		totalWeight: 0,
	}
}

func (w *WeightedRR) Add(rr *WeightNode) {
	w.nodes = append(w.nodes, &WeightNode{
		serverId:  rr.serverId,
		curWeight: 0,
		weight:    rr.weight,
	})
	w.totalWeight += rr.weight
}

func (w *WeightedRR) Remove(serverId string) {
	for i, node := range w.nodes {
		if node.serverId == serverId {
			w.nodes = append(w.nodes[:i], w.nodes[i+1:]...)
		}
	}
}

func (w *WeightedRR) UpdateWeight(serverId string, weight int64) {
	for _, node := range w.nodes {
		if node.serverId == serverId {
			// 平滑过渡：保留原有currentWeight的50%
			node.curWeight = node.curWeight * weight / node.weight
			node.weight = weight
			return
		}
	}
}

func (w *WeightedRR) Next() (*WeightNode, error) {
	if len(w.nodes) == 0 {
		return nil, errors.NotFoundEffectiveServer
	}

	var best *WeightNode

	for _, node := range w.nodes {
		node.curWeight += node.weight

		if best == nil || node.curWeight > best.curWeight {
			best = node
		}
	}

	best.curWeight -= w.totalWeight
	return best, nil
}

func (w *WeightedRR) Get() (*WeightNode, error) {
	return w.Next()
}
