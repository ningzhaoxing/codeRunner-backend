# 并发与沙箱风险评估

日期：2026-04-20
评估范围：大量用户同时提交代码执行时的风险点。

## 结论

当前配置在 **单机低并发** 下可用；大并发（> 池容量）会从业务降级（超时）直到 **宿主机级 DoS**。沙箱基础封锁（无网络、无 capability、内存/CPU 限额）已到位，但缺少 PID、磁盘、跨用户隔离的纵深防御。

## 风险清单（按严重度排序）

### 🔴 P0 — 致命

#### 1. Fork 炸弹打挂宿主机
- **现象**：runner 容器没有 `pids_limit`，用户代码 `while true: fork()` 会耗尽宿主机全局 PID（Linux 默认 `kernel.pid_max ≈ 4M`，但 cgroup 没限额时单容器可占满）。
- **影响**：宿主机级 DoS，所有容器（包括 server/client、其他 runner）都会无法创建新进程。
- **修复**：`docker-compose/{dev,product}/client/docker-compose.yml` 所有 runner 加 `pids_limit: 64`。
- **成本**：1 行 YAML × 8 runner × 2 环境。

#### 2. 跨用户 / 跨语言代码可见
- **现象**：`/tmp:/app/tmp:shared` 把宿主机整个 `/tmp` 挂进 client 容器；每个 runner 又挂 `/tmp/<lang>-N:/app`。用户代码执行时 `ls /app` 看得到本次 slot 的所有历史残留，而 client 容器可以读 `/tmp/*` 所有 runner 的目录。
- **影响**：A 用户的代码可以读到 B 用户几秒前提交的源码与输出。隐私 / 知识产权泄漏。
- **修复**：
  - runner 不再共享宿主机 `/tmp`；改用 docker named volume 或 per-slot bind mount。
  - 每次执行后**强制清理**整个 slot 目录（不是只删 UUID 子目录）。
- **成本**：中，需同步 `runCode.go` 清理逻辑和 compose 挂载配置。

---

### 🟠 P1 — 业务严重

#### 3. 池被单用户独占导致 DoS
- **现象**：无 per-uid 速率限制 / 配额。单个恶意或失控客户端可反复提交 `time.Sleep(30s)`，在 5s 内占满所有 slot，其他用户直接 `ErrContainerPoolExhausted`。
- **影响**：服务级 DoS，其他用户全部被拒。
- **修复**：gRPC 拦截器加 per-uid 令牌桶（如 `golang.org/x/time/rate`），限制每 uid QPS 和在途请求数。
- **成本**：中，加 1 个 interceptor + Redis 后端。

#### 4. 容器复用累积污染
- **现象**：`InContainerRunCode` 只做 `docker exec`。用户 `go run` / `python` 启动的孙子进程、后台线程、未关闭的文件不会因为 exec 结束而退出。长命容器里进程数、FD、临时文件持续增长，直到执行超时才走 `replenish` 路径重启。
- **影响**：p99 延迟劣化；偶发 OOM；同 slot 下个用户受污染。
- **修复**：每次执行结束后，在容器内 `pkill -9 -u runner`（或类似）再归还池；或切换到短命容器模型（每次执行起一个新容器，`--rm`）。
- **成本**：中高。短命方案性能代价在 Java（JVM 冷启动）最大。

---

### 🟡 P2 — 加固项

#### 5. 容器可写磁盘未限额
- **现象**：runner 没设 `read_only: true`，也没限制 tmpfs 大小。`dd if=/dev/zero of=/app/x` 能把 `/tmp/<lang>-N/` 写到宿主机 `/tmp` 分区写满。
- **修复**：加
  ```yaml
  read_only: true
  tmpfs:
    /tmp: size=64m,mode=1777
  ```
  代码写入目录仍然是 `/app`，需确认 runtime 只往 `/app` 写；若编译产物也写 `/app`，可能需要把 `/app` 改成有限额的 tmpfs。
- **成本**：小，但需回归测试确认各语言 runner 能正常编译运行。

#### 6. 缺少 `no-new-privileges`
- **现象**：`cap_drop: ALL` 已经很严，但没有 `security_opt: ["no-new-privileges:true"]`。
- **影响**：纵深防御缺失。已有 cap_drop 情况下实际提权路径很窄。
- **修复**：每个 runner 加 `security_opt: ["no-new-privileges:true"]`。
- **成本**：1 行。

#### 7. 无 ulimit（nofile / fsize）
- **现象**：用户代码可开 10 万 FD 或写 10GB 单文件（受 mem_limit 间接限制）。
- **修复**：加 `ulimits: {nofile: 256, fsize: 10485760}`。
- **成本**：1 行。

#### 8. `rc.file.Close()` nil panic
- **位置**：[runCode.go:215](../../internal/infrastructure/containerBasic/runCode.go#L215)
- **现象**：`createFile` 在 `getFileExtension` / `createBlockContent` 阶段失败时 `rc.file` 仍为 nil，defer 调用会 panic 直接挂掉 worker。
- **修复**：`if rc.file != nil { rc.file.Close() }`。
- **成本**：3 行。

#### 9. 池规模小
- **现象**：各语言仅 1-2 个 slot，单并发 > 2 即排队。
- **修复**：根据目标 QPS × 执行耗时扩容，或改成短命容器动态起停。
- **成本**：配置 / 架构层面。

---

## 修复优先级建议

**立即（一个 PR）**：#1 pids_limit + #5 read_only + tmpfs + #6 no-new-privileges + #7 ulimits + #8 nil 检查 —— 全是局部改动，能把「宿主机级 DoS」和「worker panic」两个最坏情况先堵上。

**短期（一周内）**：#3 per-uid 限流。

**中期**：#2 跨用户隔离（换 named volume + 清理策略）、#4 执行后进程清理。

**长期**：考虑 gVisor / Kata / Firecracker 替代共享内核的 runc，或切换短命容器 + 镜像预热。

## 相关代码

- 容器池：[internal/infrastructure/containerBasic/pool.go](../../internal/infrastructure/containerBasic/pool.go)
- 执行入口：[internal/infrastructure/containerBasic/runCode.go](../../internal/infrastructure/containerBasic/runCode.go)
- 运行配置：[docker-compose/product/client/docker-compose.yml](../../docker-compose/product/client/docker-compose.yml)
