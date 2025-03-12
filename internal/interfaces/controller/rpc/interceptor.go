package rpc

import (
	token2 "codeRunner-siwu/internal/infrastructure/token"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if info.FullMethod == "/codeRunner.v1.tokenIssuer/GenerateToken" {
			return handler(ctx, req)
		}
		// 从上下文中获取元数据
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "missing metadata")
		}

		// 从元数据中获取 token
		token, ok := md["token"]
		if !ok || len(token) == 0 {
			return nil, status.Error(codes.Unauthenticated, "token not found")
		}

		// 调用基础设施层的 TokenManager 验证 token
		tokenManager := token2.NewToken([]byte("I'm si_wu"))
		valid, err := tokenManager.Verify(token[0])
		if err != nil || !valid {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}
		// 如果 token 有效，继续处理请求
		return handler(ctx, req)
	}
}
