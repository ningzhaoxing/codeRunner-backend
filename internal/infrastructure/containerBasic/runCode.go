package containerBasic

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	proto "codeRunner-siwu/api/proto"
	cErrors "codeRunner-siwu/internal/infrastructure/common/errors"
	"codeRunner-siwu/internal/infrastructure/metrics"
)

type Container interface {
	RunCode(request *proto.ExecuteRequest) (duration int64, response proto.ExecuteResponse, err error)
}

type runCode struct {
	DockerContainer
	//错误
	err error
	//扩展名
	extension string
	//文件句柄
	file *os.File
	//文件目录
	path string
}

func NewRunCode(dockerContainer DockerContainer) *runCode {
	return &runCode{
		DockerContainer: dockerContainer,
		err:             nil,
	}
}

// 获取文件扩展名
func (r *runCode) getFileExtension(lang string) *runCode {
	lang = strings.ToLower(lang)
	extensionMap := map[string]string{
		"golang":     "go",
		"python":     "py",
		"javascript": "js",
		"java":       "java",
		"c":          "c",
	}
	if ext, ok := extensionMap[lang]; ok {
		r.extension = ext
		return r
	}
	r.err = fmt.Errorf("当前服务不支持此类型")
	return r
}

// 创建目录
func (r *runCode) createBlockContent(path string) *runCode {
	if r.err != nil {
		zap.S().Error("获取拓展名失败 err=", r.err)
		return r
	}
	r.err = os.MkdirAll(path, 0775)
	r.path = path
	return r
}

// 创建空文件
func (r *runCode) createBlockFile() *runCode {
	if r.err != nil {
		zap.S().Error("创建UUid目录失败 err=", r.err)
		return r
	}
	codePath := fmt.Sprintf("%s/main.%s", r.path, r.extension)
	r.file, r.err = os.Create(codePath)
	return r
}

// 写入代码
func (r *runCode) writeCode(code string) *runCode {
	if r.err != nil {
		zap.S().Error("runCode-writeCode err =", r.err)
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
func (r *runCode) createFile(language, code, path string) error {
	if r.err != nil {
		return r.err
	}
	r.getFileExtension(language).createBlockContent(path).createBlockFile().writeCode(code).sync()
	if r.err != nil {
		zap.S().Error("runCode-createFile 的 err=", r.err)
		return r.err
	}
	return nil
}

// 获得执行命令
func (r *runCode) getCommand(language, path string) (string, []string) {
	switch language {
	case "golang":
		return "go", []string{"run", path}
	case "c":
		outputPath := strings.TrimSuffix(path, ".c") // 移除 .c 后缀
		compileAndRun := fmt.Sprintf(
			"gcc -Wall -Wextra -Werror -O2 -o %s %s && %s",
			outputPath,
			path,
			outputPath,
		)
		return "sh", []string{"-c", compileAndRun}
	case "python":
		return "python", []string{path}
	case "java":
		// 获取文件名（不含路径）
		baseName := filepath.Base(path)
		// 获取类名（去掉.java后缀）
		className := strings.TrimSuffix(baseName, ".java")
		// 获取文件所在目录
		fileDir := filepath.Dir(path)

		// 组合完整命令
		fullCmd := fmt.Sprintf(
			"javac -d %s %s && java -cp %s %s",
			fileDir,   // 编译输出目录
			path,      // 源文件路径
			fileDir,   // 类路径
			className, // 主类名
		)

		return "sh", []string{"-c", fullCmd}
	case "javascript":
		return "node", []string{path}
	}
	return "", nil
}

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

	// 使用 slot.HostPath ��建文件
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

	// 构建容器内路径
	containerPath := fmt.Sprintf("/app/%s/main.%s", uniqueID, r.extension)
	duration, response.Result, err = r.runCodeContainer(request.Language, containerPath, slot)

	// Prometheus 指标
	status := "success"
	if err != nil {
		status = "error"
		response.Err = err.Error()
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
