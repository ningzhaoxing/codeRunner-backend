---
title: 容器池技术方案
type: design
status: draft
created: 2026-03-27
updated: 2026-04-02
related:
  - docs/context/designs/architecture-roadmap.md
---

# 容器池技术方案

> 状态：待实现  
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

---

## 三、改动范围

### 新增文件

```
internal/infrastructure/containerBasic/pool.go
```

### 修改文件

```
internal/infrastructure/containerBasic/container.go   — 接入池
internal/infrastructure/config/initConfig.go          — 增加 pool_size 配置项
docker-compose/dev/client/docker-compose.yml          — 每种语言多个容器
docker-compose/product/client/docker-compose.yml
```

---

## 四、核心数据结构

### pool.go

```go
package containerBasic

import (
    "context"
    "fmt"
    "sync"
    "time"

    "go.uber.org/zap"
)

// 单语言容器池
type langPool struct {
    idle   chan string   // 空闲容器名队列（buffered channel 天然实现 acquire/release）
    lang   string
    mu     sync.Mutex
    size   int          // 目标池大小（用于故障补充时判断）
    image  string       // 镜像名，补充容器时使用
}

// ContainerPool 管理所有语��的容器池
type ContainerPool struct {
    pools map[string]*langPool  // key: language
}

func newContainerPool(poolSizes map[string]int) *ContainerPool {
    pools := make(map[string]*langPool, len(poolSizes))
    for lang, size := range poolSizes {
        lp := &langPool{
            idle:  make(chan string, size),
            lang:  lang,
            size:  size,
            image: langToImage[lang],
        }
        // 按 "code-runner-{lang}-{i}" 命名规则预填充
        for i := 0; i < size; i++ {
            lp.idle <- fmt.Sprintf("code-runner-%s-%d", lang, i)
        }
        pools[lang] = lp
    }
    return &ContainerPool{pools: pools}
}

// Acquire 从池中取出一个空闲容器名，ctx 超时则返回错误
func (p *ContainerPool) Acquire(ctx context.Context, lang string) (string, error) {
    lp, ok := p.pools[lang]
    if !ok {
        return "", fmt.Errorf("不支持的语言: %s", lang)
    }
    select {
    case name := <-lp.idle:
        return name, nil
    case <-ctx.Done():
        return "", fmt.Errorf("容器池等待超时，语言=%s", lang)
    }
}

// Release 将容器归还池中；若容器已损坏传入 healthy=false，则异步补充新容器
func (p *ContainerPool) Release(lang, containerName string, healthy bool) {
    lp, ok := p.pools[lang]
    if !ok {
        return
    }
    if healthy {
        lp.idle <- containerName
        return
    }
    // 容器不健康：不归还，异步重建后再放入
    go p.replenish(lp, containerName)
}

// replenish 重建损坏的容器并重新放入池（实现略，调用 docker SDK 启动同名容器）
func (p *ContainerPool) replenish(lp *langPool, containerName string) {
    // TODO: 调用 docker SDK 重启/重建 containerName
    // 重建成功后：
    zap.S().Infow("容器重建完成，重新加入池", "container", containerName)
    lp.idle <- containerName
}
```

### 关键设计决策

**为什么用 buffered channel 而不是 sync.Pool？**

`sync.Pool` 对象可以被 GC 回收，不适合管理有限的外部资源。buffered channel 的容量即池大小，`<-chan` 自然实现阻塞等待和背压。

**容器命名规则：`code-runner-{lang}-{index}`**

与 docker-compose 的 `container_name` 一一对应，`Acquire` 直接返回容器名，无需额外查询。

---

## 五、容器执行层改动（container.go）

`dockerContainerClient` 持有 `*ContainerPool`，`InContainerRunCode` 改为先 Acquire 再执行：

