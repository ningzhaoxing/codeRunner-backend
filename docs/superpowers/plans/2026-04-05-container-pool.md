# 容器池实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现容器池化，每种语言维护 N 个容器实例，请求并行分配到不同容器，提升并发吞吐量。

**Architecture:** 在 `containerBasic` 包新增 `pool.go`，用 buffered channel 管理 `ContainerSlot`（容器名 + 宿主机路径）。`DockerContainer` 接口新增 `AcquireSlot`/`ReleaseSlot` 方法，`runCode.RunCode()` 在创建文件前获取 slot，使用 `slot.HostPath` 写入文件，执行后归还。`Container` 接口（暴露给 domain 层）签名不变。

**Tech Stack:** Go 1.23, Docker SDK, buffered channel, Prometheus, Zap

**Design Doc:** `docs/context/designs/container-pool-design.md`

---

## File Structure

### New Files
| File | Responsibility |
|------|----------------|
| `internal/infrastructure/common/errors/container.go` | 容器池相关 error types |
| `internal/infrastructure/containerBasic/pool.go` | ContainerSlot、ContainerPool、Acquire/Release/Close/replenish |
| `internal/infrastructure/containerBasic/pool_test.go` | 池的单元测试（不依赖 Docker） |

### Modified Files
| File | Changes |
|------|---------|
| `internal/infrastructure/containerBasic/container.go` | DockerContainer 接口加 AcquireSlot/ReleaseSlot；dockerContainerClient 加 pool 字段；NewDockerClient 接受 pool config；InContainerRunCode 改为接收 containerName；ensureContainerExists 遍历所有池容器；移除 GetContains 加 GetImageName |
| `internal/infrastructure/containerBasic/runCode.go` | RunCode 中 Acquire→createFile(slot.HostPath)→exec→Release；runCodeContainer 传 slot.Name |
| `internal/infrastructure/config/initConfig.go` | ClientConfig 加 ContainerPoolConfig |
| `internal/interfaces/adapter/initialize/service.go` | NewDockerClient 传入 pool config |
| `internal/infrastructure/metrics/metrics.go` | 新增 pool 相关 metrics |
| `configs/dev.yaml` | 加 container_pool 配置段 |
| `configs/product.yaml` | 加 container_pool 配置段 |
| `docker-compose/dev/client/docker-compose.yml` | 每语言 1 容器（dev 退化为池大小=1） |
| `docker-compose/product/client/docker-compose.yml` | 每语言 2 容器 |

---

## Task 1: 新增 error types

**Files:**
- Create: `internal/infrastructure/common/errors/container.go`

- [ ] **Step 1: 创建 container.go**

```go
package errors

import "errors"

var (
	ErrContainerExecTimeout   = errors.New("容器执行超时")
	ErrContainerPoolExhausted = errors.New("容器池资源耗尽")
	ErrContainerPoolClosed    = errors.New("容器池已关闭")
	ErrUnsupportedLanguage    = errors.New("不支持的语言")
)
```

- [ ] **Step 2: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/common/errors/...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Commit**

```bash
git add internal/infrastructure/common/errors/container.go
git commit -m "feat: add container pool error types"
```

---

## Task 2: 新增 pool.go 核心实现

**Files:**
- Create: `internal/infrastructure/containerBasic/pool.go`

- [ ] **Step 1: 创建 pool.go — ContainerSlot 和 langPool 类型**

```go
package containerBasic

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"go.uber.org/zap"

	cErrors "codeRunner-siwu/internal/infrastructure/common/errors"
)

// ContainerSlot 表示池中一个容器的完整信息
type ContainerSlot struct {
	Name     string // 容器名，如 "code-runner-go-0"
	HostPath string // 宿主机挂载路径，如 "/app/tmp/golang-0"
}

// langPool 单语言容器池
type langPool struct {
	idle  chan ContainerSlot
	lang  string
	size  int
	image string
}

// ContainerPool 管理所有语言的容器池
type ContainerPool struct {
	pools  map[string]*langPool
	cli    *client.Client
	closed atomic.Bool
}
```

- [ ] **Step 2: 实现 NewContainerPool**

