---
title: 容器池技术方案
type: design
status: draft
created: 2026-03-27
updated: 2026-04-05
related:
  - docs/context/designs/architecture-roadmap.md
---

# 容器池技术方案

> 状态：待实现（Review 反馈已整合，待编码）  
> 关联优化：[架构优化迭代方案 §1](./architecture-roadmap.md)

---

## 一、现状与问题

### 当前执行链路

```
RunCode(request)
    ↓
InContainerRunCode(language, cmd, args)
    ↓
ContainerInspect("code-runner-go")   ← 每次都要查询
    ↓
ContainerExecCreate / ExecAttach      ← 串行排队等待同一个容器
    ↓
返回结果
```

每种语言固定对应一个容器（`code-runner-go`、`code-runner-python` 等）。并发请求进入同一容器后，Docker 的 exec 本身可以并行，但文件系统的 `/app/{uuid}/main.go` 挂载目录是共享的，高并发下 I/O 存在竞争，且 CPU 配额（`cpus: "1.0"`）限制了整体吞吐量。

### 核心瓶颈

| 问题 | 原因 |
|------|------|
| 单容器 CPU 上限为 1 核 | docker-compose 硬限制，无法动态扩展 |
| 所有请求共享同一容器的文件系统 | `/app` 目录挂载点相同，高并发 I/O 竞争 |
| 无备用容器 | 容器故障时服务中断，直到重启 |

---

## 二、设计目标

1. 每种语言维护 N 个容器，请求并行分配到不同容器
2. 接口层（`DockerContainer` 接口）不变，改动对上层透明
3. 容器故障时自动从池中摘除并异步补充，不影响在途请求
4. 池大小可配置，初期硬编码，后续接入热更新
5. 服务关闭时优雅关停，等待在途请求完成

---

## 三、改动范围

### 新增文件

```
internal/infrastructure/containerBasic/pool.go
internal/infrastructure/common/errors/container.go  — 容器相关 error types
```

### 修改文件

```
internal/infrastructure/containerBasic/container.go   — 接入池，适配 RunCode 路径
internal/infrastructure/containerBasic/runCode.go     — 宿主机路径改为使用 slot.HostPath
internal/infrastructure/config/initConfig.go          — 增加 pool_size 配置项
docker-compose/dev/client/docker-compose.yml          — 每种语言多个容器
docker-compose/product/client/docker-compose.yml
```

---

## 四、核心数据结构

### 新增 error types（container.go in errors/）

```go
package errors

import "errors"

var (
    ErrContainerExecTimeout  = errors.New("容器执行超时")
    ErrContainerPoolExhausted = errors.New("容器池资源耗尽")
    ErrContainerPoolClosed    = errors.New("容器池已关闭")
    ErrUnsupportedLanguage    = errors.New("不支持的语言")
)
```

### pool.go

