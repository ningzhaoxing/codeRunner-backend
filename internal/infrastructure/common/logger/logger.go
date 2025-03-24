package logger

import "github.com/sirupsen/logrus"

type Logger interface {
	Info(msg string)
	Error(msg string)
	Panic(msg string)
}

type LoggerTmpl struct {
}

func NewLoggerTmpl() *LoggerTmpl {
	return &LoggerTmpl{}
}

func (l *LoggerTmpl) Info(msg string) {
	logrus.Info(msg)
}

func (l *LoggerTmpl) Error(msg string) {
	logrus.Error(msg)
}

func (l *LoggerTmpl) Panic(msg string) {
	logrus.Panic(msg)
}
