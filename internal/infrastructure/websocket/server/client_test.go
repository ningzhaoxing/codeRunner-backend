package server

import (
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupTestWSPair(t *testing.T) (*WebsocketClientImpl, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		connCh <- c
	}))

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), nil)
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	serverConn := <-connCh
	s.Close()

	impl := NewWebsocketClientImpl(serverConn)
	return impl, clientConn
}

func TestSendSync_Success(t *testing.T) {
	impl, workerConn := setupTestWSPair(t)
	defer workerConn.Close()

	// Start Read loop to route result messages to pendingSync channel
	go func() {
		for {
			if _, err := impl.Read(); err != nil {
				return
			}
		}
	}()

	requestID := "req-123"
	payload := []byte(`{"id":"req-123","language":"golang","codeBlock":"fmt.Println(1)"}`)
	resultPayload := []byte(`{"id":"req-123","result":"1\n","err":""}`)

	go func() {
		_, data, _ := workerConn.ReadMessage()
		var msg protocol.WsMessage
		json.Unmarshal(data, &msg)
		if msg.Type != protocol.MsgTypeExecuteSync {
			t.Errorf("Worker received type=%q, want execute_sync", msg.Type)
		}
		reply, _ := json.Marshal(protocol.WsMessage{
			Type:      protocol.MsgTypeResult,
			RequestID: requestID,
			Payload:   resultPayload,
		})
		workerConn.WriteMessage(websocket.TextMessage, reply)
	}()

	result, err := impl.SendSync(requestID, payload, 5*time.Second)
	if err != nil {
		t.Fatalf("SendSync error: %v", err)
	}
	if string(result) != string(resultPayload) {
		t.Errorf("result = %s, want %s", result, resultPayload)
	}
}

func TestSendSync_Timeout(t *testing.T) {
	impl, workerConn := setupTestWSPair(t)
	defer workerConn.Close()

	go func() {
		for {
			if _, err := impl.Read(); err != nil {
				return
			}
		}
	}()

	_, err := impl.SendSync("req-timeout", []byte(`{}`), 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
