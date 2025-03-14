package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/token"
	"log"
)

type tokenIssuer struct {
	token.TokenIssuer
}

func NewToken() *tokenIssuer {
	tokenPublic := token.NewToken([]byte("I'm si_wu"))
	return &tokenIssuer{tokenPublic}
}

func (t *tokenIssuer) GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	response, err = t.TokenIssuer.Public(request)
	if err != nil {
		log.Println("application.service.GenerateToken() Public err=", err)
		return response, err
	}
	return response, nil
}
