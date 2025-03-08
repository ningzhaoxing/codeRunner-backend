package entity

import (
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"sync"
)

type LoadBalanceDomain interface {
	GetServer() (*weightedRRBalance.WeightNode, error)
	AddServer(server *weightedRRBalance.WeightNode)
	RemoveServer(string)
	UpdateServerWeight(string, int64)
}

type WeightedRR struct {
	bananceStrategy.LoadBalance
	rw sync.RWMutex // 互斥锁，这时因为负载均衡维护了一个全局的 server 列表。
}

func NewWeightedRR() *WeightedRR {
	weightedRR := weightedRRBalance.NewWeightedRR()
	return &WeightedRR{
		LoadBalance: weightedRR,
		rw:          sync.RWMutex{},
	}
}

func (s *WeightedRR) GetServer() (*weightedRRBalance.WeightNode, error) {
	s.rw.Lock()
	defer s.rw.Unlock()
	return s.Get()
}

func (s *WeightedRR) AddServer(server *weightedRRBalance.WeightNode) {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.Add(server)
}

func (s *WeightedRR) RemoveServer(serverId string) {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.Remove(serverId)
}

func (s *WeightedRR) UpdateServerWeight(serverId string, weight int64) {
	s.rw.Lock()
	defer s.rw.Unlock()
	s.UpdateWeight(serverId, weight)
}
