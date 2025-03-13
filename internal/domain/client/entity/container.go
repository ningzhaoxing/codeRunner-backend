package entity

import (
	"codeRunner-siwu/api/proto"
	"context"
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type DockerContainer interface {
	RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error)
}

type dockerContainerClient struct {
	ctx context.Context
	cli *client.Client
}

// NewDockerClient 新构造函数：通过完整host地址连接
func NewDockerClient(ctx context.Context) (*dockerContainerClient, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"),
		client.WithAPIVersionNegotiation(), // 自动协商API版本
	)
	if err != nil {
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}
	return &dockerContainerClient{ctx: ctx, cli: cli}, nil
}

// CreateContainer 创建指定容器
func (client *dockerContainerClient) createContainer(image string, dirName string) (container.CreateResponse, error) {
	config := &container.Config{
		Image: image,
		User:  "nobody", // 以非特权用户运行
	}

	hostConfig := &container.HostConfig{
		ReadonlyRootfs: true,            // 只读文件系统
		CapDrop:        []string{"ALL"}, // 移除所有特权能力
		Resources: container.Resources{
			Memory:   100 * 1024 * 1024, // 限制100MB内存
			CPUQuota: 50000,             // 限制50% CPU
		},
		Binds: []string{fmt.Sprintf("/tmp/tmpDir/%s:/app", dirName)}, // 挂载宿主机目录到容器内/mnt
	}

	resp, err := client.cli.ContainerCreate(
		client.ctx,
		config,
		hostConfig,
		nil,
		nil,
		"",
	)
	if err != nil {
		return container.CreateResponse{}, fmt.Errorf("容器创建失败: %v", err)
	}
	return resp, nil
}

// RmContainer 删除指定id容器
func (client *dockerContainerClient) rmContainer(id string) error {
	// 设置删除选项
	option := container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
		Force:         true,
	}
	err := client.cli.ContainerRemove(client.ctx, id, option)
	if err != nil {
		return fmt.Errorf("删除容器失败:%v", err)
	}
	return nil
}

// StopContainer 停止指定id容器
func (client *dockerContainerClient) stopContainer(id string) error {
	err := client.cli.ContainerStop(client.ctx, id, container.StopOptions{})
	if err != nil {
		return fmt.Errorf("停止容器失败:%v", err)
	}
	return nil
}

// 获取镜像名
func (client *dockerContainerClient) getImageName(lang string) string {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "golang:1.21-alpine",
		"python": "python:3.11-slim",
		"node":   "node:20-alpine",
		"java":   "openjdk:21-jdk-slim",
		"c++":    "gcc:12.2.0",
	}
	if ext, ok := extensionMap[lang]; ok {
		return ext
	}
	return "txt"
}

// 获取文件扩展名
func (client *dockerContainerClient) getFileExtension(lang string) (string, error) {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "go",
		"python": "py",
		"node":   "js",
		"java":   "java",
		"c++":    "cpp",
	}
	if ext, ok := extensionMap[lang]; ok {
		return ext, nil
	}
	return "", fmt.Errorf("当前服务不支持此类型")
}

func (client *dockerContainerClient) RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error) {
	// 1. 生成唯一临时目录
	uniqueID := uuid.New().String()
	tempDir := filepath.Join("/tmp/tmpDir", uniqueID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("创建临时目录失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}
	defer os.RemoveAll(tempDir) // 确保目录最终被删除

	// 2. 创建代码文件（根据语言扩展名）
	ext, err := client.getFileExtension(request.Language)
	if err != nil {
		return response, fmt.Errorf("不支持的语言类型: %s", request.Language)
	}
	codePath := filepath.Join(tempDir, fmt.Sprintf("main.%s", ext))
	if err := os.WriteFile(codePath, []byte(request.CodeBlock), 0644); err != nil {
		log.Printf("写入代码文件失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}

	// 3. 创建并启动容器
	resp, err := client.createContainer(request.Language, tempDir)
	if err != nil {
		log.Printf("容器创建失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}
	containerID := resp.ID
	defer func() { // 确保容器最终被清理
		_ = client.stopContainer(containerID)
		_ = client.rmContainer(containerID)
	}()

	// 4. 启动容器
	if err := client.cli.ContainerStart(client.ctx, containerID, container.StartOptions{}); err != nil {
		log.Printf("启动容器失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}

	// 5. 等待容器执行完成
	statusCh, errCh := client.cli.ContainerWait(client.ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		log.Printf("容器执行异常: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	case <-client.ctx.Done():
		return response, fmt.Errorf("超时取消")
	case <-statusCh: // 正常退出
	}

	// 6. 读取容器日志
	logs, err := client.cli.ContainerLogs(
		client.ctx,
		containerID,
		container.LogsOptions{ShowStdout: true, ShowStderr: true},
	)
	if err != nil {
		log.Printf("获取日志失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}
	defer logs.Close()

	logContent, _ := io.ReadAll(logs)
	response.Result = string(logContent)
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl
	return response, nil
}
