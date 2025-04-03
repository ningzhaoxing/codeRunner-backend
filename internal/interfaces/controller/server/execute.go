package server

import (
	"codeRunner-siwu/api/proto"
	"context"
	"github.com/sirupsen/logrus"
)

func (ctl *EndpointCtl) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	err := ctl.Srv.Execute(in)
	if err != nil {
		logrus.Error("interfaces-controller-rpc-execute Execute的 s.Service.Execute err=", err)
		return nil, err
	}
	return nil, err
}
