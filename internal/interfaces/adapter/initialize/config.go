package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"log"
)

func InitConfig() (*config.Config, error) {
	c, err := config.LoadConfig()
	if err != nil {
		log.Println("interfaces-adapter-initialize-config InitConfig() 的config.LoadConfig() err=", err)
		return nil, err
	}
	return c, nil
}
