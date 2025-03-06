package initialize

import "codeRunner-siwu/internal/infrastructure/config"

func InitConfig() (*config.Config, error) {
	c, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}
	return c, nil
}
