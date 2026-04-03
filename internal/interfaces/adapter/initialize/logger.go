package initialize

import (
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/config"
)

func InitLogger(c *config.Config) error {
	l := logger.NewZapImpl(c)
	return l.InitLogger()
}
