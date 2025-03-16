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
	"strings"
	"time"
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
		log.Println("domain.client.entity.NewDockerClient() NewClientWithOpts err=", err)
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}
	return &dockerContainerClient{ctx: ctx, cli: cli}, nil
}

// CreateContainer 创建指定容器
func (client *dockerContainerClient) createContainer(image string, dirName string) (container.CreateResponse, error) {
	config := &container.Config{
		Image:      image,
		User:       "nobody", // 以非特权用户运行
		WorkingDir: "/app",
		Cmd:        []string{"sh", "-c", "go run main.go"},
	}

	hostConfig := &container.HostConfig{
		ReadonlyRootfs: false,           // 只读文件系统
		CapDrop:        []string{"ALL"}, // 移除所有特权能力
		Resources: container.Resources{
			Memory:   100 * 1024 * 1024, // 限制100MB内存
			CPUQuota: 50000,             // 限制50% CPU
		},
		Binds: []string{fmt.Sprintf("%s:/app", dirName)}, // 挂载宿主机目录到容器内/mnt
	}
	//Mounts: []mount.Mount{
	//	{
	//		Type:   mount.TypeBind,                                             // 使用 bind 挂载
	//		Source: fmt.Sprintf("%s", strings.TrimSuffix(dirName, "/main.go")), // 使用父目录进行挂载
	//		Target: "/app",                                                     // 子容器中的目标挂载点
	//	},
	//},

	fmt.Println("挂载路径 -> /app", dirName)

	resp, err := client.cli.ContainerCreate(
		client.ctx,
		config,
		hostConfig,
		nil,
		nil,
		"",
	)
	if err != nil {
		log.Println("domain.client.entity.createContainer() ContainerCreate err=", err)
		return container.CreateResponse{}, fmt.Errorf("容器创建失败: %v", err)
	}
	return resp, nil
}

// StopContainer 停止指定id容器
func (client *dockerContainerClient) stopContainer(id string) error {
	// 1. 停止容器
	if err := client.cli.ContainerStop(client.ctx, id, container.StopOptions{}); err != nil {
		log.Printf("停止容器失败: %v", err)
		return fmt.Errorf("停止容器失败: %v", err)
	}

	// 2. 断开容器所有网络连接
	if err := client.cli.NetworkDisconnect(client.ctx, "bridge", id, true); err != nil {
		log.Printf("断开默认网络失败: %v", err)
		return err
	}

	// 3. 强制删除容器（调整删除选项）
	removeOpts := container.RemoveOptions{
		Force:         true,  // 强制删除
		RemoveLinks:   false, // 显式关闭链接删除（避免冲突）
		RemoveVolumes: true,  // 按需保留或删除卷
	}

	if err := client.cli.ContainerRemove(client.ctx, id, removeOpts); err != nil {
		log.Printf("删除容器失败: %v", err)
		return fmt.Errorf("删除容器失败: %v", err)
	}

	return nil
}

// 获取镜像名
func (client *dockerContainerClient) getImageName(lang string) string {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "code-runner-go",
		"python": "code-runner-python",
		"node":   "code-runner-js",
		"java":   "code-runner-java",
		"c++":    "code-runner-cpp",
	}
	if ext, ok := extensionMap[lang]; ok {
		return ext
	}
	return ""
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
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl
	// 1. 生成唯一临时目录（使用系统标准临时目录）
	uniqueID := uuid.New().String()

	tempDir := fmt.Sprintf("/tmp/%s", uniqueID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("创建临时目录失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}
	//defer os.RemoveAll(tempDir)

	// 2. 创建代码文件
	ext, err := client.getFileExtension(request.Language)
	if err != nil {
		response.Err = fmt.Sprintf("不支持的语言类型: %s", request.Language)
		return response, nil
	}

	codePath := fmt.Sprintf("%s/main.%s", tempDir, ext)
	file, err := os.Create(codePath)
	if err != nil {
		log.Printf("创建代码文件失败: %v", err)
		response.Err = "docker客户端错误"
		return response, nil
	}
	_, err = file.WriteString(request.CodeBlock)
	if err != nil {
		log.Printf("写入代码文件失败: %v", err)
		response.Err = "docker客户端错误"
		return response, nil
	}

	// 3. 创建并启动容器
	imageName := client.getImageName(request.Language)
	if imageName == "" {
		response.Err = "不支持的语言类型"
		return response, nil
	}
	resp, err := client.createContainer(imageName, codePath)
	if err != nil {
		log.Printf("容器创建失败: %v", err)
		response.Err = fmt.Errorf("docker客户端错误").Error()
		return response, nil
	}
	containerID := resp.ID

	// 4. 启动容器
	if err := client.cli.ContainerStart(client.ctx, containerID, container.StartOptions{}); err != nil {
		log.Printf("启动容器失败: %v", err)
		response.Err = fmt.Errorf("docker客户端错误").Error()
		return response, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 5. 等待容器执行完成
	statusCh, errCh := client.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		log.Printf("容器执行异常: %v", err)
		response.Err = fmt.Errorf("docker客户端错误").Error()
		return response, nil
	case <-ctx.Done():
		log.Printf("超时取消:")
		response.Err = fmt.Errorf("超时取消").Error()
		return response, nil
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
		response.Err = fmt.Errorf("docker客户端错误").Error()
		return response, nil
	}
	defer logs.Close()
	//停止容器
	defer client.stopContainer(containerID)
	//删除容器
	logContent, _ := io.ReadAll(logs)
	response.Result = string(logContent)
	return response, nil
}
