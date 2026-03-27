package server

import (
	"codeRunner-siwu/api/proto"
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxCodeBlockSize = 64 * 1024 // 64KB

func (ctl *EndpointCtl) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	if len(in.CodeBlock) > maxCodeBlockSize {
		return nil, status.Errorf(codes.InvalidArgument, "codeBlock exceeds max size limit (%d bytes)", maxCodeBlockSize)
	}

	err := ctl.Srv.Execute(in)
	if err != nil {
		zap.S().Error("interfaces-controller-rpc-execute Execute的 s.Service.Execute err=", err)
		return nil, err
	}

	// 返回"已接受"语义：请求已提交，结果通过 callBackUrl 异步返回
	return &proto.ExecuteResponse{
		Id:       in.Id,
		GrpcCode: "accepted",
	}, nil
}
