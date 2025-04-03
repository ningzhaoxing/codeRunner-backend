package initialize

import (
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/config"
	"github.com/sirupsen/logrus"
)

func InitLogger(c *config.Config) error {

	log := logger.NewLogrusImpl(*c)

	err := log.InitLogger()
	if err != nil {
		logrus.Fatal(err)
		return err
	}
	return nil
}
