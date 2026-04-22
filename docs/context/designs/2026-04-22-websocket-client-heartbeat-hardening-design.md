# WebSocket Client Heartbeat Hardening — Design

- 日期：2026-04-22
- 范围：`internal/infrastructure/websocket/client/client.go`
- 关联代码：`WebsocketClientImpl.heartBeat`、`connect`、`reconnect`、`Close`、`Read`
- 关联工程：Worker 节点与 Server 的长连接健康
- 参考 server 侧正确实现：`internal/infrastructure/websocket/server/client.go:122-152`

## 1. 背景与问题

Worker 与 Server 之间通过 WebSocket 保持长连接，Server 将 `execute` / `execute_sync` 任务通过此连接下发。生产环境日志（worker@`42.229.45.170` → server@`101.42.7.205:7979`）呈现稳定的失败循环：

```
连接成功 → 8~15 分钟 → read: connection reset by peer → 重连 → 循环
```

排查后定位到 client 侧 5 处病灶：

| 病灶 | 位置 | 影响 |
|---|---|---|
| B1. `pingPeriod = 2s` 过频 | `client.go:42` | 每秒约 1 帧控制流量，徒增开销，云厂商可能触发异常告警 |
| B2. `pongWait` 声明但未使用 | `client.go:33,43` | client 无 read deadline，无法主动发现死连接 |
| B3. 无 `SetPongHandler` / `SetPingHandler` | - | 无法在收到 pong/ping 时续期 deadline |
| B4. 重连后心跳 goroutine 立刻退出 | `Close()` `close(stopPingCh)`, `reconnect()` 调用 `connect()` 启新 goroutine, 新 goroutine 读到已关闭的旧 channel 立即退出 | **重连后心跳完全失效**，这是 NAT 老化的真正帮凶 |
| B5. `heartBeat` 里 ping 失败直接 `reconnect()` | `client.go:230-235` | 心跳 goroutine 发起重连，重连又新起心跳 goroutine，存在嵌套 / goroutine 泄漏风险 |

Server 侧（`internal/infrastructure/websocket/server/client.go`）实现正确：30s ping / 60s pongWait / PongHandler 续期 deadline。**本次只修 client，不动 server**。

## 2. 目标 / 非目标

### 目标

1. 让 client 在 read 侧设置并维护 `SetReadDeadline(pongWait)`，对齐 server 的行为。
2. client 主动每 30s 发一次 ping，同时响应 server 的 ping（gorilla/websocket 默认行为 + 可选 PongHandler 续期）。
3. 修复 B4：重连后 `stopPingCh` 能正确重建，新心跳 goroutine 正常工作。
4. 修复 B5：`heartBeat` 内部不再触发 `reconnect`，而是通过 error 信号让上层决策。
5. 重连策略改为指数退避 + 无限重试，消除"3 次失败进程退出"的脆弱点。
6. **保持 `WebsocketClient` 接口不变**：`Dail / Read / CallBackSend / WebsocketSend / Close` 签名与行为契约不变，上层（`internal/interfaces/adapter/initialize/app.go` 等）零修改。

### 非目标

- 不动 server 侧代码。
- 不改通信协议（消息格式、ACK 语义）。
- 不改跨环境配置 / 路径 / 端口。
- 不做 TLS / WSS 迁移（另一话题）。
- 不引入第三方重连库（`gorilla/websocket` 已够用）。
- 不暴露心跳参数为外部配置项（先硬编码，稳定后再提）。
- 不改日志级别 / 日志库（保留 zap）。

## 3. 架构与改动范围

**唯一改动文件**：`internal/infrastructure/websocket/client/client.go`。

结构性变化：

- `WebsocketClientImpl` 结构体：
  - 删除字段：`pingPeriod / pongWait / reconnectWait / stopPingCh`
  - 新增字段：`writeMu sync.Mutex`（保护所有 `conn.Write*`）
  - 新增字段：`lifecycleMu sync.Mutex`（保护 `conn` 与 `currentStopCh` 的替换，防止 `Close()` 与 `reconnect()` / `Read()` 自愈路径并发引发的 double-close panic）
  - 新增字段：`currentStopCh chan struct{}`（每条连接一份）
  - 新增字段：`closed bool`（用户显式调用 `Close()` 后设为 true，Read() 自愈路径据此决定是否放弃重连）
