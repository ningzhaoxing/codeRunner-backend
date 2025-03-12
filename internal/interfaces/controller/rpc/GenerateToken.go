package rpc

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service"
	"context"
)

type TokenServer struct {
}

func (t *TokenServer) GenerateToken(ctx context.Context, request *proto.GenerateTokenRequest) (*proto.GenerateTokenResponse, error) {
	tokenIssuer := service.NewToken()
	response, err := tokenIssuer.GenerateToken(request)
	if err != nil {
		return response, err
	}
	return response, nil
}

func (t *TokenServer) mustEmbedUnimplementedTokenIssuerServer() {}
