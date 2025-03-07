package service

import (
	"codeRunner-siwu/api/proto"
	"context"
)

type CodeRunner struct {
}

func (c *CodeRunner) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	return nil, nil
}
