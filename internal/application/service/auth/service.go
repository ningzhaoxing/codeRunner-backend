package auth

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/token"
	"log"
)

type Service struct {
	token.TokenIssuer
}

func NewService() *Service {
	tokenPublic := token.NewToken()
	return &Service{tokenPublic}
}

func (t *Service) GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	response, err = t.TokenIssuer.Public(request)
	if err != nil {
		log.Println("application.service.GenerateToken() Public err=", err)
		return response, err
	}
	return response, nil
}
