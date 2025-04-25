package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"github.com/sirupsen/logrus"
	"io"
	"os"
)

type Logger interface {
	InitLogger() error
}

type LogrusImpl struct {
	config *config.Config
}

func NewLogrusImpl(config *config.Config) *LogrusImpl {
	return &LogrusImpl{config: config}
}

func (l *LogrusImpl) InitLogger() error {
	// 解析配置文件中的日志等级
	logLevel, err := logrus.ParseLevel(l.config.Logger.Level)
	if err != nil {
		logrus.Fatalf("Invalid log level: %v", err)
		return err
	}
	// 设置日志等级
	logrus.SetLevel(logLevel)

	// 根据配置文件设置日志格式
	switch l.config.Logger.Format {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	default:
		logrus.Warn("Unsupported log format, using text as default")
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	}

	// 初始化日志输出，这里仅输出到标准输出
	mw := io.MultiWriter(os.Stdout)
	logrus.SetOutput(mw)

	// 输出测试日志信息
	logrus.Println("hshsh")

	return nil
}

// getLogWriter 获取日志写入器
//func (l *LogrusImpl) getLogWriter(logPath, appName string) (io.Writer, error) {
//	if logPath == "" {
//		return nil, fmt.Errorf("logPath is empty")
//	}
//
//	// 转换为绝对路径（避免相对路径的歧义）
//	absLogPath, err := filepath.Abs(logPath)
//	if err != nil {
//		logrus.Errorf("解析日志路径失败: %v", err)
//		return nil, fmt.Errorf("解析日志路径失败: %w", err)
//	}
//
//	// 创建目录（确保权限为 0755）
//	if err := os.MkdirAll(absLogPath, 0755); err != nil {
//		logrus.Errorf("创建日志目录失败: %v", err)
//		return nil, fmt.Errorf("创建日志目录失败: %w", err)
//	}
//
//	// 关键点：使用双重转义生成时间占位符
//	fileNamePattern := fmt.Sprintf("%s/%s-%%Y-%%m-%%d.log", absLogPath, appName)
//	linkName := fmt.Sprintf("%s/%s-current.log", absLogPath, appName)
//
//	// 创建 rotatelogs 实例
//	writer, err := rotatelogs.New(
//		fileNamePattern,
//		rotatelogs.WithLinkName(linkName),         // 符号链接
//		rotatelogs.WithRotationTime(24*time.Hour), // 每天切割
//		rotatelogs.WithMaxAge(30*24*time.Hour),    // 保留30天
//	)
//	if err != nil {
//		logrus.Errorf("创建rotatelogs失败: %v", err)
//		return nil, fmt.Errorf("创建rotatelogs失败: %w", err)
//	}
//
//	return writer, nil
//}
