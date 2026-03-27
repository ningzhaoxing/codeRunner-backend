package p2cBalance

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	decayTime        = int64(time.Second * 10) // EWMA 衰减窗口
	forcePickTimeout = int64(time.Second)       // 强制选择超时，防饥饿
	initSuccess      = float64(1000)            // 初始成功率
	throttleSuccess  = float64(500)             // 健康阈值
	penalty          = int64(math.MaxInt32)     // 不健康节点惩罚值
)

// P2CNode 表示负载均衡中的一个节点，跟踪延迟、在途请求数和成功率。
type P2CNode struct {
	serverId string
	weight   int64

	// 以下字段通过 atomic 操作，lag/success 存储 math.Float64bits 编码的 float64
	lag      uint64 // EWMA 发送延迟（纳秒）
	inflight int64  // 当前在途请求数
	success  uint64 // EWMA 成功率 (0 ~ 1000)
	requests int64  // 总请求数（统计用）
	lastPick int64  // 上次被选中时间（UnixNano）

	mu    sync.Mutex
	stamp time.Time // EWMA 上次更新时间
}

func newP2CNode(serverId string, weight int64) *P2CNode {
	return &P2CNode{
		serverId: serverId,
		weight:   weight,
		success:  math.Float64bits(initSuccess),
		stamp:    time.Now(),
	}
}

// GetId 满足 service.BalanceNode 接口
func (n *P2CNode) GetId() string {
	return n.serverId
}

// load 计算节点负载：sqrt(ewma_lag + 1) * (inflight + 1) / weight
// 不健康节点直接返回 penalty
func (n *P2CNode) load() int64 {
	lag := math.Sqrt(math.Float64frombits(atomic.LoadUint64(&n.lag)) + 1)
	inflight := float64(atomic.LoadInt64(&n.inflight) + 1)
	succ := math.Float64frombits(atomic.LoadUint64(&n.success))

	if succ < throttleSuccess {
		return penalty
	}

	w := n.weight
	if w <= 0 {
		w = 1
	}

	return int64(lag * inflight / float64(w))
}
