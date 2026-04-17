package protocol

import "testing"

func TestMsgTypeConstants(t *testing.T) {
	if MsgTypeExecuteSync != "execute_sync" {
		t.Errorf("MsgTypeExecuteSync = %q, want %q", MsgTypeExecuteSync, "execute_sync")
	}
	if MsgTypeResult != "result" {
		t.Errorf("MsgTypeResult = %q, want %q", MsgTypeResult, "result")
	}
	if MsgTypeExecute != "execute" {
		t.Errorf("MsgTypeExecute = %q, want %q", MsgTypeExecute, "execute")
	}
	if MsgTypeAck != "ack" {
		t.Errorf("MsgTypeAck = %q, want %q", MsgTypeAck, "ack")
	}
}
