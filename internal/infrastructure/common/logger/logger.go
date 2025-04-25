package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"fmt"
	"github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"io"
	"os"
	"time"
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

	// 设置日志等级
	level, err := logrus.ParseLevel(l.config.Logger.Level)
	if err != nil {
		logrus.Fatal("Invalid log level", err)
		return err
	}
	logrus.SetLevel(level)
	//设置日志格式
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
	}
	writer, err := l.getLogWriter("./logs", "codeRunner-client")
	if err != nil {
		return fmt.Errorf("failed to get log writer: %w", err)
	}
	mw := io.MultiWriter(os.Stdout, writer)
	logrus.SetOutput(mw)
	logrus.Println("hshsh")
	return nil
}

// getLogWriter 获取日志写入器
func (l *LogrusImpl) getLogWriter(logPath, appName string) (io.Writer, error) {
	if logPath == "" {
		return nil, fmt.Errorf("logPath is empty")
	}

	// 确保日志目录存在
	if err := os.MkdirAll(logPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// 创建按日期分割的日志文件
	currentDate := time.Now().Format("2006-01-02")
	fileName := fmt.Sprintf("%s/%s-%s.log", logPath, appName, currentDate)
	fmt.Printf("Log file name: %s\n", fileName) // 添加调试信息

	writer, err := rotatelogs.New(
		fileName,
		rotatelogs.WithMaxAge(30*24*time.Hour),    // 保留30天
		rotatelogs.WithRotationTime(24*time.Hour), // 每天切割
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create rotatelogs writer: %w", err)
	}

	return writer, nil
}
