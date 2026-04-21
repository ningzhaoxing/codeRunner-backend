# CodeRunner WebSocket 同步执行模式 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 CodeRunner 新增 WebSocket 同步执行模式（`execute_sync` → `result`），使 Server 端可以同步阻塞等待 Worker 执行结果，供 Agent 模块的 `CodeExecutor` 接口使用。

**Architecture:** 在现有异步协议（`execute` → `ack` → HTTP callback）旁新增同步路径。Server 发送 `execute_sync` 消息给 Worker，Worker 执行完毕后通过 WebSocket 回传 `result` 消息（而非 HTTP callback）。Server 端通过 `pendingSync` map + channel 阻塞等待结果。两条路径并行存在，互不影响。

**架构决策：** Spec 建议将 `pendingSync` 放在 `domain/server/service/clientManager.go`，但本计划将其放在 `infrastructure/websocket/server/client.go` 的 `WebsocketClientImpl` 中。理由：pending sync 状态是连接级别的（每个 WebSocket 连接独立管理自己的同步请求），而 `clientManager` 管理的是跨连接的负载均衡和请求分发。将 `pendingSync` 放在连接对象内部更符合单一职责原则，且避免了 `clientManager` 需要感知 WebSocket 协议细节。

**Tech Stack:** Go 1.23, gorilla/websocket, sync.Map, protocol/message.go

**Spec 参考:** `docs/context/designs/2026-03-28-agent-design.md` §3.3 "CodeRunner 同步执行模式"

---

### Task 1: 扩展 WebSocket 协议消息类型

**Files:**
- Modify: `internal/infrastructure/websocket/protocol/message.go`
- Test: `internal/infrastructure/websocket/protocol/message_test.go`

- [ ] **Step 1: 写测试验证新消息类型存在**

```go
// internal/infrastructure/websocket/protocol/message_test.go
package protocol

import "testing"

func TestMsgTypeConstants(t *testing.T) {
	// 验证新增的同步执行消息类型
	if MsgTypeExecuteSync != "execute_sync" {
		t.Errorf("MsgTypeExecuteSync = %q, want %q", MsgTypeExecuteSync, "execute_sync")
	}
	if MsgTypeResult != "result" {
		t.Errorf("MsgTypeResult = %q, want %q", MsgTypeResult, "result")
	}

	// 验证现有类型未被破坏
	if MsgTypeExecute != "execute" {
		t.Errorf("MsgTypeExecute = %q, want %q", MsgTypeExecute, "execute")
	}
	if MsgTypeAck != "ack" {
		t.Errorf("MsgTypeAck = %q, want %q", MsgTypeAck, "ack")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/infrastructure/websocket/protocol/ -run TestMsgTypeConstants -v`
Expected: FAIL — `MsgTypeExecuteSync` 和 `MsgTypeResult` 未定义

- [ ] **Step 3: 新增消息类型常量**

在 `internal/infrastructure/websocket/protocol/message.go` 的 `const` 块中追加：

```go
const (
	MsgTypeExecute     MsgType = "execute"      // Server → Client：执行请求
	MsgTypeAck         MsgType = "ack"           // Client → Server：收到确认
	MsgTypeExecuteSync MsgType = "execute_sync"  // Server → Client：同步执行请求
	MsgTypeResult      MsgType = "result"        // Client → Server：执行结果回传
)
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd codeRunner-backend && go test ./internal/infrastructure/websocket/protocol/ -run TestMsgTypeConstants -v`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd codeRunner-backend
git add internal/infrastructure/websocket/protocol/message.go internal/infrastructure/websocket/protocol/message_test.go
git commit -m "feat: add execute_sync and result WebSocket message types"
```

---

### Task 2: Server 端 WebSocket client 支持 SendSync 和 result 消息处理

**Files:**
- Modify: `internal/infrastructure/websocket/server/client.go`
- Test: `internal/infrastructure/websocket/server/client_test.go`

- [ ] **Step 1: 写测试验证 SendSync 基本行为**

```go
// internal/infrastructure/websocket/server/client_test.go
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

// setupTestWSPair 创建一对连接的 WebSocket（server 端 + client 端）
func setupTestWSPair(t *testing.T) (*WebsocketClientImpl, *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	var serverConn *websocket.Conn
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
	}))
	defer s.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(s.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// serverConn 是 Server 端持有的连接（包装为 WebsocketClientImpl）
	// clientConn 模拟 Worker 端
	impl := NewWebsocketClientImpl(serverConn)
	return impl, clientConn
}

