package service

import (
	"codeRunner-siwu/internal/domain/server/entity/serverManage"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"sync"
)

type ServerManagerDomain interface {
	Add(*serverManage.InnerServer, int64)
	Remove(string)
	GetServerByBalance() (*serverManage.InnerServer, error)
}

type ServerManager struct {
	servers []*serverManage.InnerServer
	rw      sync.RWMutex
	bananceStrategy.LoadBalance
}

func NewServerManager() *ServerManager {
	return &ServerManager{
		servers:     make([]*serverManage.InnerServer, 0),
		rw:          sync.RWMutex{},
		LoadBalance: weightedRRBalance.NewWeightedRR(),
	}
}

func (s *ServerManager) Add(server *serverManage.InnerServer, weight int64) {
	s.rw.Lock()
	s.servers = append(s.servers, server)
	s.LoadBalance.Add(weightedRRBalance.NewWeightNode(server.GetId(), weight))
	s.rw.Unlock()
}

func (s *ServerManager) Remove(id string) {
	s.rw.Lock()
	for i, server := range s.servers {
		if id == server.GetId() {
			s.servers = append(s.servers[:i], s.servers[i+1:]...)
			break
		}
	}
	s.LoadBalance.Remove(id)
	s.rw.Unlock()
}

// GetServerByBalance 通过负载均衡获取服务器
func (s *ServerManager) GetServerByBalance() (*serverManage.InnerServer, error) {
	node, err := s.LoadBalance.Get()
	if err != nil {
		return nil, err
	}

	for _, server := range s.servers {
		if server.GetId() == node.GetId() {
			return server, nil
		}
	}
	return nil, errors.NotFoundEffectiveServer
}
