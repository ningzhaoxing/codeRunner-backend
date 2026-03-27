package initialize

import (
	"codeRunner-siwu/internal/infrastructure/common/logger"
	"codeRunner-siwu/internal/infrastructure/config"
	"log"
)

func InitLogger(c *config.Config) error {
	log := logger.NewZapImpl(c)

	err := log.InitLogger()

	if err != nil {
		log.Fatal(err)
		return err
	}
	return nil
}
