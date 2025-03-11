package service

import (
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"sync"
)

type ClientManagerDomain interface {
	Add(*entity.Client, int64)
	Remove(string)
	GetServerByBalance() (*entity.Client, error)
}

type ClientManager struct {
	clients []*entity.Client
	rw      sync.RWMutex
	bananceStrategy.LoadBalance
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		clients:     make([]*entity.Client, 0),
		rw:          sync.RWMutex{},
		LoadBalance: weightedRRBalance.NewWeightedRR(),
	}
}

func (s *ClientManager) Add(client *entity.Client, weight int64) {
	s.rw.Lock()
	s.clients = append(s.clients, client)
	s.LoadBalance.Add(weightedRRBalance.NewWeightNode(client.GetId(), weight))
	s.rw.Unlock()
}

func (s *ClientManager) Remove(id string) {
	s.rw.Lock()
	for i, server := range s.clients {
		if id == server.GetId() {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			break
		}
	}
	s.LoadBalance.Remove(id)
	s.rw.Unlock()
}

// GetServerByBalance 通过负载均衡获取客户端
func (s *ClientManager) GetServerByBalance() (*entity.Client, error) {
	node, err := s.LoadBalance.Get()
	if err != nil {
		return nil, err
	}

	for _, server := range s.clients {
		if server.GetId() == node.GetId() {
			return server, nil
		}
	}
	return nil, errors.NotFoundEffectiveServer
}
