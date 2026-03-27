package weightedRRBalance

import (
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"sync"

	"github.com/sirupsen/logrus"
)

type WeightedRR struct {
	mu          sync.RWMutex
	nodes       []*WeightNode // 所有的服务实例
	totalWeight int64
}

func NewWeightedRR() *WeightedRR {
	return &WeightedRR{
		nodes:       make([]*WeightNode, 0),
		totalWeight: 0,
	}
}

func (w *WeightedRR) Add(rr *WeightNode) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.nodes = append(w.nodes, &WeightNode{
		serverId:  rr.serverId,
		curWeight: 0,
		weight:    rr.weight,
	})
	w.totalWeight += rr.weight
}

func (w *WeightedRR) Remove(serverId string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i, node := range w.nodes {
		if node.serverId == serverId {
			w.totalWeight -= node.weight
			w.nodes = append(w.nodes[:i], w.nodes[i+1:]...)
			return
		}
	}
}

func (w *WeightedRR) updateWeight(serverId string, weight int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, node := range w.nodes {
		if node.serverId == serverId {
			// 平滑过渡：保留原有currentWeight的50%
			w.totalWeight = w.totalWeight - node.weight + weight
			node.curWeight = node.curWeight * weight / node.weight
			node.weight = weight
			return
		}
	}
}

func (w *WeightedRR) Next() (*WeightNode, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.nodes) == 0 {
		logrus.Error("infrastructure-balanceStrategy-weightedRRBalance-weightedRR  Next() 的 err = %v ", errors.NotFoundEffectiveServer)
		return nil, errors.NotFoundEffectiveServer
	}

	var best *WeightNode

	for _, node := range w.nodes {
		node.curWeight += node.weight

		if best == nil || node.curWeight > best.curWeight {
			best = node
		}
	}

	if best != nil {
		best.curWeight -= w.totalWeight
	}

	return best, nil
}

func (w *WeightedRR) Get() (*WeightNode, error) {
	return w.Next()
}