```go
package containerBasic

import (
    "context"
    "fmt"
    "sync/atomic"

    "github.com/docker/docker/api/types/container"
    "github.com/docker/docker/client"
    "go.uber.org/zap"

    cErrors "codeRunner/internal/infrastructure/common/errors"
)

// ContainerSlot 表示池中一个容器的完整信息
type ContainerSlot struct {
    Name     string // 容器名，如 "code-runner-go-0"
    HostPath string // 宿主机挂载路径，如 "/app/tmp/golang-0"
}

// langPool 单语言容器池
type langPool struct {
    idle  chan ContainerSlot // 空闲容器槽位队列（buffered channel 天然实现 acquire/release）
    lang  string
    size  int               // 目标池大小
    image string            // 镜像名，补充容器时使用
}

// ContainerPool 管理所有语言的容器池
type ContainerPool struct {
    pools  map[string]*langPool // key: language
    cli    *client.Client       // Docker client，replenish 时使用
    closed atomic.Bool          // 关停标记
}

// newContainerPool 创建容器池
// images: 已有的 language → container image name 映射（复用 dockerContainerClient.images）
func newContainerPool(cli *client.Client, poolSizes map[string]int, images map[string]string) *ContainerPool {
    pools := make(map[string]*langPool, len(poolSizes))
    for lang, size := range poolSizes {
        lp := &langPool{
            idle:  make(chan ContainerSlot, size),
            lang:  lang,
            size:  size,
            image: images[lang],
        }
        for i := 0; i < size; i++ {
            lp.idle <- ContainerSlot{
                Name:     fmt.Sprintf("code-runner-%s-%d", lang, i),
                HostPath: fmt.Sprintf("/app/tmp/%s-%d", lang, i),
            }
        }
        pools[lang] = lp
    }
    return &ContainerPool{pools: pools, cli: cli}
}

// Acquire 从池中取出一个空闲容器，ctx 超时则返回错误
func (p *ContainerPool) Acquire(ctx context.Context, lang string) (ContainerSlot, error) {
    if p.closed.Load() {
        return ContainerSlot{}, cErrors.ErrContainerPoolClosed
    }
    lp, ok := p.pools[lang]
    if !ok {
        return ContainerSlot{}, fmt.Errorf("%w: %s", cErrors.ErrUnsupportedLanguage, lang)
    }
    select {
    case slot := <-lp.idle:
        return slot, nil
    case <-ctx.Done():
        return ContainerSlot{}, fmt.Errorf("%w: 语言=%s", cErrors.ErrContainerPoolExhausted, lang)
    }
}

// Release 将容器归还池中；若容器已损坏传入 healthy=false，则异步补充新容器
func (p *ContainerPool) Release(lang string, slot ContainerSlot, healthy bool) {
    lp, ok := p.pools[lang]
    if !ok {
        return
    }
    if healthy {
        lp.idle <- slot
        return
    }
    go p.replenish(lp, slot)
}

// Close 优雅关停：标记关闭，排空 channel，阻塞中的 Acquire 通过 ctx 超时退出
func (p *ContainerPool) Close() {
    p.closed.Store(true)
    for _, lp := range p.pools {
        close(lp.idle)
    }
}

// replenish 重建损坏的容器并重新放入池
func (p *ContainerPool) replenish(lp *langPool, slot ContainerSlot) {
    const maxRetries = 3
    ctx := context.Background()

    for attempt := 1; attempt <= maxRetries; attempt++ {
        // 1. 移除旧容器（force，无论什么状态）
        _ = p.cli.ContainerRemove(ctx, slot.Name, container.RemoveOptions{Force: true})

        // 2. 重新创建同名容器（复用 ensureContainerExists 中的创建逻辑）
        //    配置：image=lp.image, name=slot.Name, bind=slot.HostPath:/app
        //    安全限制：network_mode=none, cap_drop=ALL, mem_limit=512m, cpus=1.0
        err := createAndStartContainer(p.cli, ctx, slot.Name, lp.image, slot.HostPath)
        if err != nil {
            zap.S().Warnw("容器重建失败，准备重试",
                "container", slot.Name,
                "attempt", attempt,
                "err", err,
            )
            continue
        }

        zap.S().Infow("容器重建完成，重新加入池",
            "container", slot.Name,
            "attempt", attempt,
        )
        if !p.closed.Load() {
            lp.idle <- slot
        }
        return
    }

    // 所有重试失败：池永久缩小 1，记录告警
    zap.S().Errorw("容器重建失败，池永久缩小",
        "container", slot.Name,
        "language", lp.lang,
        "maxRetries", maxRetries,
    )
}

// createAndStartContainer 创建并启动容器（提取自 ensureContainerExists，供 replenish 复用）
func createAndStartContainer(cli *client.Client, ctx context.Context, name, image, hostPath string) error {
    // TODO: 实现——ContainerCreate + ContainerStart
    // 参数：image, name, Binds=[hostPath:/app], NetworkMode=none,
    //       CapDrop=[ALL], Memory=512MB, NanoCPUs=1e9, Cmd=["sleep","infinity"]
    return nil
}
```

### 关键设计决策

**为什么用 buffered channel 而不是 sync.Pool？**

`sync.Pool` 对象可以被 GC 回收，不适合管理有限的外部资源。buffered channel 的容量即池大小，`<-chan` 自然实现阻塞等待和背压。

**容器命名规则：`code-runner-{lang}-{index}`**