```go
// langHostDir 语言 → 宿主机挂载目录名映射（与 docker-compose volumes 保持一致）
var langHostDir = map[string]string{
	"golang":     "golang",
	"python":     "python",
	"javascript": "javascript",
	"java":       "java",
	"c":          "c",
}

// newContainerPool 创建容器池（包内使用，不导出）
// images: language → container name prefix 映射（如 "golang" → "code-runner-go"）
func newContainerPool(cli *client.Client, poolSizes map[string]int, images map[string]string) *ContainerPool {
	pools := make(map[string]*langPool, len(poolSizes))
	for lang, size := range poolSizes {
		imgPrefix := images[lang] // e.g. "code-runner-go"
		hostDir := langHostDir[lang] // e.g. "golang"
		lp := &langPool{
			idle:  make(chan ContainerSlot, size),
			lang:  lang,
			size:  size,
			image: imgPrefix,
		}
		for i := 0; i < size; i++ {
			lp.idle <- ContainerSlot{
				Name:     fmt.Sprintf("%s-%d", imgPrefix, i),   // "code-runner-go-0" — 与 docker-compose container_name 一致
				HostPath: fmt.Sprintf("/app/tmp/%s-%d", hostDir, i), // "/app/tmp/golang-0" — 与 docker-compose volumes 一致
			}
		}
		pools[lang] = lp
	}
	return &ContainerPool{pools: pools, cli: cli}
}
```

- [ ] **Step 3: 实现 Acquire**

```go
// Acquire 从池中取出一个空闲容器
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
			return ContainerSlot{}, cErrors.ErrContainerPoolClosed
		}
		return slot, nil
	case <-ctx.Done():
		return ContainerSlot{}, fmt.Errorf("%w: 语言=%s", cErrors.ErrContainerPoolExhausted, lang)
	}
}
```

- [ ] **Step 4: 实现 Release**

```go
// Release 将容器归还池中
func (p *ContainerPool) Release(lang string, slot ContainerSlot, healthy bool) {
	if p.closed.Load() {
		return // 池已关闭，不归还
	}
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
```

- [ ] **Step 5: 实现 Close**

```go
// Close 优雅关停
func (p *ContainerPool) Close() {
	p.closed.Store(true)
	for _, lp := range p.pools {
		close(lp.idle)
	}
}
```

- [ ] **Step 6: 实现 replenish**

```go
// replenish 重建损坏的容器并重新放入池
func (p *ContainerPool) replenish(lp *langPool, slot ContainerSlot) {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx := context.Background()

		// 重启容器（docker-compose 管理的容器支持 restart）
		// 先 stop 再 start，比 remove+create 安全——不丢失 docker-compose 的配置
		timeout := 5 // seconds
		_ = p.cli.ContainerStop(ctx, slot.Name, container.StopOptions{Timeout: &timeout})
		err := p.cli.ContainerStart(ctx, slot.Name, container.StartOptions{})
		if err != nil {
			zap.S().Warnw("容器重启失败，准备重试",
				"container", slot.Name,
				"attempt", attempt,
				"err", err,
			)
			continue
		}

		zap.S().Infow("容器重启完成，重新加入池",
			"container", slot.Name,
			"attempt", attempt,
		)
		if !p.closed.Load() {
			lp.idle <- slot
		}
		return
	}

	zap.S().Errorw("容器重建失败，池永久缩小",
		"container", slot.Name,
		"language", lp.lang,
		"maxRetries", maxRetries,
	)
}

// IdleCount 返回指定语言的空闲容器数（用于 metrics）
func (p *ContainerPool) IdleCount(lang string) int {
	lp, ok := p.pools[lang]
	if !ok {
		return 0
	}
	return len(lp.idle)
}

// Languages 返回所有支持的语言列表
func (p *ContainerPool) Languages() []string {
	langs := make([]string, 0, len(p.pools))
	for lang := range p.pools {
		langs = append(langs, lang)
	}
	return langs
}
```

- [ ] **Step 7: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/containerBasic/...`
Expected: BUILD SUCCESS

- [ ] **Step 8: Commit**

```bash
git add internal/infrastructure/containerBasic/pool.go
git commit -m "feat: add container pool with Acquire/Release/Close/replenish"
```

---

## Task 3: pool_test.go 单元测试

**Files:**
- Create: `internal/infrastructure/containerBasic/pool_test.go`

- [ ] **Step 1: 写 Acquire/Release 基本流程测试**

```go
package containerBasic

import (
	"context"
	"testing"
	"time"
)

func newTestPool(lang string, size int) *ContainerPool {
	images := map[string]string{lang: "test-image"}
	poolSizes := map[string]int{lang: size}
	// cli=nil，测试不涉及 Docker 操作（同包可访问 unexported 函数）
	return newContainerPool(nil, poolSizes, images)
}

