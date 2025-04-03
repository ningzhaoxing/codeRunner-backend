package auth

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/common/token"
	"fmt"
	"github.com/sirupsen/logrus"
)

type Service interface {
	GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error)
}

type ServiceImpl struct {
	token.TokenIssuer
}

func NewService(token token.TokenIssuer) *ServiceImpl {
	return &ServiceImpl{token}
}

func (t *ServiceImpl) GenerateToken(request *proto.GenerateTokenRequest) (response *proto.GenerateTokenResponse, err error) {
	response, err = t.TokenIssuer.Public(request)
	if err != nil {
		logrus.Error(fmt.Sprintln("application.service.GenerateToken() Public err=\n", err))
		return response, err
	}
	return response, nil
}