与 docker-compose 的 `container_name` 一一对应，`Acquire` 直接返回容器名，无需额外查询。

**Acquire 返回 `ContainerSlot` 而非单纯容器名（Review #1 修正）**

关键问题：`runCode.go` 中 `createFile()` 写入的宿主机路径为 `/app/tmp/{language}/{uuid}/`，但池化后每个容器挂载不同的宿主机目录（`/app/tmp/golang-0:/app`、`/app/tmp/golang-1:/app`）。如果 Acquire 只返回容器名，调用方无法知道文件应写入哪个宿主机目录，会导致文件写到错误位置、容器内找不到文件。

因此 `ContainerSlot` 同时携带 `Name` 和 `HostPath`，`RunCode` 使用 `slot.HostPath` 替代原来硬编码的 `/app/tmp/{language}` 路径。

#### images 映射复用，不引入 langToImage（Review #2 修正）

不新增包级变量 `langToImage`，而是在 `newContainerPool()` 时直接传入已有的 `dockerContainerClient.images` map，避免维护两份映射。

---

## 五、容器执行层改动（container.go + runCode.go）

### container.go 改动

`dockerContainerClient` 持有 `*ContainerPool`，`InContainerRunCode` 改为先 Acquire 再执行：

```go
type dockerContainerClient struct {
    ctx  context.Context
    cli  *client.Client
    pool *ContainerPool   // 新增
    // language, images 字段保留（images 在初始化时传给 pool）
}

func (c *dockerContainerClient) InContainerRunCode(language string, cmd string, args []string) (int64, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // 从池中获取空闲容器
    slot, err := c.pool.Acquire(ctx, language)
    if err != nil {
        return 0, "", err
    }

    // 查询容器 ID
    info, err := c.cli.ContainerInspect(ctx, slot.Name)
    if err != nil {
        zap.S().Errorw("ContainerInspect 失败", "container", slot.Name, "err", err)
        c.pool.Release(language, slot, false)
        return 0, "", err
    }

    start := time.Now()
    result, err := c.buildExec(ctx, cmd, info.ID, args)
    duration := time.Since(start).Milliseconds()

    if err != nil {
        // 使用 errors.Is 判断超时，不依赖字符串匹配（Review #5 修正）
        healthy := !errors.Is(err, cErrors.ErrContainerExecTimeout) &&
                   ctx.Err() != context.DeadlineExceeded
        c.pool.Release(language, slot, healthy)
        return 0, "", err
    }

    c.pool.Release(language, slot, true)
    return duration, result, nil
}
```

### runCode.go 改动（Review #1 关键修正）

`RunCode` 方法签名需接收 `slot ContainerSlot`，宿主机路径改为使用 `slot.HostPath`：

```go
// 改造前
func (r *runCode) RunCode(request *pb.CodeRequest) (int64, string, error) {
    uniqueID := uuid.New().String()
    path := fmt.Sprintf("/app/tmp/%s/%s", request.Language, uniqueID)  // 硬编码语言目录
    // ...
}

// 改造后
func (r *runCode) RunCode(request *pb.CodeRequest, slot ContainerSlot) (int64, string, error) {
    uniqueID := uuid.New().String()
    path := fmt.Sprintf("%s/%s", slot.HostPath, uniqueID)  // 使用池分配的宿主机路径
    // ...
    containerPath := fmt.Sprintf("/app/%s/main.%s", uniqueID, r.extension)  // 容器内路径不变
    // ...
}
```

**路径映射关系变化：**

```
改造前：
  宿主机 /app/tmp/golang/{uuid}/main.go  →  挂载 /tmp/golang:/app  →  容器内 /app/{uuid}/main.go

改造后：
  宿主机 /app/tmp/golang-0/{uuid}/main.go  →  挂载 /tmp/golang-0:/app  →  容器内 /app/{uuid}/main.go
  宿主机 /app/tmp/golang-1/{uuid}/main.go  →  挂载 /tmp/golang-1:/app  →  容器内 /app/{uuid}/main.go
```

### GetContains 方法适配（Review #4 修正）

`GetContains(language)` 原来返回固定容器名，池化后语义不再明确。排查调用方：

