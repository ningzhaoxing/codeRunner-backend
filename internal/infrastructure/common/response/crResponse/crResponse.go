package crResponse

import "codeRunner-siwu/api/proto"

func Success(id string) *proto.ExecuteResponse {
	return &proto.ExecuteResponse{
		Id:       id,
		GrpcCode: "200",
	}
}

func Fail(id string, err error) *proto.ExecuteResponse {
	return &proto.ExecuteResponse{
		Id:       id,
		GrpcCode: "500",
		Result:   err.Error(),
	}
}
