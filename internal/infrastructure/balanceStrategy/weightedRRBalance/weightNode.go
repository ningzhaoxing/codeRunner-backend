package weightedRRBalance

type WeightNode struct {
	serverId  string // 服务器id
	curWeight int64  // 当前权重
	weight    int64  // 初始权重
}

func NewWeightNode(serverId string, weight int64) *WeightNode {
	return &WeightNode{
		serverId: serverId,
		weight:   weight,
	}
}

func (w *WeightNode) GetId() string {
	return w.serverId
}