- 如果上层仅用于获取镜像名 → 重命名为 `GetImageName(language)` 返回 `c.images[language]`
- 如果上层用于获取容器名 → 应改为通过 `Acquire/Release` 流程获取
- 如果无实际调用方 → 废弃该方法

```go
// 适配方案：改为返回镜像名，明确语义
func (c *dockerContainerClient) GetImageName(language string) string {
    return c.images[language]
}
```

**变更点：**

- 原来 `c.images[language]` 直接拼出唯一容器名 → 改为从池中 Acquire，返回 `ContainerSlot`
- 执行完成后调用 `Release`，根据执行结果标记是否健康
- 超时判断改用 `errors.Is` + `ctx.Err()` 替代字符串匹配
- `RunCode` 使用 `slot.HostPath` 写入文件，不再硬编码语言目录
- `DockerContainer` 接口签名不变（`InContainerRunCode` 对外签名不变，`slot` 在内部传递）

---

## 六、docker-compose 改动

每种语言从 1 个容器扩展为 N 个，以 Go 为例（其他语言同理）：

```yaml
# 原来
code-runner-go:
  container_name: code-runner-go
  volumes:
    - /tmp/golang:/app

# 改为（N=2，dev 环境）
code-runner-go-0:
  image: code-runner-go:latest
  container_name: code-runner-go-0
  command: sleep infinity
  restart: unless-stopped
  network_mode: none
  cap_drop: [ALL]
  mem_limit: 512m
  memswap_limit: 512m
  cpus: "1.0"
  volumes:
    - /tmp/golang-0:/app   # ← 独立目录，避免文件系统竞争

code-runner-go-1:
  image: code-runner-go:latest
  container_name: code-runner-go-1
  command: sleep infinity
  restart: unless-stopped
  network_mode: none
  cap_drop: [ALL]
  mem_limit: 512m
  memswap_limit: 512m
  cpus: "1.0"
  volumes:
    - /tmp/golang-1:/app   # ← 独立目录
```

**每个容器独立挂载目录**是关键——`runCode.go` 中使用 `slot.HostPath` 写入对应目录，容器内路径 `/app/{uuid}/main.go` 在不同容器中互不干扰。

> **注意**：5 种语言 × N 个容器的 YAML 维护成本较高，考虑用脚本生成或 docker-compose 的 `deploy.replicas`（Swarm 模式）替代手写。

---

## 七、配置项（initConfig.go）

```go
type ClientConfig struct {
    // ... 已有字段
    ContainerPool ContainerPoolConfig `yaml:"container_pool"`
}

type ContainerPoolConfig struct {
    Golang     int `yaml:"golang"     default:"2"`
    Python     int `yaml:"python"     default:"2"`
    JavaScript int `yaml:"javascript" default:"2"`
    Java       int `yaml:"java"       default:"1"`
    C          int `yaml:"c"          default:"2"`
}
```

`NewDockerClient()` 从 config 读取池大小，传入 `newContainerPool()`。

---

## 八、初始化改动（container.go NewDockerClient + ensureContainerExists）

### NewDockerClient 签名改动

```go
func NewDockerClient(poolCfg ContainerPoolConfig) *dockerContainerClient {
    cli, _ := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

    images := map[string]string{
        "golang":     "code-runner-go",
        "java":       "code-runner-java",
        "c":          "code-runner-cpp",
        "python":     "code-runner-python",
        "javascript": "code-runner-js",
    }

    poolSizes := map[string]int{
        "golang":     poolCfg.Golang,
        "python":     poolCfg.Python,
        "javascript": poolCfg.JavaScript,
        "java":       poolCfg.Java,
        "c":          poolCfg.C,
    }

    pool := newContainerPool(cli, poolSizes, images)

    // 初始化时遍历所有池中容器做健康探测（Review #6 修正）
    for lang, lp := range pool.pools {
        for i := 0; i < lp.size; i++ {
            containerName := fmt.Sprintf("code-runner-%s-%d", lang, i)
            hostPath := fmt.Sprintf("/app/tmp/%s-%d", lang, i)
            ensureContainerExists(cli, containerName)   // 已有方法，检查并启动容器
            createContent(hostPath)                     // 创建宿主机挂载目录
        }
    }

    return &dockerContainerClient{
        cli:    cli,
        pool:   pool,
        images: images,
        // ...
    }
}
```

