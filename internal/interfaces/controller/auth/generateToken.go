package auth

import (
	"codeRunner-siwu/api/proto"
	"context"
	"github.com/sirupsen/logrus"
)

func (t *EndpointCtl) GenerateToken(ctx context.Context, request *proto.GenerateTokenRequest) (*proto.GenerateTokenResponse, error) {
	response, err := t.Srv.GenerateToken(request)
	if err != nil {
		logrus.Error(" interfaces-controller-rpc-GenerateToken GenerateToken的tokenIssuer.GenerateToken err=", err)
		return response, err
	}
	return response, nil
}
