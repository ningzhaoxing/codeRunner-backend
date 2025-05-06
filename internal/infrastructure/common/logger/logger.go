package logger

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Logger interface {
	InitLogger() error
}

type ZapImpl struct {
	config *config.Config
}

func NewZapImpl(config *config.Config) *ZapImpl {
	return &ZapImpl{config: config}
}

func (l *ZapImpl) InitLogger() error {
	//fmt.Printf("LogPath: %s, AppName: %s, Level: %d\n", LogPath, AppName, Level)
	writeSyncer := l.getLogWriter("logs", "coderunner")

	encoder := l.getEncoder()
	level := l.getLevel(l.config.Logger.Level)
	// 控制台输出
	consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(zapcore.Lock(os.Stdout)), level)

	var core zapcore.Core
	if writeSyncer != nil {
		// 文件输出
		fileCore := zapcore.NewCore(encoder, writeSyncer, level)
		core = zapcore.NewTee(consoleCore, fileCore)
	} else {
		core = consoleCore
		fmt.Println("文件日志初始化失败，仅使用控制台输出")
		return errors.New("文件日志初始化失败，仅使用控制台输出")
	}

	logger := zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(logger)
	// 替换全局log输出（新增部分）
	stdLog := zap.NewStdLog(logger)
	log.SetFlags(0) // 去掉标准库的时间前缀
	log.SetOutput(stdLog.Writer())
	return nil
}

func (l *ZapImpl) getLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func (l *ZapImpl) getEncoder() zapcore.Encoder {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = l.customTimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // 添加彩色编码
	return zapcore.NewConsoleEncoder(encoderConfig)              // 改用Console编码器
}
func (l *ZapImpl) getLogWriter(logPath, appName string) zapcore.WriteSyncer {
	if appName == "" {
		appName = "default"
	}

	if logPath != "" {
		if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
			fmt.Printf("创建日志目录失败: %v\n", err)
			return nil
		}
	}

	currentDate := time.Now().Format("2006-01-02")
	fileName := filepath.Join(logPath, fmt.Sprintf("%s-%s.log", appName, currentDate))
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("创建日志文件失败: %v\n", err)
		return nil
	}
	return zapcore.AddSync(file)
}

func (l *ZapImpl) customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	zoneName, offset := t.Zone()
	offsetHours := offset / 3600

	var zoneStr string
	switch offsetHours {
	case 8:
		zoneStr = "东八区"
	case 7:
		zoneStr = "东七区"
	case -8:
		zoneStr = "西八区"
	default:
		zoneStr = fmt.Sprintf("%s%+d", zoneName, offsetHours)
	}

	timeStr := t.Format("2006年1月2日 15时04分05秒") + t.Format(".000")[1:] + "毫秒 " + zoneStr
	enc.AppendString(timeStr)
}
