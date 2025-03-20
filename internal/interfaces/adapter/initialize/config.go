package initialize

import (
	"codeRunner-siwu/internal/infrastructure/config"
	"log"
)

func InitConfig() (*config.Config, error) {
	appConfig := new(config.Config)
	if err := config.LoadConfig(appConfig); err != nil {
		log.Println("interfaces-adapter-initialize-config InitConfig() 的config.LoadConfig() err=", err)
		return nil, err
	}
	return appConfig, nil
}
