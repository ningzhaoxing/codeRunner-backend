package service

import (
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/balanceStrategy"
	"codeRunner-siwu/internal/infrastructure/balanceStrategy/weightedRRBalance"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	errors2 "errors"
	"fmt"
	"github.com/sirupsen/logrus"
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
	balanceStrategy.LoadBalance
}

func NewClientManagerDomainTmpl(strategy balanceStrategy.LoadBalance) *ClientManagerDomainTmpl {
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
		logrus.Error("domain.server.service.RemoveClient() GetClientById err=", err)
		return err
	}

	if err := client.Close(); err != nil {
		logrus.Error("domain.server.service.RemoveClient() Close err=", err)
		return err
	}

	s.clients.Delete(id)
	s.LoadBalance.Remove(id)
	return nil
}

func (s *ClientManagerDomainTmpl) GetClientById(id string) (*entity.Client, error) {
	client, ok := s.clients.Load(id)
	if !ok {
		logrus.Error("domain.server.service.RemoveClient() Load err=", errors.NotFoundEffectiveServer)
		return nil, errors.NotFoundEffectiveServer
	}
	return client.(*entity.Client), nil
}

func (s *ClientManagerDomainTmpl) GetClientByBalance() (*entity.Client, error) {
	node, err := s.LoadBalance.Get()
	if err != nil {
		logrus.Error("domain.server.service.GetClientByBalance() Get err=", err)
		return nil, err
	}

	cli, ok := s.clients.Load(node.GetId())
	if !ok {
		logrus.Error("domain.server.service.Load() Get err=", errors.NotFoundEffectiveServer)
		return nil, errors.NotFoundEffectiveServer
	}

	// 判断当前客户端是否已被关闭，如果已被关闭，则需要删除该客户端
	client := cli.(*entity.Client)
	if client.IsClosed() {
		err := s.RemoveClient(client.GetId())
		if err != nil {
			fmt.Println("客户端被关闭，已被移除")
			logrus.Error("domain.server.service.Load() Get err=", errors2.New("客户端被关闭，已被移除"))
			return nil, err
		}
	}

	return client, nil
}
