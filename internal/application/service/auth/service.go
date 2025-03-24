package auth

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/common/token"
	"fmt"
)

type Service interface {
	GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error)
}

type ServiceImpl struct {
	token.TokenIssuer
	logger.Logger
}

func NewService(token token.TokenIssuer, log logger.Logger) *ServiceImpl {
	return &ServiceImpl{token, log}
}

func (t *ServiceImpl) GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	response, err = t.TokenIssuer.Public(request)
	if err != nil {
		t.Logger.Error(fmt.Sprintln("application.service.GenerateToken() Public err=\n", err))
		return response, err
	}
	return response, nil
}
