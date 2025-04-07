package containerBasic

import (
	"codeRunner-siwu/api/proto"
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Container interface {
	RunCode(request *proto.ExecuteRequest) (duration float64, response proto.ExecuteResponse, err error)
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
		log.Println("获取拓展名失败 err=", r.err)
		return r
	}
	r.err = os.MkdirAll(path, 0775)
	r.path = path
	return r
}

// 创建空文件
func (r *runCode) createBlockFile() *runCode {
	if r.err != nil {
		log.Println("创建UUid目录失败 err=", r.err)
		return r
	}
	codePath := fmt.Sprintf("%s/main.%s", r.path, r.extension)
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
func (r *runCode) createFile(language, code, path string) error {
	if r.err != nil {
		return r.err
	}
	r.getFileExtension(language).createBlockContent(path).createBlockFile().writeCode(code).sync()
	if r.err != nil {
		log.Println("runCode-createFile 的 err=", r.err)
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

func (r *runCode) runCodeContainer(language, path string) (float64, string, error) {
	cmd, args := r.getCommand(language, path)
	if cmd == "" {
		return 0, "", fmt.Errorf("不支持的语言类型: %s", language)
	}
	duration, logContent, err := r.InContainerRunCode(language, cmd, args)
	if err != nil {
		return 0, "", err
	}
	return duration, logContent, nil
}

func (r *runCode) RunCode(request *proto.ExecuteRequest) (duration float64, response proto.ExecuteResponse, err error) {
	response.Id = request.Id
	response.Uid = request.Uid
	response.CallBackUrl = request.CallBackUrl
	//创建文件
	uniqueID := uuid.New().String()
	path := fmt.Sprintf("/app/tmp/%s/%s", request.Language, uniqueID)
	//创建文件
	err = r.createFile(request.Language, request.CodeBlock, path)
	if err != nil {
		logrus.Error(" containerBasic-RunCode-createFile err=", err)
		return 0, response, err
	}
	//删除目录
	defer func() {
		r.file.Close()
		err = os.RemoveAll(r.path)
		if err != nil {
			logrus.Error("删除文件夹失败,err=", err)
			return
		}
	}()
	//构建文件路径
	containerPath := fmt.Sprintf("/app/%s/main.%s", uniqueID, r.extension)
	duration, response.Result, err = r.runCodeContainer(request.Language, containerPath)
	if err != nil {
		response.Err = err.Error()
		return 0, response, err
	}
	return duration, response, nil
}
