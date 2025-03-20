package service

import (
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy"
	"codeRunner-siwu/internal/infrastructure/bananceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"log"
	"sync"
)

type ClientManagerDomain interface {
	AddClient(*entity.Client, int64)
	RemoveClient(string) error
	GetClientByBalance() (*entity.Client, error)
	GetClientById(id string) (*entity.Client, error)
}

type ClientManagerDomainTmpl struct {
	clients sync.Map
	bananceStrategy.LoadBalance
}

func NewClientManagerDomainTmpl(strategy bananceStrategy.LoadBalance) *ClientManagerDomainTmpl {
	return &ClientManagerDomainTmpl{
		clients:     sync.Map{},
		LoadBalance: strategy,
	}
}

func (s *ClientManagerDomainTmpl) AddClient(client *entity.Client, weight int64) {
	s.clients.Store(client.GetId(), client)
	s.LoadBalance.Add(weightedRRBalance.NewWeightNode(client.GetId(), weight))
}

func (s *ClientManagerDomainTmpl) RemoveClient(id string) error {
	client, err := s.GetClientById(id)
	if err != nil {
		return err
	}

	if err := client.Close(); err != nil {
		return err
	}

	s.clients.Delete(id)
	s.LoadBalance.Remove(id)
	return nil
}

func (s *ClientManagerDomainTmpl) GetClientById(id string) (*entity.Client, error) {
	client, ok := s.clients.Load(id)
	if !ok {
		return nil, errors.NotFoundEffectiveServer
	}
	return client.(*entity.Client), nil
}

func (s *ClientManagerDomainTmpl) GetClientByBalance() (*entity.Client, error) {
	node, err := s.LoadBalance.Get()
	if err != nil {
		log.Println("domain.server.service.GetClientByBalance() Get err=", err)
		return nil, err
	}

	client, ok := s.clients.Load(node.GetId())
	if !ok {
		return nil, errors.NotFoundEffectiveServer
	}
	return client.(*entity.Client), nil
}
