# WebSocket 请求可靠投递：ACK 确认机制

## 问题背景

### 当前请求链路

```
调用方 → gRPC → Server.Execute() → WebSocket → Client(VM) → Docker执行 → HTTP callback → 调用方
```

Server 调用 `client.Send(request)` 后即认为投递完成，返回 `nil`。但这里存在一个**危险窗口**。

### 危险窗口分析

```
时间轴：

T1: conn.WriteMessage(request) 成功 → 数据写入本地 TCP 发送缓冲区
T2: 网络抖动 / Client 进程崩溃 / 内网掉电
T3: Client 触发断线重连
T4: TCP 连接断开，缓冲区数据丢失
T5: Client 重连成功，新连接上没有 T1 的 request

结果：
  - Server 认为 Send 成功（T1 时 WriteMessage 返回 nil）
  - Client 从未收到这条 request
  - 调用方永远等不到 callback
  - 没有任何报错日志，问题无法被发现
```

**核心误区**：`conn.WriteMessage()` 返回 `nil`，只意味着数据写入了**本地 TCP 发送缓冲区**，不代表对端收到。这是 TCP 协议的基础特性——写入缓冲区和对端确认收到是两件事。

---

## 这不是新问题——TCP 和消息队列早就解决过它

### TCP 的解法：ACK + 重传

TCP 本身就是"不可靠网络上的可靠传输"，它的可靠性来自于：

```
发送方                          接收方
  │                                │
  │──── 数据段（seq=1）───────────>│
  │                                │ 收到，回 ACK
  │<─── ACK（ack=2）──────────────│
  │                                │
  │  发送方确认收到 ACK 后          │
  │  才将数据从发送缓冲区清除       │
  │                                │
  │  若超时未收到 ACK → 重传       │
```

关键机制：
- **序列号**：每个字节都有编号，接收方用 ACK 编号告知"我收到了哪里为止"
- **超时重传**：发送方若一定时间内未收到 ACK，重发同一数据段
- **滑动窗口**：允许多个未 ACK 的数据在途，提高吞吐量

### 消息队列的解法：消费确认（Consumer ACK）

以 RabbitMQ / Kafka 为例：

```
Producer → MQ(持久化) → Consumer
                           │
                           │ 消费并处理成功后
                           │── ACK ──> MQ 删除该消息
                           │
                           │ 处理失败 or 未 ACK
                           └── MQ 重新投递（或死信队列）
```

核心原则：**消息在被明确 ACK 之前，视为未处理，随时可重投**。

### 本项目的 ACK 方案

本项目的方案直接借鉴了上述两种机制的核心思想：

| 机制 | TCP | 消息队列 | 本项目 |
|---|---|---|---|
| 可靠性保障 | ACK + 超时重传 | Consumer ACK + 重投 | WebSocket ACK + 断线重发 |
| 确认粒度 | 字节级别 | 消息级别 | Request 级别 |
| 未确认时行为 | 超时重传 | 重新入队 | 断线时重发给其他节点 |
| 持久化 | TCP 缓冲区 | MQ 磁盘 | Server 内存（pendingReqs） |

**区别**：TCP 和 MQ 都有超时重传机制，本项目 ACK 方案目前只在**断线事件**时触发重发，没有超时重传。这是一个有意识的简化——如果 Client 在线但执行超时，需要另外的超时机制（可作为后续迭代）。

---

## 方案 B 实现：Client 收到 request 后立即回 ACK

### 核心设计

```
Server                              Client (VM)
  │                                    │
  │── {type:"execute",                 │
  │    request_id:"xxx",               │
  │    payload:...} ─────────────────>│
  │                                    │ 收到后立即（执行前）回 ACK
  │<── {type:"ack",                   │
  │     request_id:"xxx"} ────────────│
  │  pendingReqs 中移除 "xxx"          │ 开始执行代码...
  │                                    │
  │                                    │── HTTP callback ──> 调用方
```

**关键决策**：ACK 在**执行前**发出，而非执行完成后。

这与消息队列的"处理成功后 ACK"不同，原因是：
1. 代码执行结果通过 HTTP callback 独立回传，Server 无法感知执行完成
2. 执行过程中断线（Client 崩溃）→ 结果通过 callback 机制的重试保障（#7 已修复）
3. Server 只需要保障 request "被 Client 接收到"，不需要等执行完成

### 消息协议扩展

```go
// 新增 WebSocket 消息封装层
type MsgType string
const (
    MsgTypeExecute MsgType = "execute"
    MsgTypeAck     MsgType = "ack"
)

type WsMessage struct {
    Type      MsgType `json:"type"`
    RequestID string  `json:"request_id"`
    Payload   []byte  `json:"payload,omitempty"`
}
```

### pendingReqs 数据结构

```
pendingReqs: sync.Map
  key: clientID (string)
  value: *clientPending
    ├── mu: sync.Mutex
    └── reqs: map[requestID]*proto.ExecuteRequest
```

### 断线重发流程

```
Client 断线
  │
  ├── service.Run() 的 Read() 返回 error
  │
  ├── DrainPending(clientID) → 取出所有未 ACK 的 request
  │
  └── for each request:
          Execute(request) → 重新经过负载均衡分发给其他 Client
```

### 与 TCP 重传的类比

| | TCP | 本方案 |
|---|---|---|
| 触发条件 | 超时未收到 ACK | 连接断开 |
| 重传对象 | 未被 ACK 的数据段 | 未被 ACK 的 request |
| 如何知道未被 ACK | 维护发送窗口 | 维护 pendingReqs map |
| 重传目标 | 同一个对端 | 负载均衡选新节点（类似 TCP 换路由） |

---

## 已知局限

1. **无超时重传**：Client 在线但 ACK 丢失（极罕见）时无法触发重发。后续可加 TTL 扫描。
2. **内存存储**：Server 重启后 pendingReqs 丢失。需持久化（Redis/DB）才能做到跨进程可靠。
3. **ACK 前执行中断**：ACK 发出到 Execute 完成之间 Client 崩溃，request 会被重发，可能重复执行。调用方应实现幂等接收（`X-Request-ID` header 已在 callback 中携带）。
