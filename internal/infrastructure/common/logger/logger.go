package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"github.com/sirupsen/logrus"
	"os"
)

type Logger interface {
	InitLogger() error
}

type LogrusImpl struct {
	config config.Config
}

func NewLogrusImpl(config config.Config) *LogrusImpl {
	return &LogrusImpl{config: config}
}

func (l *LogrusImpl) InitLogger() error {
	// 创建一个文件用于存储日志
	logFile, err := os.OpenFile(l.config.Logger.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		logrus.Fatalf("The log file could not be opened: %v", err)
	}
	defer logFile.Close()

	// 设置日志等级
	level, err := logrus.ParseLevel(l.config.Logger.Level)
	if err != nil {
		logrus.Fatal("Invalid log level", err)
		return err
	}
	logrus.SetLevel(level)

	// 设置日志格式
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

	logrus.SetOutput(logFile)
	return nil
}
