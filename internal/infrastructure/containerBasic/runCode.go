package containerBasic

import (
	"codeRunner-siwu/api/proto"
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
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

type runCode struct {
	ContainerSrv
	cli *client.Client
	ctx context.Context
	//错误
	err error
	//扩展名
	extension string
	//文件句柄
	file *os.File
}

func NewRunCode(containSrv ContainerSrv) (*runCode, error) {
	cli, err := client.NewClientWithOpts(
		client.WithHost("unix:///var/run/docker.sock"),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("创建Docker客户端失败: %v", err)
	}

	return &runCode{
		ContainerSrv: containSrv,
		cli:          cli,
		ctx:          context.Background(),
		err:          nil,
	}, nil
}

// 获取文件扩展名
func (r *runCode) getFileExtension(lang string) *runCode {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"go":     "go",
		"python": "py",
		"node":   "js",
		"java":   "java",
		"c":      "cpp",
	}
	if ext, ok := extensionMap[lang]; ok {
		r.extension = ext
		return r
	}
	r.err = fmt.Errorf("当前服务不支持此类型")
	return r
}

// 创建空文件
func (r *runCode) createBlockFile(path string) *runCode {
	codePath := fmt.Sprintf("%s.%s", path, r.extension)
	r.file, r.err = os.Create(codePath)
	return r
}

// 写入代码
func (r *runCode) writeCode(code string) *runCode {
	if r.err != nil {
		log.Println("runCode-writeCode err =", r.err)
		return r
	}
	_, r.err = r.file.WriteString(code)
	return r
}

// 同步磁盘
func (r *runCode) sync() *runCode {
	r.err = r.file.Sync()
	return r
}

// 创建文件
func (r *runCode) createFile(language, code string, path string) error {

	r.getFileExtension(language).createBlockFile(path).writeCode(code).sync()
	if r.err != nil {
		log.Println("runCode-createFile 的 err=", r.err)
		return r.err
	}
	return nil
}

// 获得执行命令
func (r *runCode) getCommand(language, path string) (string, []string) {
	switch language {
	case "go":
		return "go", []string{"run", path}
	case "c":
		return "gcc", []string{"-o", path}
	case "python":
		return "python", []string{path}
	case "java":
		return "javac", []string{path}
	case "js":
		return "node", []string{path}
	}
	return "", nil
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

func (r *runCode) RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error) {
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl

	// 确保容器存在
	containerName := r.GetContains(request.Language)
	if err := r.ContainerSrv.EnsureContainerExists(request.Language); err != nil {
		log.Printf("确保容器 %s 存在时出错: %v", containerName, err)
		return response, fmt.Errorf("容器准备失败: %v", err)
	}

	//创建文件
	uniqueID := uuid.New().String()
	path := fmt.Sprintf("/app/tmp/%s/%s", request.Language, uniqueID)

	//创建文件
	err = r.createFile(request.Language, request.CodeBlock, path)
	if err != nil {
		log.Println(" containerBasic-RunCode-createFile err=", err)
		return response, err
	}
	defer r.file.Close()
	defer func() {
		if r.file != nil {
			r.file.Close()
			err := os.Remove(fmt.Sprintf("%s.%s", path, r.extension))
			if err != nil {
				fmt.Printf("删除文件时出错: %v\n", err)
			}
		}
	}()

	//开始执行代码
	containerPath := fmt.Sprintf("/app/%s.%s", uniqueID, r.extension)
	log.Printf("执行代码，容器路径: %s", containerPath)
	containName := r.GetContains(request.Language)

	// 设置超时上下文
	ctx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
	defer cancel()

	// 获取执行命令
	cmd, args := r.getCommand(request.Language, containerPath)
	if cmd == "" {
		return response, fmt.Errorf("不支持的语言类型: %s", request.Language)
	}

	// 执行命令
	execConfig := types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          append([]string{cmd}, args...),
		WorkingDir:   "/app",
		User:         "root",
	}

	execID, err := r.cli.ContainerExecCreate(ctx, containName, execConfig)
	if err != nil {
		log.Printf("创建执行实例失败: %v", err)
		return response, err
	}

	// 启动执行
	err = r.cli.ContainerExecStart(ctx, execID.ID, types.ExecStartCheck{})
	if err != nil {
		log.Printf("启动执行失败: %v", err)
		return response, err
	}

	// 等待执行完成
	execInspect, err := r.cli.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		log.Printf("获取执行状态失败: %v", err)
		return response, err
	}

	if execInspect.ExitCode != 0 {
		// 获取错误输出
		logs, _ := r.cli.ContainerLogs(ctx, containName, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Details:    true,
		})
		if logs != nil {
			logContent, _ := io.ReadAll(logs)
			response.Err = processDockerLogs(logContent)
		}
		return response, fmt.Errorf("执行失败，退出码: %d", execInspect.ExitCode)
	}

	// 获取输出
	logs, err := r.cli.ContainerLogs(ctx, containName, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Details:    true,
	})
	if err != nil {
		log.Printf("获取输出失败: %v", err)
		return response, err
	}

	logContent, _ := io.ReadAll(logs)
	response.Result = processDockerLogs(logContent)

	return response, nil
}
