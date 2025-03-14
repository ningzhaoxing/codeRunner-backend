package service

import (
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"log"
	"sync"
)

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
	defer s.rw.Unlock()
	s.clients = append(s.clients, client)
	s.LoadBalance.Add(weightedRRBalance.NewWeightNode(client.GetId(), weight))
}

func (s *ClientManager) Remove(id string) error {
	s.rw.Lock()
	defer s.rw.Unlock()
	for i, server := range s.clients {
		if id == server.GetId() {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			if err := server.Close(); err != nil {
				log.Println("domain.server.service.clientManager Remove() Close err=", err)
				return err
			}
			break
		}
	}
	s.LoadBalance.Remove(id)
	return nil
}

// GetServerByBalance 通过负载均衡获取客户端
func (s *ClientManager) GetServerByBalance() (*entity.Client, error) {
	node, err := s.LoadBalance.Get()
	if err != nil {
		log.Println("domain.server.service.GetServerByBalance() Get err=", err)
		return nil, err
	}

	for _, server := range s.clients {
		if server.GetId() == node.GetId() {
			return server, nil
		}
	}
	log.Println("domain.server.service.GetServerByBalance() Get err=", errors.NotFoundEffectiveServer)
	return nil, errors.NotFoundEffectiveServer
}