### ensureContainerExists 适配（Review #6）

原来检查 `code-runner-go` 一个容器，现在需要遍历 `code-runner-go-0`、`code-runner-go-1` 等所有池中容器。改动方式：`NewDockerClient` 中循环调用已有的 `ensureContainerExists`，传入新的容器名即可，**该方法本身不需要修改**。

### 调用方改动（initialize/service.go）

```go
dockerClient := docker.NewDockerClient(config.ClientCfg.ContainerPool)
```

---

## 九、优雅关停（Review #7 补充）

### 需求

- 服务关闭时，正在执行的请求需要等待完成
- Acquire 阻塞中的请求需要快速失败
- 不应在 `replenish` 中将容器放回已关闭的池

### 实现

在 `main.go` 或服务关停 hook 中调用 `pool.Close()`：

```go
// cmd/api/main.go 或 graceful shutdown handler
func shutdown() {
    // 1. 停止接受新请求（关闭 gRPC listener）
    grpcServer.GracefulStop()

    // 2. 关闭容器池（阻塞中的 Acquire 会因 channel 关闭而退出）
    dockerClient.Pool().Close()

    // 3. 等待在途请求完成（可用 sync.WaitGroup 或 context）
}
```

`Close()` 的行为：

- `closed` 标记为 true → 新的 `Acquire` 立即返回 `ErrContainerPoolClosed`
- `close(lp.idle)` → 阻塞在 `<-lp.idle` 的 goroutine 收到零值，需要在 `Acquire` 中检查零值
- `replenish` 中检查 `closed` 标记，不再归还容器

```go
func (p *ContainerPool) Acquire(ctx context.Context, lang string) (ContainerSlot, error) {
    if p.closed.Load() {
        return ContainerSlot{}, cErrors.ErrContainerPoolClosed
    }
    lp, ok := p.pools[lang]
    if !ok {
        return ContainerSlot{}, fmt.Errorf("%w: %s", cErrors.ErrUnsupportedLanguage, lang)
    }
    select {
    case slot, ok := <-lp.idle:
        if !ok {
            return ContainerSlot{}, cErrors.ErrContainerPoolClosed  // channel 已关闭
        }
        return slot, nil
    case <-ctx.Done():
        return ContainerSlot{}, fmt.Errorf("%w: 语言=%s", cErrors.ErrContainerPoolExhausted, lang)
    }
}
```

---

## 十、可观测性（Review #8 补充）

接入已有的 Prometheus 体系（`internal/infrastructure/common/metrics/`）：

```go
// metrics/container_pool.go
var (
    PoolIdleGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "container_pool_idle_count",
            Help: "当前空闲容器数",
        },
        []string{"language"},
    )
    PoolAcquireDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "container_pool_acquire_duration_seconds",
            Help:    "从池中获取容器的等待时间",
            Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 5},
        },
        []string{"language"},
    )
    PoolReplenishTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "container_pool_replenish_total",
            Help: "容器重建次数",
        },
        []string{"language", "result"},  // result: "success" / "failure"
    )
)
```

在 `Acquire` 和 `Release` 中埋点：

```go
func (p *ContainerPool) Acquire(ctx context.Context, lang string) (ContainerSlot, error) {
    start := time.Now()
    defer func() {
        metrics.PoolAcquireDuration.WithLabelValues(lang).Observe(time.Since(start).Seconds())
    }()
    // ...
}

func (p *ContainerPool) Release(lang string, slot ContainerSlot, healthy bool) {
    // ...
    metrics.PoolIdleGauge.WithLabelValues(lang).Set(float64(len(lp.idle)))
}
```

---

## 十一、执行流程对比

### 改造前

```
请求 A（golang） ──┐
请求 B（golang） ──┤──→ code-runner-go（单容器）──→ 串行执行
请求 C（golang） ──┘

宿主机路径：/app/tmp/golang/{uuid}/main.go → 挂载 /tmp/golang:/app
```

### 改造后（池大小 = 2）

