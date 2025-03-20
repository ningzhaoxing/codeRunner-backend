package containerBasic

import (
	"codeRunner-siwu/api/proto"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"log"
	"os"
	"os/exec"
	"strings"
)

type Container interface {
	RunCode(request *proto.ExecuteRequest) (response proto.ExecuteResponse, err error)
}

type runCode struct {
	ContainerSrv
	//错误
	err error
	//扩展名
	extension string
	//文件句柄
	file *os.File
}

func NewRunCode(containSrv ContainerSrv) *runCode {
	return &runCode{
		ContainerSrv: containSrv,
		err:          nil,
	}
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
	r.file, r.err = os.Create(path)
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

	codePath := fmt.Sprintf("%s.%s", path, r.extension)
	r.getFileExtension(language).createBlockFile(codePath).writeCode(code).sync()
	if r.err != nil {
		log.Println("runCode-createFile 的 err=", r.err)
		return r.err
	}
	return nil
}

// 获得执行命令
func (r *runCode) getCommand(language, containerName, path string) *exec.Cmd {
	switch language {
	case "go":
		return exec.Command("docker", "exec", containerName, "timeout", "10", "go", "run", path)
	case "c":
		return exec.Command("docker", "exec", containerName, "timeout", "10", "gcc", "-o", path)
	case "python":
		return exec.Command("docker", "exec", containerName, "timeout", "10", "python", path)
	case "java":
		return exec.Command("docker", "exec", containerName, "timeout", "10", "javac", path)
	case "js":
		return exec.Command("docker", "exec", containerName, "timeout", "10", "node", path)
	}
	return nil
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
			err := os.Remove(path)
			if err != nil {
				fmt.Printf("删除文件时出错: %v\n", err)
			}
		}
	}()
	//开始执行代码
	containerPath := fmt.Sprintf("/app/%s.%s", uniqueID, r.extension)
	containName := r.GetContains(request.Language)
	cmd := r.getCommand(request.Language, containName, containerPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			response.Result = "time.out"
			return response, r.err
		}
		log.Printf("命令执行失败: %v, 输出: %s", err, output)
		response.Err = err.Error()
		return response, r.err
	}
	if len(output) > 0 {
		response.Result = processDockerLogs(output)
	}
	return response, r.err
}
