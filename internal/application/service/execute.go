package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/domain/loadBalance/entity"
)

type CodeRunner struct {
	entity.LoadBalanceDomain
}

func NewCodeRunner() *CodeRunner {
	return &CodeRunner{
		LoadBalanceDomain: entity.NewWeightedRR(),
	}
}

func (c *CodeRunner) Execute(in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	// 通过负载均衡算法选择并获取服务器的websocket连接
	domain := c.LoadBalanceDomain
	server, err := domain.GetServer()
	if err != nil {
		return nil, err
	}
	server.GetConn()

	// 发送数据包

	return nil, nil
}
