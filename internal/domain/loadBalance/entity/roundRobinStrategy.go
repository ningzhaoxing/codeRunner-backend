package entity

import "codeRunner-siwu/internal/infrastructure/bananceStrategy"

type SelectServerDomain interface {
	SelectServer() (int, error)
	AddServer(int, int64)
	RemoveServer(int)
	UpdateServerWeight(int, int64)
}

type SelectServerDomainImpl struct {
	bananceStrategy.LoadBalance
}

func NewSelectServerDomainImplWithRR(serverId int, weight int64) *SelectServerDomainImpl {
	weightedRR := bananceStrategy.NewWeightedRR()
	weightedRR.Add(serverId, weight)
	return &SelectServerDomainImpl{
		LoadBalance: weightedRR,
	}
}

func (s *SelectServerDomainImpl) SelectServer() (int, error) {
	return s.Get()
}

func (s *SelectServerDomainImpl) AddServer(serverId int, weight int64) {
	s.Add(serverId, weight)
}

func (s *SelectServerDomainImpl) RemoveServer(serverId int) {
	s.Remove(serverId)
}

func (s *SelectServerDomainImpl) UpdateServerWeight(serverId int, weight int64) {
	s.UpdateWeight(serverId, weight)
}
