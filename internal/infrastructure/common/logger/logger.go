package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"github.com/lestrrat-go/file-rotatelogs"
	"github.com/sirupsen/logrus"
	"time"
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
	writer, _ := rotatelogs.New(
		"/study/log/app-%Y%m%d.log",
		rotatelogs.WithMaxAge(30*24*time.Hour),    // 保留7天
		rotatelogs.WithRotationTime(24*time.Hour), // 每天切割
	)
	defer writer.Close()

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

	logrus.SetOutput(writer)
	return nil
}
