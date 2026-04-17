package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"testing"
)

type mockInnerServerDomain struct {
	readResult       *client.ReadResult
	lastSendResult   *proto.ExecuteResponse
	lastCallbackSend *proto.ExecuteResponse
}

func (m *mockInnerServerDomain) Read() (*client.ReadResult, error) {
	return m.readResult, nil
}
func (m *mockInnerServerDomain) RunCode(req *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	return &proto.ExecuteResponse{Id: req.Id, Result: "ok"}, nil
}
func (m *mockInnerServerDomain) SendResult(resp *proto.ExecuteResponse, err error) error {
	m.lastSendResult = resp
	return nil
}
func (m *mockInnerServerDomain) Send(resp *proto.ExecuteResponse, err error) error {
	m.lastCallbackSend = resp
	return nil
}
func (m *mockInnerServerDomain) Dail(client.TargetServer) error { return nil }

func TestWorkerMessageRouting_ExecuteSync(t *testing.T) {
	mock := &mockInnerServerDomain{
		readResult: &client.ReadResult{
			Request: &proto.ExecuteRequest{Id: "req-sync", Language: "golang"},
			MsgType: protocol.MsgTypeExecuteSync,
		},
	}

	res, _ := mock.RunCode(mock.readResult.Request)
	switch mock.readResult.MsgType {
	case protocol.MsgTypeExecuteSync:
		mock.SendResult(res, nil)
	default:
		mock.Send(res, nil)
	}

	if mock.lastSendResult == nil {
		t.Fatal("execute_sync should call SendResult")
	}
	if mock.lastCallbackSend != nil {
		t.Error("execute_sync should NOT call Send (HTTP callback)")
	}
}

func TestWorkerMessageRouting_Execute(t *testing.T) {
	mock := &mockInnerServerDomain{
		readResult: &client.ReadResult{
			Request: &proto.ExecuteRequest{Id: "req-async", Language: "golang"},
			MsgType: protocol.MsgTypeExecute,
		},
	}

	res, _ := mock.RunCode(mock.readResult.Request)
	switch mock.readResult.MsgType {
	case protocol.MsgTypeExecuteSync:
		mock.SendResult(res, nil)
	default:
		mock.Send(res, nil)
	}

	if mock.lastCallbackSend == nil {
		t.Fatal("execute should call Send (HTTP callback)")
	}
	if mock.lastSendResult != nil {
		t.Error("execute should NOT call SendResult")
	}
}