- 常量整理：`pingPeriod / pongWait / writeWait / reconnectInitialBackoff / reconnectMaxBackoff / reconnectBackoffFactor` 改为包级常量。
- `connect()`：拿到 conn 后立刻 `SetReadDeadline(pongWait)` + `SetPongHandler` + `SetPingHandler`（显式 `WriteControl(PongMessage)` + 重置 deadline），再起 heartbeat goroutine。
- `heartBeat(conn, stopCh)`：仅负责周期性发 ping；失败不自愈，直接 return。把 conn 和 stopCh 作为参数传入形成闭包，避免 `i.conn` 在重连时被替换引发竞态。
- **`Read()` 自愈（关键设计）**：`ReadMessage()` 失败后，若 `closed == false`，在方法内部调用 `reconnect()`，成功后**返回错误给上层**让其跳过本次业务处理；下一次 `Read()` 调用时使用新连接继续读。这样对 `service.go` 完全透明。
  - 为什么不是"内部重连 + 继续读"？因为 `service.go` 的 `Run()` 在 read 返回 err 时 `return err`，进程会被 docker `restart: unless-stopped` 拉起 —— 这等同于"应用层再试一次"。但这种粗暴方式：(1) 丢失进程内状态（容器池、日志 buffer），(2) 启动延迟 > 连接延迟，(3) 每次都重新拉镜像层。自愈后返回 err 让 service 退出的话，依然靠 restart —— 所以**单靠自愈还不够**，要么改 service 要么干脆自愈后继续读。
  - **最终选择**：`Read()` 自愈后**继续读新连接**，上层无感。这要求 `service.go` 的循环不动，`Read()` 内部的"err → reconnect → 重读"逻辑完整。
- `reconnect()`：指数退避 + 无限重试；由 `Read()` 内部调用。`Close()` 被显式调用后的重连会立即退出（检查 `closed`）。
- 保持所有方法的外部可见签名不变。

## 3.1 `Read()` 自愈流程详细

```go
func (i *WebsocketClientImpl) Read() (*ReadResult, error) {
    for {
        // 拿当前 conn 的快照。lifecycleMu 防止 reconnect 中途替换 conn
        i.lifecycleMu.Lock()
        conn := i.conn
        closed := i.closed
        i.lifecycleMu.Unlock()
        if closed {
            return nil, errors.ClientClosed  // 新增 sentinel
        }

        _, m, err := conn.ReadMessage()
        if err != nil {
            zap.S().Warnf("Worker read failed, attempting reconnect: %v", err)
            if reconnectErr := i.reconnect(); reconnectErr != nil {
                // 只有 closed 会让 reconnect 退出；此时上层也该退
                return nil, reconnectErr
            }
            // 重连成功，loop 一次用新 conn 再读
            continue
        }

        // 正常消息处理（ACK / unmarshal）...
        return &ReadResult{...}, nil
    }
}
```

关键不变量：
- `Read()` 只在两种情况下返回错误：(a) `Close()` 被显式调用；(b) JSON 反序列化失败等业务错误。
- 网络层错误一律在 `Read()` 内自愈，上层感知不到。
- `reconnect()` 内部检测到 `closed == true` 立刻返回 `errors.ClientClosed`，打破无限重试。

## 4. 新的心跳与超时机制

```
        t=0                   t=30                  t=60                  ...
client ──→ ping ──────────→ ping ──────────→ ping ──────────→
       ←── pong (server)   ←── pong            ←── pong
       ←── ping (server)   ←── ping            ←── ping
       ──→ pong (auto)     ──→ pong (auto)     ──→ pong (auto)
                ↑                   ↑                   ↑
        每次任何 recv 事件，ReadDeadline 延期到 now + 60s
```

关键参数（client 包级常量）：

| 名称 | 值 | 含义 |
|---|---|---|
| `pingPeriod` | 30 * time.Second | client 主动发 ping 间隔 |
| `pongWait` | 60 * time.Second | read deadline；必须 ≥ 2 * pingPeriod 留一次容错 |
| `writeWait` | 10 * time.Second | ping 写入超时（控制帧发送 deadline） |

gorilla/websocket 约定：
- `SetReadDeadline(t)`：下一次 `ReadMessage()` 若到 t 仍无任何帧到达，返回 i/o timeout 错误。
- `SetPongHandler(fn)`：每收到 pong 时自动调用 fn；fn 内重置 deadline。
- `SetPingHandler(fn)`：每收到 ping 时自动调用 fn；默认行为是自动回 pong，我们在自定义版本里**同时**重置 deadline。
- 这些 handler 只会被在 `ReadMessage()` 的上下文里触发 —— 上层 `Read()` 调用方必须持续循环调用，否则 handler 不会被调度。现有 `service.go` 就是这样调用的，无需改动。

