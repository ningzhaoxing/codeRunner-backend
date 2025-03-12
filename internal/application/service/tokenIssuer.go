package service

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/token"
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
		return response, err
	}
	return response, nil
}
