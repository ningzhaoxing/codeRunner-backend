# Issues & 优化迭代

## 🔴 Bug 级别（会出错）

### 1. 负载均衡并发竞态

**文件**：`internal/infrastructure/balanceStrategy/weightedRRBalance/weightedRR.go`

`WeightedRR` 的 `nodes` 切片没有任何锁保护，`Add / Remove / Next` 可能被多个 goroutine 同时调用：

- VM 断线 → 触发 `Remove`
- 新请求进来 → 触发 `Next`
- 两个 goroutine 同时操作切片 → data race，轻则数组越界 panic

**修法**：加 `sync.RWMutex`，`Next / Get` 用读锁，`Add / Remove` 用写锁。

---

### 2. 心跳检测单向，实际没起作用

**文件**：`internal/infrastructure/websocket/server/client.go:40-49`

`HeartBeat()` 只设置了 `SetPingHandler`（等待对方发 Ping），服务端从来不主动发 Ping。
Gorilla WebSocket 默认会自动回复 Pong，但 `SetPingHandler` 覆盖默认行为后，没有手动发 Pong，导致客户端认为服务端没有响应。

**修法**：PingHandler 里加上 `c.conn.WriteMessage(websocket.PongMessage, []byte{})`。

---

### 3. 已关闭客户端的处理逻辑有误

**文件**：`internal/domain/server/service/clientManager.go:71-78`

```go
if client.IsClosed() {
    err := s.RemoveClient(client.GetId())
    ...
    return nil, err  // 移除成功时 err 是 nil，调用方拿不到有效客户端
}
```

`IsClosed()` 为 true 时，不管移除是否成功都返回 `nil, err`，调用方直接报错，本次请求失败。
应该移除后**重新调用一次负载均衡**换一台可用机器，而不是直接返回错误。

---

## 🔴 安全问题

### 4. 密码与 JWT Secret 硬编码

**文件**：`internal/infrastructure/common/token/tokenPublic.go`

```go
password == "123456"
jwt_secret = "I'm codeRunner"
```

密码和 Secret 硬编码在代码里并提交到 git，任何能看到仓库的人都能伪造 token。

**修法**：从环境变量读取，`os.Getenv("JWT_SECRET")`。

---

### 5. WebSocket 无 Origin 校验

**文件**：`internal/interfaces/controller/server/server.go`

Upgrader 的 `CheckOrigin` 直接 `return true`，任何域名都能建立连接，属于不必要的攻击面。

---

### 6. 代码执行无输入长度限制

`ExecuteRequest.codeBlock` 没有任何长度校验，用户可以提交超大代码，造成磁盘和 CPU 的无意义消耗。

---

## 🟡 架构设计缺陷

### 7. CallBack HTTP 回传无重试保障

**文件**：`internal/infrastructure/websocket/client/client.go`

执行完代码后通过 HTTP POST 把结果发回 `callBackUrl`：

- 没有重试：HTTP 请求失败直接丢弃结果，调用方永远收不到响应
- 没有超时配置：使用默认 HTTP Client，可能一直阻塞
- 没有幂等保障：回调失败后重连重试，可能重复执行代码

---

### 8. 断线重连期间在途请求丢失

**文件**：`internal/infrastructure/websocket/client/client.go`

客户端检测到断线后重连，但重连期间服务端可能已通过这条连接下发了执行请求，该请求永久丢失，调用方也不会收到任何错误通知。

**方向**：服务端在连接断开时将未完成的请求重新入队。

---

### 9. gRPC 接口语义不一致

**文件**：`internal/interfaces/controller/server/execute.go:15`

```go
return nil, err  // gRPC Execute 永远返回 nil response
```

`Execute` gRPC 接口是同步调用，但实际结果通过异步 HTTP 回调返回给 `callBackUrl`，gRPC 接口永远返回 `nil, nil`。调用方需要自己实现回调接收端，接口语义不清晰。

---

### 10. 容器资源限制藏在代码里

**文件**：`internal/infrastructure/containerBasic/container.go`

512MB 内存 / 1CPU / 网络隔离等配置通过 Docker SDK 在 Go 代码中硬编码设置，不直观。
由于每种语言对应固定容器，资源限制固定，可以移到 `docker-compose.yml` 中统一配置，代码更干净，配置更透明。

---

## 🟡 细节问题

### 11. 两套日志库并存

项目同时引入 `logrus` 和 `zap`，但 `logger.go` 只初始化了 zap，实际代码全部使用 logrus，且 logrus 没有配置文件输出，日志只打到控制台。应统一为一套。

---

### 12. 临时文件路径映射脆弱

**文件**：`internal/infrastructure/containerBasic/runCode.go:164-181`

```go
path := fmt.Sprintf("/app/tmp/%s/%s", request.Language, uniqueID)       // 宿主机路径
containerPath := fmt.Sprintf("/app/%s/main.%s", uniqueID, r.extension)  // 容器内路径
```

两个路径格式不一致，能跑是因为宿主机 `/tmp/{language}` 挂载到了容器 `/app`，路径对应关系脆弱，挂载配置变动即会出错。

---

## 🟢 可观测性缺失（迭代优化）

### 13. 没有 Prometheus 指标暴露

生产环境无法监控：

- 各语言执行成功率 / 平均耗时
- 容器启动失败次数
- VM 连接数变化

**方向**：集成 `prometheus/client_golang`，暴露 `/metrics` 端点。

---

### 14. 请求链路无 TraceID

gRPC 请求进来后，日志里没有统一打印 `ExecuteRequest.id`，一条请求从接收到执行完成无法在日志里串联完整链路，排查问题困难。

**方向**：在 gRPC 拦截器中提取/生成 TraceID，注入 context，日志统一输出。