## 5. `Close()` / 重连生命周期修复（B4 的根因）

**当前 bug**：

```go
// struct 字段（实例级）
stopPingCh chan struct{}

// NewWebsocketClientImpl()
stopPingCh: make(chan struct{}),   // 仅初始化一次

// Close()
close(i.stopPingCh)                // 第一次 reconnect() 后永久关闭

// connect() → go heartBeat()
for { select { case <-i.stopPingCh: return } }  // 读已关闭 channel 立即返回
```

→ 第一次重连后，新心跳 goroutine 秒退。之后 client 完全没有主动 ping，NAT 老化命中。

**修复**：把 `stopPingCh` 的生命周期**绑到单条连接上**，并用 `lifecycleMu` 防止 `Close()` 被并发调用导致 double-close panic：

```go
// 结构体新增字段
lifecycleMu   sync.Mutex
currentStopCh chan struct{}
closed        bool

// connect() 内部（被调用方已持锁，或自己持锁）：
func (i *WebsocketClientImpl) connect() error {
    // ... dialer.Dial ...
    i.lifecycleMu.Lock()
    if i.closed {
        i.lifecycleMu.Unlock()
        _ = conn.Close()
        return errors.ClientClosed
    }
    // 在替换 conn 前，先停掉旧连接的 heartbeat goroutine（若存在）
    if i.currentStopCh != nil {
        close(i.currentStopCh)
    }
    stopCh := make(chan struct{})
    i.currentStopCh = stopCh
    i.conn = conn
    i.lifecycleMu.Unlock()

    // 配置 deadline 与 handler 必须在锁外做（避免 handler 回调时再次拿锁死锁）
    conn.SetReadDeadline(time.Now().Add(pongWait))
    conn.SetPongHandler(func(string) error {
        conn.SetReadDeadline(time.Now().Add(pongWait))
        return nil
    })
    conn.SetPingHandler(func(appData string) error {
        conn.SetReadDeadline(time.Now().Add(pongWait))
        // 必须手动回 pong（覆盖默认 PingHandler 后默认行为消失）
        i.writeMu.Lock()
        defer i.writeMu.Unlock()
        return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(writeWait))
    })

    go i.heartBeat(conn, stopCh)
    return nil
}

// Close() 内部：幂等 + 防 double-close
func (i *WebsocketClientImpl) Close() error {
    i.lifecycleMu.Lock()
    defer i.lifecycleMu.Unlock()
    if i.closed {
        return nil
    }
    i.closed = true
    if i.currentStopCh != nil {
        close(i.currentStopCh)
        i.currentStopCh = nil
    }
    if i.conn != nil {
        return i.conn.Close()
    }
    return nil
}
```

要点：
- `lifecycleMu` 保护 `conn / currentStopCh / closed` 三者一起替换。
- `Close()` 幂等，`closed` 标志位防 double-close。
- `connect()` 在替换前会主动停掉旧 heartbeat（重连场景下旧 conn 已废弃但 heartbeat goroutine 可能还活着）。
- `PingHandler` 内 `WriteControl` **必须** 走 `writeMu`，否则会和 `heartBeat` 的 ping 写入并发。

## 6. 指数退避重连

```go
const (
    reconnectInitialBackoff = 3 * time.Second
    reconnectMaxBackoff     = 30 * time.Second
    reconnectBackoffFactor  = 2
)

func (i *WebsocketClientImpl) reconnect() error {
    // 关闭旧连接的 TCP 句柄，但不设置 closed 标志 —— 我们要重连，不是彻底关
    i.lifecycleMu.Lock()
    if i.closed {
        i.lifecycleMu.Unlock()
        return errors.ClientClosed
    }
    if i.currentStopCh != nil {
        close(i.currentStopCh)
        i.currentStopCh = nil
    }
    if i.conn != nil {
        _ = i.conn.Close()
    }
    i.lifecycleMu.Unlock()

    backoff := reconnectInitialBackoff
    for attempt := 1; ; attempt++ {
        // 每次重试前检查是否被显式 Close
        i.lifecycleMu.Lock()
        closed := i.closed
        i.lifecycleMu.Unlock()
        if closed {
            return errors.ClientClosed
        }

        zap.S().Infof("Worker reconnecting (attempt %d, backoff=%s) to %s:%s/%s",
            attempt, backoff, i.targetServer.host, i.targetServer.port, i.targetServer.path)
        if err := i.connect(); err == nil {
            zap.S().Info("Worker reconnect succeeded")
            return nil
        } else {
            zap.S().Warnf("Worker reconnect attempt %d failed: %v", attempt, err)
        }
        i.sleepFn(backoff) // sleepFn 默认 time.Sleep，测试时可替换
        if backoff < reconnectMaxBackoff {
            backoff *= reconnectBackoffFactor
            if backoff > reconnectMaxBackoff {
                backoff = reconnectMaxBackoff
            }
        }
    }
}
```

