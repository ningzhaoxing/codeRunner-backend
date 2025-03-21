package auth

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/application/service/auth"
	"context"
	"log"
)

type TokenServer struct {
}

func (t *TokenServer) GenerateToken(ctx context.Context, request *proto.GenerateTokenRequest) (*proto.GenerateTokenResponse, error) {
	tokenIssuer := auth.NewService()
	response, err := tokenIssuer.GenerateToken(request)
	if err != nil {
		log.Println(" interfaces-controller-rpc-GenerateToken GenerateToken的tokenIssuer.GenerateToken err=", err)
		return response, err
	}
	return response, nil
}