---

## 优先级汇总

| 优先级 | 问题 | 影响 |
| ------ | ---- | ---- |
| 🔴 立即修 | 负载均衡竞态 (#1) | 服务 panic |
| 🔴 立即修 | 心跳 Ping/Pong 逻辑错误 (#2) | 连接假活 |
| 🔴 立即修 | 密码/Secret 硬编码 (#4) | 安全漏洞 |
| 🟡 近期修 | 已关闭客户端处理逻辑 (#3) | 请求失败 |
| 🟡 近期修 | CallBack 无重试 (#7) | 结果丢失 |
| 🟡 近期修 | 两套日志库 (#11) | 日志混乱 |
| 🟡 近期修 | 容器资源配置移到 docker-compose (#10) | 可维护性 |
| 🟢 迭代优化 | Prometheus 指标 (#13) | 可观测性 |
| 🟢 迭代优化 | TraceID (#14) | 可排查性 |
| 🟢 迭代优化 | gRPC 接口语义 (#9) | 使用方体验 |

---

## 🔵 框架迁移：引入 go-zero（渐进式替换）

当前项目的 gRPC、服务发现、负载均衡、日志、配置、认证、监控均为手搓实现或散装依赖拼接，存在并发安全、可观测性缺失等问题。引入 go-zero 框架可系统性解决其中多项 issue，同时获得限流、熔断、自适应负载均衡等生产级能力。

### 现状 vs go-zero 对照

| 能力 | 现状 | go-zero 内置 |
| ---- | ---- | ------------ |
| gRPC 服务 | 裸 `google.golang.org/grpc` | zRPC（封装 gRPC） |
| HTTP 服务 | Gin | rest 框架 |
| 服务发现 | 裸 etcd client v3 | etcd 服务发现（自动注册/注销） |
| 负载均衡 | 手写 WeightedRR（**有竞态 #1**） | P2C 自适应负载均衡（线程安全） |
| 配置管理 | Viper + YAML | conf（YAML/JSON，支持环境变量） |
| 日志 | Zap + Logrus 混用（**#11**） | logx（统一日志，自动携带 TraceID） |
| JWT 认证 | 手写拦截器 + 硬编码 Secret（**#4**） | JWT 中间件 |
| 链路追踪 | 无（**#14**） | OpenTelemetry（自动注入 TraceID） |
| Prometheus | 无（**#13**） | 内置指标暴露 |
| 超时/重试 | 无（**#7**） | httpc + Breaker + Timeout |
| 限流 | 无 | 自适应限流 |

### 阶段 1：gRPC → zRPC + 服务发现 + 负载均衡

**解决 issue**：#1 负载均衡竞态、#14 TraceID

**改动范围**：

- 删除 `internal/infrastructure/balanceStrategy/`（手写负载均衡）
- 删除 `internal/infrastructure/etcd/`（手写 etcd 注册）
- 删除 `internal/infrastructure/grpc/`（裸 gRPC 注册）
- 替换为 go-zero zRPC server/client，内置 etcd 服务发现、P2C 负载均衡、OpenTelemetry

P2C（Pick of 2 Choices）算法根据实时延迟和负载自动选择最优节点，不需要手动配权重，天然线程安全，且自动摘除不健康节点。

---

### 阶段 2：配置 / 日志 / 认证

**解决 issue**：#4 密码硬编码、#11 两套日志库

**改动范围**：

- 删除 `internal/infrastructure/config/`（Viper 配置）
- 删除 `internal/infrastructure/common/logger/`（Zap 初始化）
- 移除所有 logrus 调用
- 替换为 go-zero conf + logx + JWT middleware

**配置示例**：

```yaml
# etc/coderunner.yaml
Name: coderunner
ListenOn: 0.0.0.0:50011
Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: code-runner
Auth:
  AccessSecret: ${JWT_SECRET}  # 环境变量注入，解决 #4
  AccessExpire: 86400
Telemetry:
  Name: coderunner
  Endpoint: http://jaeger:14268/api/traces
  Sampler: 1.0
```

---

### 阶段 3：可观测性

**解决 issue**：#13 Prometheus

零代码改动，配置开启即可：

```yaml
Prometheus:
  Host: 0.0.0.0
  Port: 9091
  Path: /metrics
```

go-zero 自动暴露 RPC 请求 QPS / 延迟 / 错误率、连接数等指标。业务自定义指标通过 `metric` 包添加。

---

### 阶段 4：HTTP 层（可选）

保留 Gin 仅用于 `/ws` 端点的 WebSocket upgrade，其余 HTTP API 可迁移至 go-zero rest。收益不大，优先级最低。

---

### 不受影响的模块

| 模块 | 原因 |
| ---- | ---- |
| `websocket/server/` `websocket/client/` | WebSocket 连接管理是核心业务，go-zero 不涉及 |
| `containerBasic/` | Docker SDK 操作，与框架无关 |
| `domain/` `application/` | 业务逻辑层，仅需调整依赖注入方式 |

### Issue 覆盖汇总

| Issue | 迁移阶段 | 解决方式 |
| ----- | -------- | -------- |
| #1 负载均衡竞态 | 阶段 1 | go-zero P2C 替代手写 WeightedRR |
| #4 密码硬编码 | 阶段 2 | go-zero conf + 环境变量注入 |
| #7 CallBack 无重试 | 阶段 1 | httpc + Breaker |
| #11 日志混乱 | 阶段 2 | logx 统一替代 Zap + Logrus |
| #13 Prometheus | 阶段 3 | 配置一行开启 |
| #14 TraceID | 阶段 1 | OpenTelemetry 自动注入 |
| #2 #3 #5 #6 #8 #9 #10 #12 | — | 仍需手动修复，与框架无关 |