func TestAcquireRelease(t *testing.T) {
	pool := newTestPool("golang", 2)
	defer pool.Close()

	ctx := context.Background()

	// Acquire 两个 slot
	slot1, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire slot1 failed: %v", err)
	}
	slot2, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire slot2 failed: %v", err)
	}

	// slot 名称应不同
	if slot1.Name == slot2.Name {
		t.Errorf("expected different slots, got same: %s", slot1.Name)
	}

	// HostPath 应包含语言名
	if slot1.HostPath == "" || slot2.HostPath == "" {
		t.Error("expected non-empty HostPath")
	}

	// Release 后可以再次 Acquire
	pool.Release("golang", slot1, true)
	slot3, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("Acquire after Release failed: %v", err)
	}
	if slot3.Name != slot1.Name {
		t.Errorf("expected to get back slot1 (%s), got %s", slot1.Name, slot3.Name)
	}
}
```

- [ ] **Step 2: 运行测试验证通过**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go test ./internal/infrastructure/containerBasic/ -run TestAcquireRelease -v`
Expected: PASS

- [ ] **Step 3: 写 Acquire 超时测试**

```go
func TestAcquireTimeout(t *testing.T) {
	pool := newTestPool("golang", 1)
	defer pool.Close()

	ctx := context.Background()

	// 取走唯一的 slot
	_, err := pool.Acquire(ctx, "golang")
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}

	// 再次 Acquire 应超时
	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err = pool.Acquire(timeoutCtx, "golang")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go test ./internal/infrastructure/containerBasic/ -run TestAcquireTimeout -v`
Expected: PASS

- [ ] **Step 5: 写 UnsupportedLanguage 和 PoolClosed 测试**

```go
func TestAcquireUnsupportedLanguage(t *testing.T) {
	pool := newTestPool("golang", 1)
	defer pool.Close()

	_, err := pool.Acquire(context.Background(), "rust")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestAcquireAfterClose(t *testing.T) {
	pool := newTestPool("golang", 1)
	pool.Close()

	_, err := pool.Acquire(context.Background(), "golang")
	if err == nil {
		t.Fatal("expected error after Close")
	}
}
```

- [ ] **Step 6: 运行全部池测试**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go test ./internal/infrastructure/containerBasic/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/infrastructure/containerBasic/pool_test.go
git commit -m "test: add container pool unit tests"
```

---

## Task 4: 新增 Prometheus metrics

**Files:**
- Modify: `internal/infrastructure/metrics/metrics.go`

- [ ] **Step 1: 在 metrics.go 中追加 pool metrics 变量**

在现有 `var (` 块中追加：

```go
	// PoolIdleGauge 各语言容器池空闲容器数
	PoolIdleGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "container_pool_idle_count",
			Help: "Number of idle containers in pool, labeled by language.",
		},
		[]string{"language"},
	)

	// PoolAcquireDuration 获取容器的等待时间
	PoolAcquireDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "container_pool_acquire_duration_seconds",
			Help:    "Duration of acquiring a container from pool.",
			Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 5},
		},
		[]string{"language"},
	)

	// PoolReplenishTotal 容器重建计数
	PoolReplenishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "container_pool_replenish_total",
			Help: "Total container replenish attempts, labeled by language and result.",
		},
		[]string{"language", "result"},
	)
```

- [ ] **Step 2: 在 init() 的 MustRegister 中追加注册**

```go
func init() {
	prometheus.MustRegister(
		CodeExecutionTotal,
		CodeExecutionDuration,
		WSClientsConnected,
		PoolIdleGauge,
		PoolAcquireDuration,
		PoolReplenishTotal,
	)
}
```

- [ ] **Step 3: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/metrics/...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/infrastructure/metrics/metrics.go
git commit -m "feat: add container pool Prometheus metrics"
```

---

## Task 5: pool.go 接入 metrics 埋点

**Files:**
- Modify: `internal/infrastructure/containerBasic/pool.go`

- [ ] **Step 1: Acquire 中添加耗时埋点**

在 `pool.go` 的 import 中追加 `"time"` 和 `"codeRunner-siwu/internal/infrastructure/metrics"`。

修改 `Acquire` 方法，在开头添加计时：

```go
func (p *ContainerPool) Acquire(ctx context.Context, lang string) (ContainerSlot, error) {
	start := time.Now()
	defer func() {
		metrics.PoolAcquireDuration.WithLabelValues(lang).Observe(time.Since(start).Seconds())
	}()

	// ... 原有逻辑不变
}
```