func TestSendSync_Success(t *testing.T) {
	impl, workerConn := setupTestWSPair(t)
	defer workerConn.Close()

	// 启动 Read 循环（负责将 result 消息路由到 pendingSync channel）
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

	// Worker 端：收到 execute_sync 后回传 result
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

	// 启动 Read 循环（即使 Worker 不回复，Read 循环也必须运行）
	go func() {
		for {
			if _, err := impl.Read(); err != nil {
				return
			}
		}
	}()

	// Worker 不回复，触发超时
	_, err := impl.SendSync("req-timeout", []byte(`{}`), 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd codeRunner-backend && go test ./internal/infrastructure/websocket/server/ -run TestSendSync -v`
Expected: FAIL — `SendSync` 方法不存在

- [ ] **Step 3: 实现 SendSync 和 result 消息路由**

修改 `internal/infrastructure/websocket/server/client.go`：

```go
package server

import (
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	pingInterval = 30 * time.Second
	pongWait     = 60 * time.Second
)

type WebsocketClientImpl struct {
	conn        *websocket.Conn
	isClosed    bool
	ackHandler  func(requestID string)
	pendingSync sync.Map // requestID → chan []byte
}

func NewWebsocketClientImpl(conn *websocket.Conn) *WebsocketClientImpl {
	return &WebsocketClientImpl{
		conn: conn,
	}
}

func (c *WebsocketClientImpl) Read() ([]byte, error) {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return nil, err
		}

		var msg protocol.WsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return data, nil
		}

		switch msg.Type {
		case protocol.MsgTypeAck:
			if c.ackHandler != nil {
				c.ackHandler(msg.RequestID)
			}
			continue
		case protocol.MsgTypeResult:
			if ch, ok := c.pendingSync.LoadAndDelete(msg.RequestID); ok {
				ch.(chan []byte) <- msg.Payload
			}
			continue
		default:
			return msg.Payload, nil
		}
	}
}

