package containerBasic

import (
	"bytes"
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sirupsen/logrus"
	"log"
	"os"
	"time"
)

type DockerContainer interface {
	InContainerRunCode(language string, cmd string, args []string) (int64, string, error)
	GetContains(language string) (container string)
}

type dockerContainerClient struct {
	ctx context.Context
	cli *client.Client
	err error
	//记录支持的语言类型
	language []string
	//支持的镜像
	images map[string]string
}

// GetContains 得到容器名
func (c *dockerContainerClient) GetContains(language string) (container string) {
	return c.images[language]
}

// NewDockerClient 新构造函数：通过完整host地址连接
func NewDockerClient() *dockerContainerClient {
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
	dockerContainerClient := dockerContainerClient{
		err:      nil,
		language: language,
		images:   images,
		cli:      cli,
		ctx:      context.Background(),
	}

	// 创建目录
	dockerContainerClient.createContent()

	// 确保每个语言的容器都存在
	for _, lang := range language {
		dockerContainerClient = *dockerContainerClient.ensureContainerExists(lang)
		if dockerContainerClient.err != nil {
			log.Printf("确保 %s 容器存在时出错: %v", lang, err)
			return &dockerContainerClient
		}
	}

	return &dockerContainerClient
}

// ensureContainerExists 检查容器是否存在并运行，容器由 docker-compose 负责创建和配置资源限制
func (client *dockerContainerClient) ensureContainerExists(language string) *dockerContainerClient {
	containerName := client.images[language]
	args := filters.NewArgs()
	args.Add("name", containerName)

	containers, err := client.cli.ContainerList(client.ctx, container.ListOptions{
		All:     true,
		Filters: args,
	})
	if err != nil {
		client.err = fmt.Errorf("检查容器失败: %v", err)
		return client
	}
	if len(containers) == 0 {
		// 容器不存在：应通过 docker-compose 启动，不在此处创建
		client.err = fmt.Errorf("容器 %s 不存在，请先执行 docker-compose up 启动所有 runner 容器", containerName)
		return client
	}
	if containers[0].State != "running" {
		logrus.Infof("容器 %s 未运行，正在启动...", containerName)
		if err := client.cli.ContainerStart(client.ctx, containers[0].ID, container.StartOptions{}); err != nil {
			client.err = fmt.Errorf("启动容器失败: %v", err)
			return client
		}
		logrus.Infof("容器 %s 已启动", containerName)
	}
	return client
}

// 创建对应的文件夹
func (client *dockerContainerClient) createContent() *dockerContainerClient {
	// 在/app/tmp下创建文件夹
	for i := 0; i < len(client.language); i++ {
		tempDir := fmt.Sprintf("/app/tmp/%s", client.language[i])
		if err := os.MkdirAll(tempDir, 0755); err != nil {
			logrus.Error("创建目录 %s 失败: %v", tempDir, err)
			client.err = err
			return client
		}
		logrus.Info("创建目录成功: %s", tempDir)
	}
	return client
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
		logrus.Error("创建exec失败: %v", err)
		return "", fmt.Errorf("创建exec失败: %v", err)
	}

	// 3. 启动exec并获取输出流
	exec, err := c.cli.ContainerExecAttach(ctx, resp.ID, container.ExecStartOptions{})
	if err != nil {
		logrus.Error("启动exec失败: %v", err)
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
		logrus.Error("命令执行超时")
		return "", fmt.Errorf("命令执行超时")
	}

	// 6. 处理读取错误
	if readErr != nil {
		logrus.Error("读取输出错误: %v", readErr)
		return "", readErr
	}
	// 7. 获取退出状态
	inspect, err := c.cli.ContainerExecInspect(ctx, resp.ID)
	if err != nil {
		logrus.Error("获取退出状态失败: %v", err)
		return "", fmt.Errorf("获取退出状态失败: %v", err)
	}

	// 8. 根据退出码判断结果
	if inspect.ExitCode != 0 {
		logrus.Error("执行失败，退出码: %d", inspect.ExitCode)
	} else {
		logrus.Error("执行成功")
	}
	// 打印完整输出
	return outputBuf.String(), nil
}

func (c *dockerContainerClient) InContainerRunCode(language string, cmd string, args []string) (int64, string, error) {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	containerOne, err := c.cli.ContainerInspect(ctx, c.images[language])
	if err != nil {
		logrus.Error("容器ID未找到 err=", err)
		return 0, "", err
	}
	start := time.Now()
	result, err := c.buildExec(ctx, cmd, containerOne.ID, args)
	elapsed := time.Since(start)
	duration := elapsed.Milliseconds()
	if err != nil {
		if err.Error() == "命令执行超时" {
			return 0, "", err
		}
		return 0, "", fmt.Errorf("内网服务器出错")
	}
	return duration, result, nil
}