- [ ] **Step 2: Release 中更新 idle gauge**

在 `Release` 方法末尾（healthy 归还之后和 go replenish 之后）都添加：

```go
func (p *ContainerPool) Release(lang string, slot ContainerSlot, healthy bool) {
	if p.closed.Load() {
		return
	}
	lp, ok := p.pools[lang]
	if !ok {
		return
	}
	if healthy {
		lp.idle <- slot
		metrics.PoolIdleGauge.WithLabelValues(lang).Set(float64(len(lp.idle)))
		return
	}
	go p.replenish(lp, slot)
	metrics.PoolIdleGauge.WithLabelValues(lang).Set(float64(len(lp.idle)))
}
```

- [ ] **Step 3: replenish 中记录成功/失败计数**

在 replenish 成功时添加：
```go
metrics.PoolReplenishTotal.WithLabelValues(lp.lang, "success").Inc()
```

在所有重试失败时添加：
```go
metrics.PoolReplenishTotal.WithLabelValues(lp.lang, "failure").Inc()
```

- [ ] **Step 4: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/containerBasic/...`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/containerBasic/pool.go
git commit -m "feat: add metrics instrumentation to container pool"
```

---

## Task 6: 修改 initConfig.go 添加 ContainerPoolConfig

**Files:**
- Modify: `internal/infrastructure/config/initConfig.go`

- [ ] **Step 1: 添加 ContainerPoolConfig 结构体**

在 `ClientConfig` 结构体下方新增：

```go
type ContainerPoolConfig struct {
	Golang     int `yaml:"golang"`
	Python     int `yaml:"python"`
	JavaScript int `yaml:"javascript"`
	Java       int `yaml:"java"`
	C          int `yaml:"c"`
}
```

- [ ] **Step 2: ClientConfig 加 ContainerPool 字段**

```go
type ClientConfig struct {
	Server struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
		Path string `yaml:"path"`
	} `yaml:"server"`
	App struct {
		Weight int64 `yaml:"weight"`
	} `yaml:"app"`
	ContainerPool ContainerPoolConfig `yaml:"container_pool"`
}
```

- [ ] **Step 3: 添加 ToPoolSizes 便捷方法**

```go
// ToPoolSizes 转换为 map[string]int，供 NewContainerPool 使用
func (c ContainerPoolConfig) ToPoolSizes() map[string]int {
	m := map[string]int{
		"golang":     c.Golang,
		"python":     c.Python,
		"javascript": c.JavaScript,
		"java":       c.Java,
		"c":          c.C,
	}
	// 默认值：未配置时每种语言 1 个容器（退化为当前行为）
	for k, v := range m {
		if v <= 0 {
			m[k] = 1
		}
	}
	return m
}
```

- [ ] **Step 4: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/config/...`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/config/initConfig.go
git commit -m "feat: add ContainerPoolConfig to client config"
```

---

## Task 7: 修改 configs YAML 文件

**Files:**
- Modify: `configs/dev.yaml`
- Modify: `configs/product.yaml`

- [ ] **Step 1: dev.yaml 加 container_pool 段（每语言 1 个，退化为当前行为）**

在 `client:` 段末尾追加：

```yaml
  container_pool:
    golang: 1
    python: 1
    javascript: 1
    java: 1
    c: 1
```

- [ ] **Step 2: product.yaml 加 container_pool 段（每语言 2 个）**

在 `client:` 段末尾追加：

```yaml
  container_pool:
    golang: 2
    python: 2
    javascript: 2
    java: 1
    c: 2
```

- [ ] **Step 3: Commit**

```bash
git add configs/dev.yaml configs/product.yaml
git commit -m "feat: add container pool size config for dev and prod"
```

---

## Task 8: 修改 container.go — 接口和实现

**Files:**
- Modify: `internal/infrastructure/containerBasic/container.go`

这是核心改动，分多步完成。

- [ ] **Step 1: 修改 DockerContainer 接口**

将：
```go
type DockerContainer interface {
	InContainerRunCode(language string, cmd string, args []string) (int64, string, error)
	GetContains(language string) (container string)
}
```

改为：
```go
type DockerContainer interface {
	InContainerRunCode(containerName string, cmd string, args []string) (int64, string, error)
	AcquireSlot(ctx context.Context, language string) (ContainerSlot, error)
	ReleaseSlot(language string, slot ContainerSlot, healthy bool)
}
```

