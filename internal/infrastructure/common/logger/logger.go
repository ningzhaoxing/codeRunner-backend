package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"log"
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
	// 确保日志目录存在
	logDir := "./logs"
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return err
		}
	}

	// 初始化 rotatelogs，检查错误
	writer, err := rotatelogs.New(
		logDir+"/app-%Y%m%d.log",
		rotatelogs.WithMaxAge(30*24*time.Hour),
		rotatelogs.WithRotationTime(24*time.Hour),
	)
	if err != nil {
		log.Fatalf("初始化rotatelogs失败: %v", err)
	}

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

	logrus.SetOutput(writer)
	return nil
}
