# P2C + EWMA 自适应负载均衡

## 为什么要从 WeightedRR 迁移到 P2C

### 原 WeightedRR 存在的问题

1. **并发竞态（Issue #1）**：`nodes` 切片无锁保护，`Add/Remove/Next` 被多个 goroutine 同时调用时会触发 data race，轻则数组越界 panic，重则静默数据损坏。

2. **静态权重，无法感知节点真实负载**：权重在客户端连接时通过 `?weight=100` 传入后就固定不变。如果某台 VM 因为 CPU 打满、网络拥塞等原因变慢，WeightedRR 仍然按原权重分配流量，导致慢节点持续积压请求。

3. **`Remove` 未更新 `totalWeight`**：节点移除后 `totalWeight` 仍包含已删除节点的权重，导致后续所有轮询的权重计算偏移。

4. **无健康感知**：节点发送失败后没有任何反馈机制，下一次请求仍可能被分配到同一个故障节点。

### P2C + EWMA 如何解决这些问题

| 问题 | P2C 的解决方式 |
| ---- | -------------- |
| 并发竞态 | atomic 操作 + per-node mutex，热路径无全局锁竞争 |
| 静态权重无法感知负载 | EWMA 实时跟踪发送延迟 + 在途请求数，自动将流量导向更快的节点 |
| 权重计算偏移 | 不依赖全局 totalWeight，每次只比较两个节点的实时 load |
| 无健康感知 | 跟踪发送成功率，成功率低于 50% 的节点自动被惩罚（load = MaxInt32） |

---

## 算法原理

### P2C（Power of Two Choices）

核心思想：**随机选两个，挑更好的那个**。

相比遍历所有节点选最优（O(n)、需要全局锁），P2C 只需比较两个随机节点（O(1)），在大规模集群下性能优势明显。数学上已证明，P2C 将最大负载从 O(log n / log log n) 降低到 O(log log n)。

```
Pick 流程：
  0 个节点 → 返回错误
  1 个节点 → 直接选
  2+ 个节点 → 随机选 2 个 → 比较 load → 选低的
```

### EWMA（指数加权移动平均）

用于平滑延迟和成功率的波动，避免单次毛刺导致节点被错误惩罚。

```
衰减权重 w = e^(-elapsed / 10s)

new_lag     = old_lag     × w  +  current_lag  × (1 - w)
new_success = old_success × w  +  success_val  × (1 - w)
```

- `elapsed` 越大（节点越久没更新），历史数据影响越小
- 10 秒内的数据影响力 > 37%，30 秒前的数据基本衰减为 0

### Load 计算公式

```
load = √(ewma_lag + 1) × (inflight + 1) / weight
```

- `ewma_lag`：发送延迟越高 → load 越大
- `inflight`：在途请求越多 → load 越大
- `weight`：配置权重越高 → load 越小（兼容原有 weight 参数）
- `√` 对延迟取平方根，避免偶发高延迟过度影响选择

### 防饥饿机制

如果某个节点超过 1 秒没被选中，即使它的 load 较高也会被强制选择一次。这确保：
- 新加入的节点能获得初始流量来建立 EWMA 基线
- 恢复健康的节点不会被永久冷落

---

## 适配说明

go-zero 内置的 P2C 是 gRPC 连接级别的负载均衡，无法直接用于本项目的 WebSocket worker 池场景。本实现借鉴了 go-zero `zrpc/internal/balancer/p2c` 的核心算法（P2C 选择 + EWMA 延迟跟踪 + 防饥饿 + 健康检测），针对以下差异做了适配：

| 差异点 | go-zero 原版 | 本项目适配 |
| ------ | ------------ | ---------- |
| 节点类型 | gRPC SubConn | WebSocket Client（长连接 worker） |
| 延迟来源 | RPC 端到端延迟 | WebSocket Send 延迟（反映连接健康度） |
| 完成信号 | gRPC Done 回调 | 手动 `Done(id, duration, err)` 在 Send 后调用 |
| 接口形态 | gRPC Balancer/Picker | 自定义 `LoadBalancer` 接口 |

由于服务端在 `client.Send()` 后无法感知代码执行完成时间（结果通过 HTTP callback 直接回传调用方），EWMA 基于 **WebSocket 发送延迟** 而非端到端执行延迟。发送延迟虽然量级较小，但能有效反映连接拥塞、网络抖动等问题，结合在途请求数已足够做出合理的负载决策。

---

## 常量参考

| 常量 | 值 | 含义 |
| ---- | -- | ---- |
| `decayTime` | 10s | EWMA 衰减窗口 |
| `forcePickTimeout` | 1s | 强制选择超时 |
| `initSuccess` | 1000 | 初始成功率（代表 100%） |
| `throttleSuccess` | 500 | 健康阈值（低于此值视为不健康） |
| `penalty` | MaxInt32 | 不健康节点的惩罚 load 值 |
