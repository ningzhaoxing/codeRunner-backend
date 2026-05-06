package containerBasic

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

type DockerContainer interface {
	InContainerRunCode(containerName string, cmd string, args []string) (int64, string, error)
	AcquireSlot(ctx context.Context, language string) (ContainerSlot, error)
	ReleaseSlot(language string, slot ContainerSlot, healthy bool)
}

type dockerContainerClient struct {
	ctx context.Context
	cli *client.Client
	err error
	//记录支持的语言类型
	language []string
	//支持的镜像
	images map[string]string
	pool   *ContainerPool
}

// AcquireSlot 从池中获取一个空闲容器槽位
func (c *dockerContainerClient) AcquireSlot(ctx context.Context, language string) (ContainerSlot, error) {
	return c.pool.Acquire(ctx, language)
}

// ReleaseSlot 将容器槽位归还池中
func (c *dockerContainerClient) ReleaseSlot(language string, slot ContainerSlot, healthy bool) {
	c.pool.Release(language, slot, healthy)
}

// Pool 返回容器池实例
func (c *dockerContainerClient) Pool() *ContainerPool {
	return c.pool
}

// NewDockerClient 新构造函数：通过完整host地址连接
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

	for _, lang := range language {
		size := poolSizes[lang]
		imgPrefix := images[lang]
		hostDir := langHostDir[lang]
		for i := 0; i < size; i++ {
			containerName := fmt.Sprintf("%s-%d", imgPrefix, i)
			hostPath := fmt.Sprintf("/app/tmp/%s-%d", hostDir, i)
			if mkErr := os.MkdirAll(hostPath, 0755); mkErr != nil {
				zap.S().Errorf("创建目录 %s 失败: %v", hostPath, mkErr)
			}
			dockerClient.ensureContainerExistsByName(containerName)
		}
	}

	return dockerClient
}

// ensureContainerExistsByName 检查指定名称的容器是否存在并运行
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

func (c *dockerContainerClient) buildExec(ctx context.Context, cmd, id string, args []string) (string, error) {
	// 1. 创建exec配置
	execConfig := container.ExecOptions{
		Cmd:          append([]string{cmd}, args...),
		AttachStdout: true,
		AttachStderr: true,
	}

	// 2. 创建exec实例
	resp, err := c.cli.ContainerExecCreate(ctx, id, execConfig)
	if err != nil {
		zap.S().Error("创建exec失败: %v", err)
		return "", fmt.Errorf("创建exec失败: %v", err)
	}

	// 3. 启动exec并获取输出流
	exec, err := c.cli.ContainerExecAttach(ctx, resp.ID, container.ExecStartOptions{})
	if err != nil {
		zap.S().Error("启动exec失败: %v", err)
		return "", fmt.Errorf("启动exec失败: %v", err)
	}
	defer exec.Close()

	// 4. 异步读取输出（同时处理stdout和stderr）
	var (
		outputBuf bytes.Buffer
		readErr   error
		doneChan  = make(chan struct{})
	)

	go func() {
		defer close(doneChan)
		// 使用标准库方法分离stdout/stderr
		_, readErr = stdcopy.StdCopy(&outputBuf, &outputBuf, exec.Conn)
	}()

	// 5. 等待执行完成或超时
	select {
	case <-doneChan:
		// 正常读取完成
	case <-ctx.Done():
		zap.S().Error("命令执行超时")
		return "", fmt.Errorf("命令执行超时")
	}

	// 6. 处理读取错误
	if readErr != nil {
		zap.S().Error("读取输出错误: %v", readErr)
		return "", readErr
	}
	// 7. 获取退出状态
	inspect, err := c.cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		zap.S().Error("获取退出状态失败: %v", err)
		return "", fmt.Errorf("获取退出状态失败: %v", err)
	}

	// 8. 根据退出码判断结果
	if inspect.ExitCode != 0 {
		zap.S().Errorf("执行失败，退出码: %d", inspect.ExitCode)
	} else {
		zap.S().Info("执行成功")
	}
	// 打印完整输出
	return outputBuf.String(), nil
}

func (c *dockerContainerClient) InContainerRunCode(containerName string, cmd string, args []string) (int64, string, error) {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	containerOne, err := c.cli.ContainerInspect(ctx, containerName)
	if err != nil {
		zap.S().Warnw("容器检查失败，尝试重新确认容器状态", "container", containerName, "err", err)
		c.ensureContainerExistsByName(containerName)
		containerOne, err = c.cli.ContainerInspect(ctx, containerName)
		if err != nil {
			zap.S().Error("容器ID未找到 err=", err)
			return 0, "", err
		}
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
