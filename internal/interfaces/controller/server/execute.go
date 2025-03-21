package server

import (
	"codeRunner-siwu/api/proto"
	"context"
	"log"
)

func (ctl *EndpointCtl) Execute(ctx context.Context, in *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	err := ctl.Srv.Execute(in)
	if err != nil {
		log.Println("interfaces-controller-rpc-execute Execute的 s.Service.Execute err=", err)
		return nil, err
	}
	return nil, err
}