```
请求 A（golang） ──→ Acquire → slot{code-runner-go-0, /app/tmp/golang-0} ──→ 并行执行
请求 B（golang） ──→ Acquire → slot{code-runner-go-1, /app/tmp/golang-1} ──→ 并行执行
请求 C（golang） ──→ Acquire → 阻塞等待（背压）
                              ↓ A 完成后 Release
                    Acquire → slot{code-runner-go-0, /app/tmp/golang-0} ──→ 继续执行

宿主机路径：/app/tmp/golang-{i}/{uuid}/main.go → 挂载 /tmp/golang-{i}:/app
```

---

## 十二、风险与注意事项

| 风险 | 应对 |
|------|------|
| `runCode.go` 宿主机路径需改为 `slot.HostPath` | **已纳入改动范围**，使用 `slot.HostPath/{uuid}` 替代 `/app/tmp/{language}/{uuid}` |
| `containerPath` 容器内路径 `/app/{uuid}` | 每个容器独立挂载目录，容器内路径不冲突，无需修改 |
| Acquire 超时（所有容器都在忙） | ctx 超时后返回 `ErrContainerPoolExhausted`，前置限流防护可减少此类情况 |
| 容器重建失败导致池永久缩小 | `replenish` 最多重试 3 次，失败后日志告警，后续接入通知 webhook |
| 容器重建期间池暂时缩小 | `replenish` 是异步的，池大小暂时减 1，可接受 |
| dev/product 两套 docker-compose 都需要修改 | 同步修改，pool_size 配置分别设小（dev=1）和大（prod=2+） |
| 服务关闭时在途请求 | `Close()` 标记关闭 + `GracefulStop`，在途请求自然完成后退出 |
| `GetContains` 语义不明 | 重命名为 `GetImageName`，返回镜像名而非容器名 |

---

## 十三、实施步骤

1. **新增 error types**：`internal/infrastructure/common/errors/container.go`，定义容器池相关错误
2. **新增 `pool.go`**：实现 `ContainerSlot`、`langPool`、`ContainerPool`、`Acquire`、`Release`、`Close`、`replenish`（不修改任何已有文件）
3. **修改 `container.go`**：
   - `dockerContainerClient` 加 `pool` 字段
   - 改造 `InContainerRunCode`，使用 `slot` 替代固定容器名
   - 改造 `NewDockerClient`，遍历池中所有容器做 `ensureContainerExists`
   - 将 `GetContains` 重命名为 `GetImageName`（或废弃，视调用方情况）
4. **修改 `runCode.go`**：`RunCode` 接收 `ContainerSlot`，宿主机路径改为 `slot.HostPath`
5. **修改 `initConfig.go`**：加 `ContainerPoolConfig` 结构体
6. **修改 `initialize/service.go`**：传入 pool 配置
7. **新增 metrics**：`container_pool_idle_count`、`container_pool_acquire_duration`、`container_pool_replenish_total`
8. **修改两套 docker-compose**：每语言扩为 N 个容器，独立挂载目录
9. **补充优雅关停**：main.go shutdown handler 调用 `pool.Close()`
10. **验证**：`go build ./...` + 并发压测对比（单容器 vs 池）

---

## 附录：Review 修正追踪

| # | 问题 | 状态 | 修正位置 |
| --- | ------ | ------ | ---------- |
| 1 | 宿主机路径映射不一致（关键） | ✅ 已修正 | §四 ContainerSlot、§五 runCode.go 改动 |
| 2 | langToImage 未定义 | ✅ 已修正 | §四 newContainerPool 传入 images map |
| 3 | replenish 实现缺失 | ✅ 已补充 | §四 replenish 方法（重试 + 告警） |
| 4 | GetContains 语义不明 | ✅ 已修正 | §五 重命名为 GetImageName |
| 5 | 超时判断字符串匹配 | ✅ 已修正 | §四 error types + §五 errors.Is |
| 6 | ensureContainerExists 需适配 | ✅ 已修正 | §八 遍历池中所有容器 |
| 7 | 缺少优雅关停 | ✅ 已补充 | §九 Close() + shutdown handler |
| 8 | 缺少可观测性 | ✅ 已补充 | §十 Prometheus metrics |
