package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"codeRunner-siwu/internal/infrastructure/common/response/crResponse"
	"context"
)

type CodeRunner struct {
}

func (c *CodeRunner) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	codeRunner := service.NewCodeRunner()
	_, err := codeRunner.Execute(in)
	if err != nil {
		return crResponse.Fail(in.Id, err), err
	}
	return crResponse.Success(in.Id), nil
}
