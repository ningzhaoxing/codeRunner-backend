package service

import (
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const maxRetryPickClient = 3

// BalanceNode 是负载均衡节点的抽象接口
type BalanceNode interface {
	GetId() string
}

// LoadBalancer 是负载均衡策略接口
type LoadBalancer interface {
	Add(id string, weight int64)
	Get() (BalanceNode, error)
	Remove(id string)
	Done(id string, duration time.Duration, err error)
}

type ClientManagerDomainTmpl struct {
	clients sync.Map
	LoadBalancer
}

func NewClientManagerDomainTmpl(strategy LoadBalancer) *ClientManagerDomainTmpl {
	return &ClientManagerDomainTmpl{
		clients:      sync.Map{},
		LoadBalancer: strategy,
	}
}

func (s *ClientManagerDomainTmpl) AddClient(client *entity.Client, weight int64) {
	s.clients.Store(client.GetId(), client)
	s.LoadBalancer.Add(client.GetId(), weight)
}

func (s *ClientManagerDomainTmpl) RemoveClient(id string) error {
	client, err := s.GetClientById(id)
	if err != nil {
		logrus.Error("domain.server.service.RemoveClient() GetClientById err=", err)
		return err
	}

	if err := client.Close(); err != nil {
		logrus.Error("domain.server.service.RemoveClient() Close err=", err)
		return err
	}

	s.clients.Delete(id)
	s.LoadBalancer.Remove(id)
	return nil
}

func (s *ClientManagerDomainTmpl) GetClientById(id string) (*entity.Client, error) {
	client, ok := s.clients.Load(id)
	if !ok {
		logrus.Error("domain.server.service.GetClientById() Load err=", errors.NotFoundEffectiveServer)
		return nil, errors.NotFoundEffectiveServer
	}
	return client.(*entity.Client), nil
}

func (s *ClientManagerDomainTmpl) GetClientByBalance() (*entity.Client, error) {
	for i := 0; i < maxRetryPickClient; i++ {
		node, err := s.LoadBalancer.Get()
		if err != nil {
			logrus.Error("domain.server.service.GetClientByBalance() Get err=", err)
			return nil, err
		}

		cli, ok := s.clients.Load(node.GetId())
		if !ok {
			s.LoadBalancer.Remove(node.GetId())
			continue
		}

		client := cli.(*entity.Client)
		if client.IsClosed() {
			logrus.Warn("domain.server.service.GetClientByBalance() client closed, removing and retrying, id=", client.GetId())
			_ = s.RemoveClient(client.GetId())
			continue
		}

		return client, nil
	}

	return nil, errors.NotFoundEffectiveServer
}

func (s *ClientManagerDomainTmpl) Done(id string, duration time.Duration, err error) {
	s.LoadBalancer.Done(id, duration, err)
}