- [ ] **Step 2: 修改 dockerContainerClient 结构体，加 pool 字段**

```go
type dockerContainerClient struct {
	ctx      context.Context
	cli      *client.Client
	err      error
	language []string
	images   map[string]string
	pool     *ContainerPool
}
```

- [ ] **Step 3: 实现 AcquireSlot 和 ReleaseSlot**

```go
func (c *dockerContainerClient) AcquireSlot(ctx context.Context, language string) (ContainerSlot, error) {
	return c.pool.Acquire(ctx, language)
}

func (c *dockerContainerClient) ReleaseSlot(language string, slot ContainerSlot, healthy bool) {
	c.pool.Release(language, slot, healthy)
}
```

- [ ] **Step 4: 修改 InContainerRunCode — 参数从 language 改为 containerName**

```go
func (c *dockerContainerClient) InContainerRunCode(containerName string, cmd string, args []string) (int64, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	containerOne, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		zap.S().Error("容器ID未找到 err=", err)
		return 0, "", err
	}

	start := time.Now()
	result, err := c.buildExec(ctx, cmd, containerOne.ID, args)
	elapsed := time.Since(start)
	duration := elapsed.Milliseconds()

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return 0, "", cErrors.ErrContainerExecTimeout
		}
		return 0, "", fmt.Errorf("内网服务器出错")
	}
	return duration, result, nil
}
```

注意：import 中需要添加 `cErrors "codeRunner-siwu/internal/infrastructure/common/errors"`。

- [ ] **Step 5: 修改 NewDockerClient — 接受 pool config 参数**

```go
func NewDockerClient(poolCfg config.ContainerPoolConfig) *dockerContainerClient {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		panic("docker客户端创建失败" + err.Error())
		return nil
	}

	language := []string{"golang", "c", "java", "python", "javascript"}
	images := map[string]string{
		"golang":     "code-runner-go",
		"java":       "code-runner-java",
		"c":          "code-runner-cpp",
		"python":     "code-runner-python",
		"javascript": "code-runner-js",
	}

	poolSizes := poolCfg.ToPoolSizes()
	pool := newContainerPool(cli, poolSizes, images)

	dockerClient := &dockerContainerClient{
		err:      nil,
		language: language,
		images:   images,
		cli:      cli,
		ctx:      context.Background(),
		pool:     pool,
	}

	// 为所有池中容器创建目录并检查存在性
	for _, lang := range language {
		size := poolSizes[lang]
		imgPrefix := images[lang]       // e.g. "code-runner-go"
		hostDir := langHostDir[lang]    // e.g. "golang"
		for i := 0; i < size; i++ {
			containerName := fmt.Sprintf("%s-%d", imgPrefix, i)    // "code-runner-go-0"
			hostPath := fmt.Sprintf("/app/tmp/%s-%d", hostDir, i)  // "/app/tmp/golang-0"
			if err := os.MkdirAll(hostPath, 0755); err != nil {
				zap.S().Errorf("创建目录 %s 失败: %v", hostPath, err)
			}
			dockerClient.ensureContainerExistsByName(containerName)
		}
	}

	return dockerClient
}
```

- [ ] **Step 6: 添加 ensureContainerExistsByName 方法（按容器名检查）**

```go
func (c *dockerContainerClient) ensureContainerExistsByName(containerName string) {
	args := filters.NewArgs()
	args.Add("name", containerName)

	containers, err := c.cli.ContainerList(c.ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		zap.S().Errorf("检查容器 %s 失败: %v", containerName, err)
		return
	}
	if len(containers) == 0 {
		zap.S().Warnf("容器 %s 不存在，请先执行 docker-compose up", containerName)
		return
	}
	if containers[0].State != "running" {
		zap.S().Infof("容器 %s 未运行，正在启动...", containerName)
		if err := c.cli.ContainerStart(c.ctx, containers[0].ID, container.StartOptions{}); err != nil {
			zap.S().Errorf("启动容器 %s 失败: %v", containerName, err)
			return
		}
		zap.S().Infof("容器 %s 已启动", containerName)
	}
}
```

- [ ] **Step 7: 删除旧的 GetContains、createContent、ensureContainerExists 方法**

这三个方法被新逻辑替代：
- `GetContains` → 已从接口移除，无调用方
- `createContent` → 在 NewDockerClient 中改为按池遍历创建
- `ensureContainerExists(language)` → 改为 `ensureContainerExistsByName(containerName)`

- [ ] **Step 8: 添加 Pool() 方法用于外部访问（graceful shutdown 需要）**