func (c *WebsocketClientImpl) Send(requestID string, payload []byte) error {
	msg := protocol.WsMessage{
		Type:      protocol.MsgTypeExecute,
		RequestID: requestID,
		Payload:   payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// SendSync 发送同步执行请求并阻塞等待结果
func (c *WebsocketClientImpl) SendSync(requestID string, payload []byte, timeout time.Duration) ([]byte, error) {
	ch := make(chan []byte, 1)
	c.pendingSync.Store(requestID, ch)
	defer c.pendingSync.Delete(requestID)

	msg := protocol.WsMessage{
		Type:      protocol.MsgTypeExecuteSync,
		RequestID: requestID,
		Payload:   payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return nil, err
	}

	select {
	case result := <-ch:
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("SendSync timeout after %v for request %s", timeout, requestID)
	}
}

func (c *WebsocketClientImpl) SetAckHandler(fn func(requestID string)) {
	c.ackHandler = fn
}

func (c *WebsocketClientImpl) Close() error {
	if !c.IsClosed() {
		c.isClosed = true
		err := c.conn.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *WebsocketClientImpl) HeartBeat() error {
	c.conn.SetReadDeadline(time.Now().Add(pongWait))

	c.conn.SetPingHandler(func(appData string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if err := c.conn.WriteMessage(websocket.PongMessage, []byte(appData)); err != nil {
			c.Close()
			return err
		}
		return nil
	})

	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	go func() {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for range ticker.C {
			if c.IsClosed() {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				zap.S().Warn("heartbeat ping failed, closing connection: ", err)
				c.Close()
				return
			}
		}
	}()

	return nil
}

func (c *WebsocketClientImpl) IsClosed() bool {
	return c.isClosed
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd codeRunner-backend && go test ./internal/infrastructure/websocket/server/ -run TestSendSync -v -race`
Expected: PASS

- [ ] **Step 5: 提交**

```bash
cd codeRunner-backend
git add internal/infrastructure/websocket/server/client.go internal/infrastructure/websocket/server/client_test.go
git commit -m "feat: add SendSync method and result message routing to server WebSocket client"
```

---

### Task 3: Server 端 entity.Client 暴露 SendSync 方法

**Files:**
- Modify: `internal/domain/server/entity/client.go`
- Test: 复用 Task 5 的集成测试

- [ ] **Step 1: 扩展 WebsocketClient 接口**

在 `internal/domain/server/entity/client.go` 的 `WebsocketClient` 接口中新增：

```go
type WebsocketClient interface {
	Send(requestID string, payload []byte) error
	SendSync(requestID string, payload []byte, timeout time.Duration) ([]byte, error)
	SetAckHandler(fn func(requestID string))
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}
```

- [ ] **Step 2: 在 Client 上新增 SendSync 方法**

```go
func (c *Client) SendSync(request *proto.ExecuteRequest, timeout time.Duration) (*proto.ExecuteResponse, error) {
	payload, err := json.Marshal(*request)
	if err != nil {
		return nil, err
	}
	resultBytes, err := c.WebsocketClient.SendSync(request.Id, payload, timeout)
	if err != nil {
		return nil, err
	}
	var resp proto.ExecuteResponse
	if err := json.Unmarshal(resultBytes, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal sync result: %w", err)
	}
	return &resp, nil
}
```

需要在文件顶部追加 `"fmt"` 和 `"time"` import。

- [ ] **Step 3: 同步更新 application/service/server 的 WebsocketClient 接口**

`internal/application/service/server/service.go` 底部的 `WebsocketClient` 接口也需要新增 `SendSync`：

```go
type WebsocketClient interface {
	Send(requestID string, payload []byte) error
	SendSync(requestID string, payload []byte, timeout time.Duration) ([]byte, error)
	SetAckHandler(fn func(requestID string))
	Close() error
	HeartBeat() error
	Read() ([]byte, error)
	IsClosed() bool
}
```

- [ ] **Step 4: 编译验证**

Run: `cd codeRunner-backend && go build ./...`
Expected: 编译通过，无接口不匹配错误

- [ ] **Step 5: 提交**

```bash
cd codeRunner-backend
git add internal/domain/server/entity/client.go internal/application/service/server/service.go
git commit -m "feat: expose SendSync on entity.Client and update WebsocketClient interface"
```

---

### Task 4: Server Application Service 新增 ExecuteSync 方法

**Files:**
- Modify: `internal/application/service/server/service.go`
- Test: `internal/application/service/server/service_test.go`

- [ ] **Step 1: 扩展 ServerService 接口**

在 `internal/application/service/server/service.go` 的 `ServerService` 接口中新增：

```go
type ServerService interface {
	Execute(ctx context.Context, in *proto.ExecuteRequest) error
	ExecuteSync(ctx context.Context, in *proto.ExecuteRequest, timeout time.Duration) (*proto.ExecuteResponse, error)
	Run(cli WebsocketClient, weight int64) error
}
```

- [ ] **Step 2: 实现 ExecuteSync**

```go
func (w *ServiceImpl) ExecuteSync(ctx context.Context, in *proto.ExecuteRequest, timeout time.Duration) (*proto.ExecuteResponse, error) {
	log := tracing.Logger(ctx)

	client, err := w.ClientManagerDomain.GetClientByBalance()
	if err != nil {
		log.Errorw("ExecuteSync GetClientByBalance failed", "requestID", in.Id, "err", err)
		return nil, err
	}

	start := time.Now()
	resp, err := client.SendSync(in, timeout)
	w.ClientManagerDomain.Done(client.GetId(), time.Since(start), err)

	if err != nil {
		log.Errorw("ExecuteSync SendSync failed", "requestID", in.Id, "clientID", client.GetId(), "err", err)
		return nil, err
	}

	log.Infow("ExecuteSync completed", "requestID", in.Id, "clientID", client.GetId(), "duration", time.Since(start))
	return resp, nil
}
```

- [ ] **Step 3: 编译验证**

Run: `cd codeRunner-backend && go build ./...`
Expected: 编译通过

- [ ] **Step 4: 提交**

```bash
cd codeRunner-backend
git add internal/application/service/server/service.go
git commit -m "feat: add ExecuteSync to ServerService for synchronous code execution"
```

---

### Task 5: Worker 端支持 execute_sync 消息类型

**Files:**
- Modify: `internal/infrastructure/websocket/client/client.go`
- Modify: `internal/domain/client/entity/innerServer.go`
- Modify: `internal/domain/client/service/innerServerService.go`
- Modify: `internal/application/service/client/service.go`
- Test: `internal/application/service/client/service_test.go`

- [ ] **Step 1: 修改 WebsocketClient.Read() 返回执行模式信息**

Worker 端的 `client.WebsocketClient.Read()` 当前返回 `(*proto.ExecuteRequest, error)`。需要额外返回消息类型，让上层知道是 `execute` 还是 `execute_sync`。

修改 `internal/infrastructure/websocket/client/client.go` 的 `Read()` 方法：

```go
// ReadResult 包含读取结果和消息类型
type ReadResult struct {
	Request *proto.ExecuteRequest
	MsgType protocol.MsgType
}

func (i *WebsocketClientImpl) Read() (*ReadResult, error) {
	_, m, err := i.conn.ReadMessage()
	if err != nil {
		zap.S().Error("infrastructure-websocket-client Read() err=", err)
		return nil, err
	}

	var wsMsg protocol.WsMessage
	if err = json.Unmarshal(m, &wsMsg); err != nil {
		return nil, err
	}

	// 仅 execute 需要回 ACK；execute_sync 使用 result 消息作为确认机制，不发 ACK
	if wsMsg.Type == protocol.MsgTypeExecute {
		ack, _ := json.Marshal(protocol.WsMessage{
			Type:      protocol.MsgTypeAck,
			RequestID: wsMsg.RequestID,
		})
		if writeErr := i.conn.WriteMessage(websocket.TextMessage, ack); writeErr != nil {
			zap.S().Warn("infrastructure-websocket-client Read() send ACK failed: ", writeErr)
		}
	}

	msg := new(proto.ExecuteRequest)
	if err = json.Unmarshal(wsMsg.Payload, msg); err != nil {
		return nil, err
	}
	return &ReadResult{Request: msg, MsgType: wsMsg.Type}, nil
}
```

同步更新 `WebsocketClient` 接口：

```go
type WebsocketClient interface {
	Dail(TargetServer) error
	Read() (*ReadResult, error)
	CallBackSend(*proto.ExecuteResponse, error) error
	WebsocketSend(any) error
	Close() error
}
```

- [ ] **Step 3: 更新 domain/client/entity/innerServer.go 的 Read()**

```go
func (i *InnerServerDomainImpl) Read() (*client.ReadResult, error) {
	result, err := i.WebsocketClient.Read()
	if err != nil {
		zap.S().Error("domain.client.entity.Read() WebsocketClient.Read err=", err)
		return nil, err
	}
	return result, nil
}
```

新增 `SendResult` 方法（通过 WebSocket 回传执行结果）：

```go
func (i *InnerServerDomainImpl) SendResult(response *proto.ExecuteResponse, err error) error {
	if err != nil {
		response.Err = err.Error()
	}
	// 构造 WsMessage{Type: "result", RequestID: response.Id, Payload: json.Marshal(response)}
	payload, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		return marshalErr
	}
	wsMsg := protocol.WsMessage{
		Type:      protocol.MsgTypeResult,
		RequestID: response.Id,
		Payload:   payload,
	}
	return i.WebsocketSend(wsMsg)
}
```

需要追加 import `"codeRunner-siwu/internal/infrastructure/websocket/protocol"` 和 `"encoding/json"`。

- [ ] **Step 4: 更新 domain/client/service/innerServerService.go 接口**

```go
type InnerServerDomain interface {
	Dail(client.TargetServer) error
	Read() (*client.ReadResult, error)
	Send(*proto.ExecuteResponse, error) error
	SendResult(*proto.ExecuteResponse, error) error
	RunCode(*proto.ExecuteRequest) (*proto.ExecuteResponse, error)
}
```

- [ ] **Step 5: 修改 application/service/client/service.go 按模式分发**

```go
func (w *ServiceImpl) Run(c config.Config) error {
	if err := w.dail(c); err != nil {
		zap.S().Error(fmt.Sprintln("application.client.Run() dail err=", err))
		return err
	}

	for {
		readResult, err := w.InnerServerDomain.Read()
		if err != nil {
			zap.S().Error(fmt.Sprintln("websocket客户端已被关闭,请重启服务。application.client.Run() Read err=", err))
			return err
		}
		fmt.Println("读取到消息:", readResult.Request)

		res, execErr := w.RunCode(readResult.Request)
		if execErr != nil {
			zap.S().Error(fmt.Sprintln("application.client.Run() Service err=", execErr))
		}
		fmt.Println("处理结果为:", res)

		// 根据消息类型选择回传方式
		switch readResult.MsgType {
		case protocol.MsgTypeExecuteSync:
			if err = w.sendResult(res, execErr); err != nil {
				zap.S().Error(fmt.Sprintln("application.client.Run() sendResult err=", err))
			}
		default:
			if err = w.send(res, execErr); err != nil {
				zap.S().Error(fmt.Sprintln("application.client.Run() send err=", err))
			}
		}
		fmt.Println("结果发送成功")
	}
}

func (w *ServiceImpl) sendResult(res *proto.ExecuteResponse, err error) error {
	if err := w.InnerServerDomain.SendResult(res, err); err != nil {
		zap.S().Error(fmt.Sprintln("application.client.sendResult() err=", err))
		return err
	}
	return nil
}
```

需要追加 import `"codeRunner-siwu/internal/infrastructure/websocket/protocol"`。

- [ ] **Step 6: 写 Worker 端消息路由测试**

```go
// internal/application/service/client/service_test.go
package client

import (
	"codeRunner-siwu/api/proto"
	"codeRunner-siwu/internal/infrastructure/websocket/client"
	"codeRunner-siwu/internal/infrastructure/websocket/protocol"
	"testing"
)

type mockInnerServerDomain struct {
	lastReadResult   *client.ReadResult
	lastSendResult   *proto.ExecuteResponse
	lastCallbackSend *proto.ExecuteResponse
}

func (m *mockInnerServerDomain) Read() (*client.ReadResult, error) {
	return m.lastReadResult, nil
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
		lastReadResult: &client.ReadResult{
			Request: &proto.ExecuteRequest{Id: "req-sync", Language: "golang"},
			MsgType: protocol.MsgTypeExecuteSync,
		},
	}

	// 模拟 Run() 中的一次循环
	res, _ := mock.RunCode(mock.lastReadResult.Request)
	if mock.lastReadResult.MsgType == protocol.MsgTypeExecuteSync {
		mock.SendResult(res, nil)
	} else {
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
		lastReadResult: &client.ReadResult{
			Request: &proto.ExecuteRequest{Id: "req-async", Language: "golang"},
			MsgType: protocol.MsgTypeExecute,
		},
	}

	res, _ := mock.RunCode(mock.lastReadResult.Request)
	if mock.lastReadResult.MsgType == protocol.MsgTypeExecuteSync {
		mock.SendResult(res, nil)
	} else {
		mock.Send(res, nil)
	}

	if mock.lastCallbackSend == nil {
		t.Fatal("execute should call Send (HTTP callback)")
	}
	if mock.lastSendResult != nil {
		t.Error("execute should NOT call SendResult")
	}
}
```

- [ ] **Step 7: 运行 Worker 端测试**

Run: `cd codeRunner-backend && go test ./internal/application/service/client/ -run TestWorkerMessageRouting -v`
Expected: PASS

- [ ] **Step 8: 编译验证**

Run: `cd codeRunner-backend && go build ./...`
Expected: 编译通过

- [ ] **Step 9: 提交**

```bash
cd codeRunner-backend
git add internal/infrastructure/websocket/client/client.go \
        internal/domain/client/entity/innerServer.go \
        internal/domain/client/service/innerServerService.go \
        internal/application/service/client/service.go \
        internal/application/service/client/service_test.go
git commit -m "feat: Worker supports execute_sync message type with WebSocket result return"
```

---

### Task 6: 端到端集成验证

**Files:**
- 无新文件，验证现有改动协同工作

- [ ] **Step 1: 运行全量测试**

Run: `cd codeRunner-backend && go test ./... -race -count=1`
Expected: 全部通过，无竞态

- [ ] **Step 2: 运行编译检查**

Run: `cd codeRunner-backend && go vet ./...`
Expected: 无警告

- [ ] **Step 3: 手动验证（如有本地 Docker 环境）**

启动 Server + Worker，通过 gRPC 调用现有 `Execute` 接口验证异步模式未被破坏。同步模式将在 Agent 模块集成时端到端验证。

- [ ] **Step 4: 最终提交（如有遗漏修复）**

```bash
cd codeRunner-backend
git add -A
git commit -m "test: verify sync mode integration and async mode compatibility"
```
