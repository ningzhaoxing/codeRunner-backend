package p2cBalance

import (
	"codeRunner-siwu/internal/domain/server/service"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type P2CBalancer struct {
	mu    sync.Mutex
	nodes []*P2CNode
	r     *rand.Rand
}

func NewP2CBalancer() *P2CBalancer {
	return &P2CBalancer{
		nodes: make([]*P2CNode, 0),
		r:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *P2CBalancer) Add(id string, weight int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, n := range b.nodes {
		if n.serverId == id {
			return
		}
	}

	b.nodes = append(b.nodes, newP2CNode(id, weight))
}

func (b *P2CBalancer) Remove(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, n := range b.nodes {
		if n.serverId == id {
			b.nodes = append(b.nodes[:i], b.nodes[i+1:]...)
			return
		}
	}
}

func (b *P2CBalancer) Get() (service.BalanceNode, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	n := len(b.nodes)
	switch {
	case n == 0:
		return nil, errors.NotFoundEffectiveServer
	case n == 1:
		b.pick(b.nodes[0])
		return b.nodes[0], nil
	default:
		a := b.r.Intn(n)
		bb := b.r.Intn(n - 1)
		if bb >= a {
			bb++
		}

		chosen := b.choose(b.nodes[a], b.nodes[bb])
		b.pick(chosen)
		return chosen, nil
	}
}

// Done 在请求完成后调用，更新节点的 EWMA 延迟、成功率，并递减在途请求数。
func (b *P2CBalancer) Done(id string, duration time.Duration, err error) {
	b.mu.Lock()
	node := b.findNode(id)
	b.mu.Unlock()

	if node == nil {
		return
	}

	node.mu.Lock()
	now := time.Now()
	elapsed := now.Sub(node.stamp)
	node.stamp = now
	node.mu.Unlock()

	w := math.Exp(-float64(elapsed) / float64(decayTime))

	oldLag := math.Float64frombits(atomic.LoadUint64(&node.lag))
	newLag := oldLag*w + float64(duration.Nanoseconds())*(1-w)
	atomic.StoreUint64(&node.lag, math.Float64bits(newLag))

	successVal := initSuccess
	if err != nil {
		successVal = 0
	}
	oldSuccess := math.Float64frombits(atomic.LoadUint64(&node.success))
	newSuccess := oldSuccess*w + successVal*(1-w)
	atomic.StoreUint64(&node.success, math.Float64bits(newSuccess))

	atomic.AddInt64(&node.inflight, -1)
}

// choose 在两个节点中选 load 更低的；若高 load 节点长时间未被选中则强制选它
func (b *P2CBalancer) choose(c1, c2 *P2CNode) *P2CNode {
	load1, load2 := c1.load(), c2.load()

	if load1 > load2 {
		c1, c2 = c2, c1
	}

	pick := atomic.LoadInt64(&c2.lastPick)
	if time.Now().UnixNano()-pick > forcePickTimeout {
		if atomic.CompareAndSwapInt64(&c2.lastPick, pick, time.Now().UnixNano()) {
			return c2
		}
	}

	return c1
}

func (b *P2CBalancer) pick(node *P2CNode) {
	atomic.AddInt64(&node.inflight, 1)
	atomic.AddInt64(&node.requests, 1)
	atomic.StoreInt64(&node.lastPick, time.Now().UnixNano())
}

func (b *P2CBalancer) findNode(id string) *P2CNode {
	for _, n := range b.nodes {
		if n.serverId == id {
			return n
		}
	}
	return nil
}