```go
// Pool 返回容器池引用，供 graceful shutdown 调用
func (c *dockerContainerClient) Pool() *ContainerPool {
	return c.pool
}
```

- [ ] **Step 9: 确保 import 正确**

container.go 的 import 应为：
```go
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"go.uber.org/zap"

	cErrors "codeRunner-siwu/internal/infrastructure/common/errors"
	"codeRunner-siwu/internal/infrastructure/config"
)
```

- [ ] **Step 10: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/containerBasic/...`
Expected: 可能会因 runCode.go 中调用方式未改而编译失败，这是预期的，在 Task 9 修复。

- [ ] **Step 11: Commit**

```bash
git add internal/infrastructure/containerBasic/container.go
git commit -m "feat: integrate container pool into DockerContainer with AcquireSlot/ReleaseSlot"
```

---

## Task 9: 修改 runCode.go — 接入池化路径

**Files:**
- Modify: `internal/infrastructure/containerBasic/runCode.go`

- [ ] **Step 1: 修改 RunCode 方法 — Acquire slot，使用 slot.HostPath**

将 `RunCode` 方法改为：

```go
func (r *runCode) RunCode(request *proto.ExecuteRequest) (duration int64, response proto.ExecuteResponse, err error) {
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl

	// 从池中获取容器 slot
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	slot, err := r.AcquireSlot(ctx, request.Language)
	if err != nil {
		zap.S().Error("containerBasic-RunCode-AcquireSlot err=", err)
		return 0, response, err
	}
	healthy := true
	defer func() {
		r.ReleaseSlot(request.Language, slot, healthy)
	}()

	// 使用 slot.HostPath 创建文件
	uniqueID := uuid.New().String()
	path := fmt.Sprintf("%s/%s", slot.HostPath, uniqueID)
	err = r.createFile(request.Language, request.CodeBlock, path)
	if err != nil {
		zap.S().Error("containerBasic-RunCode-createFile err=", err)
		return 0, response, err
	}
	defer func() {
		r.file.Close()
		if removeErr := os.RemoveAll(r.path); removeErr != nil {
			zap.S().Error("删除文件夹失败,err=", removeErr)
		}
	}()

	// 构建容器内路径（不变）
	containerPath := fmt.Sprintf("/app/%s/main.%s", uniqueID, r.extension)
	// 使用 slot.Name 执行
	duration, response.Result, err = r.runCodeContainer(request.Language, containerPath, slot)

	// Prometheus 指标
	status := "success"
	if err != nil {
		status = "error"
		response.Err = err.Error()
		// 超时标记不健康
		if errors.Is(err, cErrors.ErrContainerExecTimeout) {
			healthy = false
		}
	} else {
		metrics.CodeExecutionDuration.WithLabelValues(request.Language).Observe(float64(duration) / 1000.0)
	}
	metrics.CodeExecutionTotal.WithLabelValues(request.Language, status).Inc()

	if err != nil {
		return 0, response, err
	}
	return duration, response, nil
}
```

- [ ] **Step 2: 修改 runCodeContainer — 传入 slot.Name**

```go
func (r *runCode) runCodeContainer(language, path string, slot ContainerSlot) (int64, string, error) {
	cmd, args := r.getCommand(language, path)
	if cmd == "" {
		return 0, "", fmt.Errorf("不支持的语言类型: %s", language)
	}
	duration, logContent, err := r.InContainerRunCode(slot.Name, cmd, args)
	if err != nil {
		return 0, "", err
	}
	return duration, logContent, nil
}
```

- [ ] **Step 3: 更新 import**

```go
import (
	"codeRunner-siwu/api/proto"
	cErrors "codeRunner-siwu/internal/infrastructure/common/errors"
	"codeRunner-siwu/internal/infrastructure/metrics"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)