特点：
- **永不主动退出**：除非显式 `Close()` 被调用（`closed` 标志位）。
- **单调递增**：3s → 6s → 12s → 24s → 30s (cap)。
- **不再用 `MaxRetryAttemptsReached`** 常量（可保留定义但本文件不再使用）。
- **新增 `sleepFn func(time.Duration)` 字段**，默认 `time.Sleep`，单测可注入假时钟验证退避序列（T6 所需）。

### 为什么无限重试是合理的

- **本项目是个人运营**：没有"优雅降级"需求，长连接断开就是要继续重试。
- **唯一真正失败的场景**是显式 `Close()`，这条路径有 `closed` 标志位兜底。
- **认证类错误**在本 WS 握手阶段不存在（没有 token 校验），即使将来引入，也应该是建立一次就稳定有效，不会在重连时突然失效；若失效就是配置错误，值得继续重试直到人介入。

## 7. 接口保留

对外契约（`WebsocketClient` interface）**完全不变**：

```go
type WebsocketClient interface {
    Dail(TargetServer) error
    Read() (*ReadResult, error)
    CallBackSend(*proto.ExecuteResponse, error) error
    WebsocketSend(any) error
    Close() error
}
```

上层 `service.go` / `app.go` 零修改。

## 8. 测试策略

### 8.1 单元测试（新增 `client_test.go`）

使用 `httptest.NewServer` + `gorilla/websocket` upgrader 起一个 fake WebSocket server，观察 client 行为：

| 用例 | 方法 |
|---|---|
| T1. 连上后至少在 pingPeriod 内收到一次 client 发起的 ping | fake server `SetPingHandler`，等 35 秒内收到 ≥1 次 ping |
| T2. pongWait 到期且期间无任何帧 → Read 触发自愈（未触发业务错误返回） | 启动两个 fake server：A 在收到连接后冻结所有帧，B 立刻正常响应。client 连 A → `Read()` 阻塞到 pongWait → 内部 reconnect 转向 B（通过改 targetServer host）→ 返回正常 read 结果 |
| T3. 每次收到 server ping，ReadDeadline 被续期（验证自定义 PingHandler 手动回 pong 正确） | fake server 每 10s 主动发 ping 持续 90s，且严格要求收到对应 pong 才继续；client `Read()` 保持阻塞直到被主动 close |
| T4. Close 能停掉心跳 goroutine 且幂等 | 连上后 `Close()` 两次，sleep 2×pingPeriod，fake server 不再收到 ping；第二次 Close 不 panic |
| T5. 重连后心跳恢复（B4 回归） | 让 fake server 主动断开连接 → Read 自愈重连 → fake server 在新连接上在 pingPeriod 内再次收到 ping |
| T6. 指数退避序列正确 | 注入 mock `sleepFn`，让 `connect()` 连续失败 5 次，断言 sleepFn 被调用的参数序列为 `[3s, 6s, 12s, 24s, 30s]`（最后一个封顶） |
| T7. Close 期间 reconnect 能立即退出 | 开一个 goroutine 调用 `reconnect()`（让 dial 失败进入退避循环），主 goroutine 调用 `Close()`；断言 reconnect 在 pollInterval 内返回 `ClientClosed` |
| T8. writeMu 串行化 ping / ACK / WebsocketSend | race detector（`go test -race`）下，并发触发三种 write 路径 1 秒，无数据竞争告警 |

### 8.2 本地手工验证

- server + client 本机跑，模拟 NAT 老化：用 `iptables` 在连接 idle 90s 后丢包单向；观察 client 在 pongWait 内检测到并重连。
- 长跑：生产环境跑 24 小时，抓 `ws_clients_connected` Prometheus 指标曲线，期望保持恒为 1（或 worker 数量），不再出现大段 0。

## 9. 关键实现注意事项（不是"风险"，是必须做对的点）

### 9.1 PingHandler 必须手动回 Pong

gorilla/websocket 的默认 `PingHandler` 会自动回 Pong。**一旦你调用 `SetPingHandler(...)` 覆盖它，默认行为消失**，必须在自定义 handler 内显式 `WriteControl(PongMessage, ...)`，否则 server 收不到 pong → server 侧 pongWait 触发 → server 关连接 → 又是同样的故障。

