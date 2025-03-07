package bananceStrategy

import "codeRunner-siwu/internal/infrastructure/common/errors"

type WeightNode struct {
	serverId  int   // 服务器id
	curWeight int64 // 当前权重
	weight    int64 // 初始权重
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

func (w *WeightedRR) Add(serverId int, weight int64) {
	w.nodes = append(w.nodes, &WeightNode{
		serverId:  serverId,
		curWeight: 0,
		weight:    weight,
	})
	w.totalWeight += weight
}

func (w *WeightedRR) Get() (int, error) {
	return w.Next()
}

func (w *WeightedRR) Remove(serverId int) {
	for i, node := range w.nodes {
		if node.serverId == serverId {
			w.nodes = append(w.nodes[:i], w.nodes[i+1:]...)
		}
	}
}

func (w *WeightedRR) UpdateWeight(serverId int, weight int64) {
	for _, node := range w.nodes {
		if node.serverId == serverId {
			// 平滑过渡：保留原有currentWeight的50%
			node.curWeight = node.curWeight * weight / node.weight
			node.weight = weight
			return
		}
	}
}

func (w *WeightedRR) Next() (int, error) {
	if len(w.nodes) == 0 {
		return -1, errors.NotFoundEffectiveServer
	}

	var best *WeightNode

	for _, node := range w.nodes {
		node.curWeight += node.weight

		if best == nil || node.curWeight > best.curWeight {
			best = node
		}
	}

	best.curWeight -= w.totalWeight
	return best.serverId, nil
}