```

移除 `"log"` import（用 zap 替代）。

- [ ] **Step 4: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./internal/infrastructure/containerBasic/...`
Expected: BUILD SUCCESS

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/containerBasic/runCode.go
git commit -m "feat: integrate pool slot into RunCode with HostPath-based file creation"
```

---

## Task 10: 修改 initialize/service.go — 传入 pool config

**Files:**
- Modify: `internal/interfaces/adapter/initialize/service.go`

- [ ] **Step 1: 修改 clientServiceRegister 接受 config 参数**

```go
func clientServiceRegister(c *config.Config) (*client.ServiceImpl, error) {
	// docker客户端（传入池配置）
	dockerClient := docker.NewDockerClient(c.Client.ContainerPool)
	containerTmpl := docker.NewRunCode(dockerClient)
	websocketClientImpl := client2.NewWebsocketClientImpl()
	InnerServerDomainImpl := entity.NewInnerServerDomainImpl(containerTmpl, websocketClientImpl)
	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)
	return clientSvr, nil
}
```

- [ ] **Step 2: 修改 RunClient 中调用 clientServiceRegister**

在 `app.go` 的 `RunClient()` 中，找到 `clientServiceRegister()` 调用改为 `clientServiceRegister(c)`：

```go
srv, err := clientServiceRegister(c)
```

- [ ] **Step 3: 添加 config import**

在 `service.go` 的 import 中添加：
```go
"codeRunner-siwu/internal/infrastructure/config"
```

- [ ] **Step 4: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./...`
Expected: BUILD SUCCESS（全量编译通过）

- [ ] **Step 5: Commit**

```bash
git add internal/interfaces/adapter/initialize/service.go internal/interfaces/adapter/initialize/app.go
git commit -m "feat: pass container pool config through initialization chain"
```

---

## Task 11: 修改 docker-compose — dev 环境

**Files:**
- Modify: `docker-compose/dev/client/docker-compose.yml`

- [ ] **Step 1: 改为池化命名（dev 每语言 1 个容器）**

将每个 runner 容器改为带 `-0` 后缀的命名。例如将：

```yaml
code-runner-go:
  ...
  container_name: code-runner-go
  volumes:
    - /tmp/golang:/app
```

改为：

```yaml
code-runner-go-0:
  image: code-runner-go:latest
  build:
    context: .
    dockerfile: builds/runners/go.Dockerfile
  container_name: code-runner-go-0
  command: sleep infinity
  restart: unless-stopped
  network_mode: none
  cap_drop:
    - ALL
  mem_limit: 512m
  memswap_limit: 512m
  cpus: "1.0"
  volumes:
    - /tmp/golang-0:/app
```

对所有 5 种语言做同样处理：
- `code-runner-go` → `code-runner-go-0`，volume `/tmp/golang` → `/tmp/golang-0`
- `code-runner-python` → `code-runner-python-0`，volume `/tmp/python` → `/tmp/python-0`
- `code-runner-cpp` → `code-runner-cpp-0`，volume `/tmp/c` → `/tmp/c-0`
- `code-runner-java` → `code-runner-java-0`，volume `/tmp/java` → `/tmp/java-0`
- `code-runner-js` → `code-runner-js-0`，volume `/tmp/javascript` → `/tmp/javascript-0`

- [ ] **Step 2: 验证 YAML 语法**

Run: `python3 -c "import yaml; yaml.safe_load(open('/Users/ningzhaoxing/Desktop/coderunner/docker-compose/dev/client/docker-compose.yml'))"`
Expected: No error

- [ ] **Step 3: Commit**

```bash
git add docker-compose/dev/client/docker-compose.yml
git commit -m "feat: rename dev runner containers to pool naming convention"
```

---

## Task 12: 修改 docker-compose — product 环境

**Files:**
- Modify: `docker-compose/product/client/docker-compose.yml`

- [ ] **Step 1: 改为池化命名（product 每语言 2 个容器，Java 1 个）**

对每种语言扩展为 N 个容器（以 Go 为例）：

```yaml
code-runner-go-0:
  image: code-runner-go:latest
  build:
    context: .
    dockerfile: builds/runners/go.Dockerfile
  container_name: code-runner-go-0
  command: sleep infinity
  restart: unless-stopped
  network_mode: none
  cap_drop:
    - ALL
  mem_limit: 512m
  memswap_limit: 512m
  cpus: "1.0"
  volumes:
    - /tmp/golang-0:/app

code-runner-go-1:
  image: code-runner-go:latest
  build:
    context: .
    dockerfile: builds/runners/go.Dockerfile
  container_name: code-runner-go-1
  command: sleep infinity
  restart: unless-stopped
  network_mode: none
  cap_drop:
    - ALL
  mem_limit: 512m
  memswap_limit: 512m
  cpus: "1.0"
  volumes:
    - /tmp/golang-1:/app
```

同理对 python（×2）、cpp（×2）、java（×1 仅改名 -0）、js（×2）。

- [ ] **Step 2: 验证 YAML 语法**

Run: `python3 -c "import yaml; yaml.safe_load(open('/Users/ningzhaoxing/Desktop/coderunner/docker-compose/product/client/docker-compose.yml'))"`
Expected: No error

