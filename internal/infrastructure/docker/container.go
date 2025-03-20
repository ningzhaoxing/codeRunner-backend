package docker

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

type Container interface {
	RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error)
}

type ContainerTmpl struct {
	ctx context.Context
	cli *client.Client
}

// NewContainerTmpl 新构造函数：通过完整host地址连接
func NewContainerTmpl(ctx context.Context) (*ContainerTmpl, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"),
		client.WithAPIVersionNegotiation(), // 自动协商API版本
	)
	if err != nil {
		log.Println("domain.client.entity.NewContainerTmpl() NewClientWithOpts err=", err)
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}
	return &ContainerTmpl{ctx: ctx, cli: cli}, nil
}

// CreateContainer 创建指定容器
func (client *ContainerTmpl) createContainer(image string, dirName string) (container.CreateResponse, error) {
	config := &container.Config{
		Image:      image,
		User:       "root",
		WorkingDir: "/app",
		// 添加环境变量，确保 Go 模块初始化
		Env: []string{
			"GO111MODULE=on",
			"GOPROXY=https://goproxy.cn,direct",
		},
	}

	hostConfig := &container.HostConfig{
		ReadonlyRootfs: false,           // 只读文件系统
		CapDrop:        []string{"ALL"}, // 移除所有特权能力
		Resources: container.Resources{
			Memory:     512 * 1024 * 1024, // 增加到512MB内存
			MemorySwap: 512 * 1024 * 1024, // 限制swap使用
			CPUQuota:   100000,            // 限制100% CPU
			CPUPeriod:  100000,            // CPU CFS (Completely Fair Scheduler) 周期
			CPUCount:   1,                 // 限制使用1个CPU核心
		},
		Binds: []string{fmt.Sprintf("/tmp/%s:/app", dirName)}, // 挂载到容器的/app目录
	}

	fmt.Printf("创建容器配置：\n")
	fmt.Printf("镜像：%s\n", image)
	fmt.Printf("工作目录：%s\n", config.WorkingDir)
	fmt.Printf("挂载路径：%s\n", hostConfig.Binds[0])

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
func (client *ContainerTmpl) stopContainer(id string) error {
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
func (client *ContainerTmpl) getImageName(lang string) string {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "code-runner-go",
		"python": "code-runner-python",
		"node":   "code-runner-js",
		"java":   "code-runner-java",
		"c":      "code-runner-cpp",
	}
	if ext, ok := extensionMap[lang]; ok {
		return ext
	}
	return ""
}

// 获取文件扩展名
func (client *ContainerTmpl) getFileExtension(lang string) (string, error) {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "go",
		"python": "py",
		"node":   "js",
		"java":   "java",
		"c":      "cpp",
	}
	if ext, ok := extensionMap[lang]; ok {
		return ext, nil
	}
	return "", fmt.Errorf("当前服务不支持此类型")
}

// 处理Docker日志格式
func processDockerLogs(logContent []byte) string {

	// 跳过Docker日志头（8字节）
	if len(logContent) <= 8 {
		return ""
	}

	// 获取实际内容（跳过8字节头）
	content := logContent[8:]

	// 移除末尾的换行符
	if len(content) > 0 && content[len(content)-1] == '\n' {
		content = content[:len(content)-1]
	}

	return string(content)
}

// 创建文件目录
func (client *ContainerTmpl) createVolmn(uniqueID string) (tempDir string, err error) {
	tempDir = fmt.Sprintf("/app/tmp/%s", uniqueID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", err
	}
	return tempDir, nil
}

// 创建文件
func (client *ContainerTmpl) createFile(code string, tempDir string, ext string) (file *os.File, err error) {
	codePath := fmt.Sprintf("%s/main.%s", tempDir, ext)
	file, err = os.Create(codePath)
	if err != nil {
		log.Printf("创建代码文件失败: %v", err)
		return nil, err
	}
	_, err = file.WriteString(code)
	if err != nil {
		log.Printf("写入代码文件失败: %v", err)
		return nil, err
	}
	if err := file.Sync(); err != nil { // 强制同步到磁盘
		log.Printf("同步文件失败: %v", err)
		return nil, err
	}
	return file, nil
}
func (client *ContainerTmpl) RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error) {
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl
	// 1. 生成唯一临时目录（使用系统标准临时目录）
	uniqueID := uuid.New().String()
	//创建目录
	tempDir, err := client.createVolmn(uniqueID)
	if err != nil {
		log.Printf("创建临时目录失败: %v", err)
		return response, fmt.Errorf("docker客户端错误")
	}
	defer os.RemoveAll(tempDir)

	// 2. 得到后缀
	ext, err := client.getFileExtension(request.Language)
	if err != nil {
		response.Err = fmt.Sprintf("不支持的语言类型: %s", request.Language)
		return response, nil
	}
	// 3.创建文件
	file, err := client.createFile(request.CodeBlock, tempDir, ext)
	if err != nil {
		response.Err = "docker客户端错误"
		return response, nil
	}
	defer file.Close()

	// 4.得到镜像名
	imageName := client.getImageName(request.Language)
	if imageName == "" {
		response.Err = "不支持的语言类型"
		return response, nil
	}
	// 5.创建并启动容器
	resp, err := client.createContainer(imageName, uniqueID)
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

	// 设置30秒超时，防止程序无限运行
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 5. 等待容器执行完成
	statusCh, errCh := client.cli.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		log.Printf("容器执行异常: %v", err)
		// 获取容器日志以查看具体错误
		logs, _ := client.cli.ContainerLogs(client.ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
		if logs != nil {
			logContent, _ := io.ReadAll(logs)
			log.Printf("容器日志: %s", string(logContent))
		}
		response.Err = fmt.Errorf("容器执行异常: %v", err).Error()
		return response, nil
	case <-ctx.Done():
		log.Printf("容器执行超时")
		// 获取容器日志以查看执行状态
		logs, _ := client.cli.ContainerLogs(client.ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
		if logs != nil {
			logContent, _ := io.ReadAll(logs)
			log.Printf("容器日志: %s", string(logContent))
		}
		response.Err = fmt.Errorf("容器执行超时").Error()
		return response, nil
	case status := <-statusCh:
		log.Printf("容器执行完成，退出码: %d", status.StatusCode)
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

	//读取并处理日志
	logContent, _ := io.ReadAll(logs)
	response.Result = processDockerLogs(logContent)
	return response, nil
}
