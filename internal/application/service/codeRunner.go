package service

import (
	"codeRunner-siwu/api/proto"
	"context"
)

type CodeRunner struct {
}

func (c *CodeRunner) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	// 通过负载均衡算法选择服务器

	// 获取已选择的服务器conn

	// 发送数据包

	return nil, nil
}
