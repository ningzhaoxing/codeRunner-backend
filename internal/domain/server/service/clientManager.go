package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/server/entity"
	"codeRunner-siwu/internal/infrastructure/common/errors"
	"sync"
	"time"

	"go.uber.org/zap"
)

// clientPending 存储某个客户端的在途请求（已发送未收到 ACK）
type clientPending struct {
	mu   sync.Mutex
	reqs map[string]*proto.ExecuteRequest // requestID → request
}

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
	clients     sync.Map // clientID → *entity.Client
	pendingReqs sync.Map // clientID → *clientPending
	LoadBalancer
}

func NewClientManagerDomainTmpl(strategy LoadBalancer) *ClientManagerDomainTmpl {
	return &ClientManagerDomainTmpl{
		LoadBalancer: strategy,
	}
}

func (s *ClientManagerDomainTmpl) AddClient(client *entity.Client, weight int64) {
	s.clients.Store(client.GetId(), client)
	s.pendingReqs.Store(client.GetId(), &clientPending{
		reqs: make(map[string]*proto.ExecuteRequest),
	})
	s.LoadBalancer.Add(client.GetId(), weight)
}

func (s *ClientManagerDomainTmpl) RemoveClient(id string) error {
	client, err := s.GetClientById(id)
	if err != nil {
		zap.S().Error("domain.server.service.RemoveClient() GetClientById err=", err)
		return err
	}

	if err := client.Close(); err != nil {
		zap.S().Error("domain.server.service.RemoveClient() Close err=", err)
		return err
	}

	s.clients.Delete(id)
	s.LoadBalancer.Remove(id)
	return nil
}

func (s *ClientManagerDomainTmpl) GetClientById(id string) (*entity.Client, error) {
	client, ok := s.clients.Load(id)
	if !ok {
		zap.S().Error("domain.server.service.GetClientById() Load err=", errors.NotFoundEffectiveServer)
		return nil, errors.NotFoundEffectiveServer
	}
	return client.(*entity.Client), nil
}

func (s *ClientManagerDomainTmpl) GetClientByBalance() (*entity.Client, error) {
	for i := 0; i < maxRetryPickClient; i++ {
		node, err := s.LoadBalancer.Get()
		if err != nil {
			zap.S().Error("domain.server.service.GetClientByBalance() Get err=", err)
			return nil, err
		}

		cli, ok := s.clients.Load(node.GetId())
		if !ok {
			s.LoadBalancer.Remove(node.GetId())
			continue
		}

		client := cli.(*entity.Client)
		if client.IsClosed() {
			zap.S().Warn("domain.server.service.GetClientByBalance() client closed, removing and retrying, id=", client.GetId())
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

// TrackRequest 记录已发送但未收到 ACK 的请求
func (s *ClientManagerDomainTmpl) TrackRequest(clientID, requestID string, req *proto.ExecuteRequest) {
	v, ok := s.pendingReqs.Load(clientID)
	if !ok {
		return
	}
	p := v.(*clientPending)
	p.mu.Lock()
	p.reqs[requestID] = req
	p.mu.Unlock()
}

// AcknowledgeRequest 收到 Client 的 ACK 后移除对应请求
func (s *ClientManagerDomainTmpl) AcknowledgeRequest(clientID, requestID string) {
	v, ok := s.pendingReqs.Load(clientID)
	if !ok {
		return
	}
	p := v.(*clientPending)
	p.mu.Lock()
	delete(p.reqs, requestID)
	p.mu.Unlock()
}

// DrainPending 取出并清空某客户端所有未 ACK 的请求，用于断线后重发
func (s *ClientManagerDomainTmpl) DrainPending(clientID string) []*proto.ExecuteRequest {
	v, ok := s.pendingReqs.LoadAndDelete(clientID)
	if !ok {
		return nil
	}
	p := v.(*clientPending)
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make([]*proto.ExecuteRequest, 0, len(p.reqs))
	for _, req := range p.reqs {
		result = append(result, req)
	}
	return result
}