正确写法见 §5 中的 `SetPingHandler` 代码段，关键点：

- 用 `appData` 作为 PongMessage 的 payload（按 RFC 6455，pong 必须 echo ping 的 payload）
- 用 `writeWait` 作为写入 deadline
- 用 `writeMu` 串行化（与 heartBeat 的 ping、Read 的 ACK、WebsocketSend 互斥）

### 9.2 并发写入审计

gorilla/websocket 文档：**`WriteMessage` / `WriteControl` 不是 goroutine-safe**。本文件中所有 `conn.Write*` 调用：

| 位置 | 调用方 goroutine |
|---|---|
| `Read()` 内部 ACK 写入 | service.go 的 Read loop goroutine |
| `heartBeat()` 的 ping 写入 | heartBeat goroutine |
| `WebsocketSend()` | 业务调用方 goroutine |
| `PingHandler` 的 pong 写入（新增） | Read loop goroutine（gorilla 在 ReadMessage 内分发 handler） |

→ 至少 3 个不同 goroutine 都会写。**必须用 `writeMu` 串行化所有 `conn.Write*`**。这是新发现的 bug，不在原有 5 项病灶里，但本次修复一并解决。

### 9.3 ReadMessage 必须持续被调用

PongHandler / PingHandler 只在 `ReadMessage()` 的调用栈里被分发。本项目 `service.go` 已经在循环里持续调用 `Read() → ReadMessage()`，满足条件。**不要**把 Read 改成"按需读"。

### 9.4 `Read()` 自愈不能形成无限快循环

如果 reconnect 成功了但下一次 ReadMessage 又立刻失败，`Read()` 内的 `for` 循环可能空转。`reconnect()` 自带退避足以保护这一点 —— 真正的"网络炸毁"场景下 reconnect 退出前已经退避了 30s，自愈循环每轮至少 30s，不会爆 CPU。但仍要在测试 T2 里覆盖"连接极不稳定"的场景，验证不会失控。

## 10. 落地步骤

1. 在 `client.go` 顶部加包级常量 `pingPeriod / pongWait / writeWait / reconnectInitialBackoff / reconnectMaxBackoff / reconnectBackoffFactor`。
2. 结构体：删除 `pingPeriod/pongWait/reconnectWait/stopPingCh` 字段；新增 `writeMu sync.Mutex` / `lifecycleMu sync.Mutex` / `currentStopCh chan struct{}` / `closed bool` / `sleepFn func(time.Duration)`（默认 `time.Sleep`）。
3. `NewWebsocketClientImpl` 简化字段初始化，注入默认 `sleepFn = time.Sleep`。
4. `connect()`：dial 成功后按 §5 流程拿 `lifecycleMu` 替换 conn / stopCh，再在锁外 `SetReadDeadline / SetPongHandler / SetPingHandler`（PingHandler 内必须 `WriteControl(PongMessage)` 并走 `writeMu`），再 `go heartBeat(conn, stopCh)`。
5. `heartBeat(conn, stopCh)`：循环 ping，写入走 `writeMu`，写入失败 return；不再调用 reconnect。
6. `Read()`：按 §3.1 改成自愈循环，read err → reconnect → 继续；只有 `closed` 或反序列化错误才 return。
7. `Close()`：按 §5 改成幂等，加 `closed` 标志位与 `lifecycleMu`。
8. `reconnect()`：按 §6 改成无限指数退避，`closed` 检查兜底。
9. 引入新 sentinel 错误 `errors.ClientClosed`（在 `internal/infrastructure/common/errors/` 加常量；本次顺手加，不是独立改动）。
10. **审计** `WebsocketSend()`：所有 `conn.Write*` 加 `writeMu`。`Read()` 内的 ACK 写入也加 `writeMu`。
11. 新增 `client_test.go`，覆盖 T1-T8。其中 T2/T5 是行为级集成测试（fake server），T6/T7 用 `sleepFn` 注入，T8 跑 `-race`。
12. `go test -race ./internal/infrastructure/websocket/...` 全绿。
13. `go vet ./...` / `go build ./...` 全绿。
14. commit 拆成两个：(a) `errors.ClientClosed` 常量；(b) client.go 重构 + 测试。
15. push，CD 自动部署。
16. **生产灰度验证**：
    - 部署后 worker 容器自动重启一次（CD 触发）。
    - 立刻 curl `/api/v1/query?query=ws_clients_connected` 应为 1。
    - 1 小时后曲线应保持 1，无任何归零。
    - 24 小时后回看，确认完全消除原先的"每 8-15 分钟掉线"循环。
