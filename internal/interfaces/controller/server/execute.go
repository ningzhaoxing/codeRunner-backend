package server

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/tracing"
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const maxCodeBlockSize = 64 * 1024 // 64KB

func (ctl *EndpointCtl) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	log := tracing.Logger(ctx)

	if len(in.CodeBlock) > maxCodeBlockSize {
		return nil, status.Errorf(codes.InvalidArgument, "codeBlock exceeds max size limit (%d bytes)", maxCodeBlockSize)
	}

	err := ctl.Srv.Execute(ctx, in)
	if err != nil {
		log.Errorw("Execute failed", "requestID", in.Id, "err", err)
		return nil, err
	}

	log.Infow("Execute accepted", "requestID", in.Id)
	// 返回"已接受"语义：请求已提交，结果通过 callBackUrl 异步返回
	return &proto.ExecuteResponse{
		Id:       in.Id,
		GrpcCode: "accepted",
	}, nil
}
