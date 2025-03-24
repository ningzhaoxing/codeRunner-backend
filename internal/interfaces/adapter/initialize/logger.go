package initialize

import (
	"github.com/sirupsen/logrus"
	"os"
)

func InitLogger() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetLevel(logrus.InfoLevel) // 设置日志级别为 Info
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})

	file, err := os.OpenFile("web_app.logger", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic("日志文件打开错误")
	}

	logrus.SetOutput(file)
}
