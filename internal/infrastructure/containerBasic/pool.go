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
		imgPrefix := images[lang]    // e.g. "code-runner-go"
		hostDir := langHostDir[lang] // e.g. "golang"
		lp := &langPool{
			idle:  make(chan ContainerSlot, size),
			lang:  lang,
			size:  size,
			image: imgPrefix,
		}
		for i := 0; i < size; i++ {
			lp.idle <- ContainerSlot{
				Name:     fmt.Sprintf("%s-%d", imgPrefix, i),        // "code-runner-go-0"
				HostPath: fmt.Sprintf("/app/tmp/%s-%d", hostDir, i), // "/app/tmp/golang-0"
			}
		}
		pools[lang] = lp
	}
	return &ContainerPool{pools: pools, cli: cli}
}

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

// Release 将容器归还池中
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
		return
	}
	go p.replenish(lp, slot)
}

// Close 优雅关停
func (p *ContainerPool) Close() {
	p.closed.Store(true)
	for _, lp := range p.pools {
		close(lp.idle)
	}
}

// replenish 重建损坏的容器并重新放入池
func (p *ContainerPool) replenish(lp *langPool, slot ContainerSlot) {
	const maxRetries = 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctx := context.Background()

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