```go
type dockerContainerClient struct {
    ctx  context.Context
    cli  *client.Client
    pool *ContainerPool   // 新增
    // ... 其余字段不变
}

func (c *dockerContainerClient) InContainerRunCode(language string, cmd string, args []string) (int64, string, error) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    // 从池中获取空闲容器
    containerName, err := c.pool.Acquire(ctx, language)
    if err != nil {
        return 0, "", err
    }

    // 查询容器 ID
    info, err := c.cli.ContainerInspect(ctx, containerName)
    healthy := true
    if err != nil {
        zap.S().Errorw("ContainerInspect 失败", "container", containerName, "err", err)
        c.pool.Release(language, containerName, false)
        return 0, "", err
    }

    start := time.Now()
    result, err := c.buildExec(ctx, cmd, info.ID, args)
    duration := time.Since(start).Milliseconds()

    if err != nil {
        // exec 失败不一定代表容器损坏，区分超时和其他错误
        if err.Error() == "命令执行超时" {
            healthy = false  // 超时的容器可能卡住，标记不健康
        }
        c.pool.Release(language, containerName, healthy)
        return 0, "", err
    }

    c.pool.Release(language, containerName, true)
    return duration, result, nil
}
```

**变更点：**
- 原来 `c.images[language]` 直接拼出唯一容器名 → 改为从池中 Acquire
- 执行完成后调用 `Release`，根据执行结果标记是否健康
- `DockerContainer` 接口签名不变，上层无需修改

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

**每个容器独立挂载目录**是关键——`runCode.go` 中生成的 `/app/{uuid}/main.go` 路径在不同容器中互不干扰。

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

## 八、初始化改动（initialize/service.go）

```go
// 改动点：NewDockerClient 接收池大小配置
dockerClient := docker.NewDockerClient(config.ClientCfg.ContainerPool)
```

`NewDockerClient` 签名改为：

```go
func NewDockerClient(poolCfg ContainerPoolConfig) *dockerContainerClient {
    poolSizes := map[string]int{
        "golang":     poolCfg.Golang,
        "python":     poolCfg.Python,
        "javascript": poolCfg.JavaScript,
        "java":       poolCfg.Java,
        "c":          poolCfg.C,
    }
    pool := newContainerPool(poolSizes)
    // ... 其余初始化不变
    return &dockerContainerClient{pool: pool, ...}
}
```

---

## 九、执行流程对比

### 改造前

```
请求 A（golang） ──┐
请求 B（golang） ──┤──→ code-runner-go（单容器）──→ 串行执行
请求 C（golang） ──┘
```

### 改造后（池大小 = 2）

```
请求 A（golang） ──→ Acquire → code-runner-go-0 ──→ 并行执行
请求 B（golang） ──→ Acquire → code-runner-go-1 ──→ 并行执行
请求 C（golang） ──→ Acquire → 阻塞等待（背压）
                              ↓ A 完成后 Release
                    Acquire → code-runner-go-0 ──→ 继续执行
```

---

## 十、风险与注意事项

| 风险 | 应对 |
|------|------|
| `runCode.go` 的 `containerPath` 拼接使用 `/app/{uuid}` | 每个容器独立挂载目录，路径不冲突，无需修改 |
| Acquire 超时（所有容器都在忙） | ctx 超时后返回 `ResourceExhausted` 错误，前置限流防护（§5 限流方案）可减少此类情况 |
| 容器重建期间池暂时缩小 | `replenish` 是异步的，池大小暂时减 1，可接受 |
| dev/product 两套 docker-compose 都需要修改 | 同步修改，pool_size 配置分别设小（dev=1）和大（prod=2+） |

---

## 十一、实施步骤

1. **新增 `pool.go`**：实现 `langPool`、`ContainerPool`、`Acquire`、`Release`（不修改任何已有文件）
2. **修改 `container.go`**：`dockerContainerClient` 加 `pool` 字段，改造 `InContainerRunCode`
3. **修改 `initConfig.go`**：加 `ContainerPoolConfig` 结构体
4. **修改 `initialize/service.go`**：传入 pool 配置
5. **修改两套 docker-compose**：每语言扩为 N 个容器，独立挂载目录
6. **验证**：`go build ./...` + 并发压测对比（单容器 vs 池）