- [ ] **Step 3: Commit**

```bash
git add docker-compose/product/client/docker-compose.yml
git commit -m "feat: expand product runner containers to pool size"
```

---

## Task 13: 优雅关停（graceful shutdown）

**Files:**
- Modify: `internal/interfaces/adapter/initialize/app.go`

- [ ] **Step 1: 修改 clientServiceRegister 返回 dockerClient 引用**

`clientServiceRegister` 需要返回 pool 的引用供关停使用。修改返回值：

```go
func clientServiceRegister(c *config.Config) (*client.ServiceImpl, *docker.ContainerPool, error) {
	dockerClient := docker.NewDockerClient(c.Client.ContainerPool)
	containerTmpl := docker.NewRunCode(dockerClient)
	websocketClientImpl := client2.NewWebsocketClientImpl()
	InnerServerDomainImpl := entity.NewInnerServerDomainImpl(containerTmpl, websocketClientImpl)
	clientSvr := client.NewServiceImpl(InnerServerDomainImpl)
	return clientSvr, dockerClient.Pool(), nil
}
```

注意：这要求 `container.go` 中的 `Pool()` 方法返回 `*ContainerPool`（Task 8 Step 8 已添加），并且 `ContainerPool` 类型是导出的。

- [ ] **Step 2: 修改 RunClient 添加信号处理和 pool.Close()**

```go
func RunClient() {
	c, err := InitConfig()
	if err != nil {
		panic("配置文件解析错误" + err.Error())
		return
	}

	err = InitLogger(c)
	if err != nil {
		panic("日志文件解析错误" + err.Error())
		return
	}

	srv, pool, err := clientServiceRegister(c)
	if err != nil {
		panic("服务注册失败,原因:" + err.Error())
	}

	// 优雅关停：监听系统信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		zap.S().Info("收到关停信号，正在关闭容器池...")
		pool.Close()
		os.Exit(0)
	}()

	if err := srv.Run(*c); err != nil {
		log.Println(fmt.Sprintf("服务启动失败err=%s\n", err))
	}
}
```

需要在 `app.go` 的 import 中追加：
```go
"os"
"os/signal"
"syscall"
docker "codeRunner-siwu/internal/infrastructure/containerBasic"
```

- [ ] **Step 3: 验证编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 4: Commit**

```bash
git add internal/interfaces/adapter/initialize/app.go internal/interfaces/adapter/initialize/service.go
git commit -m "feat: add graceful shutdown with pool.Close() on SIGINT/SIGTERM"
```

---

## Task 14: 全量编译验证 + 测试

**Files:** None (verification only)

- [ ] **Step 1: 全量编译**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 2: 运行所有测试**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go test ./... -v`
Expected: ALL PASS

- [ ] **Step 3: go vet 检查**

Run: `cd /Users/ningzhaoxing/Desktop/coderunner && go vet ./...`
Expected: No issues

- [ ] **Step 4: 如有编译或测试问题，逐个修复后重新验证**

---

## 实施注意事项

1. **容器名命名规则（已解决）：** 池中容器名使用 `images[lang]` 的 value 作为前缀（如 `code-runner-go-0`），而非 language key（如 ~~`code-runner-golang-0`~~）。宿主机路径使用 `langHostDir[lang]` 映射（如 `/app/tmp/golang-0`），与 docker-compose volumes 保持一致。两者通过 `langHostDir` map 解耦。

2. **`runCode.go` 中的 `log.Println`：** 当前代码中混用了 `log` 和 `zap`，CLAUDE.md 要求只用 Zap。实现时顺手将涉及修改的行改为 `zap.S()`，但不批量清理未改动的代码。

3. **`Release` 对已关闭池的保护：** `Close()` 会 close channel，之后 `Release` 中的 `lp.idle <- slot` 会 panic。因此 `Release` 开头需检查 `p.closed.Load()`（已在计划中处理）。

4. **设计文档偏差说明：** 设计文档原意是 `InContainerRunCode` 签名不变、pool 操作在其内部完成。但由于 `RunCode` 需要在创建文件前获取 `slot.HostPath`，这要求 Acquire 发生在 `InContainerRunCode` 之前。因此本计划将 pool 操作提升到 `RunCode` 层，并通过 `DockerContainer` 接口暴露 `AcquireSlot`/`ReleaseSlot`。`Container` 接口（暴露给 domain 层）签名不变，domain 层无感知。
