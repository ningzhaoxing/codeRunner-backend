package config

import (
	"fmt"

	"go.uber.org/zap"
	"github.com/spf13/viper"
)

var configPath = "./configs/product.yaml"

type Config struct {
	Server ServerConfig `yaml:"server"`
	Client ClientConfig `yaml:"client"`
	Logger LoggerConfig `yaml:"log"`
}

type ServerConfig struct {
	Grpc struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
	} `yaml:"grpc"`
	Etcd struct {
		Endpoints string `yaml:"endpoints"`
		Key       string `yaml:"key"`
	} `yaml:"etcd"`
	App struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
	} `yaml:"app"`
}

type ClientConfig struct {
	Server struct {
		Host string `yaml:"host"`
		Port string `yaml:"port"`
		Path string `yaml:"path"`
	} `yaml:"server"`
	App struct {
		Weight int64 `yaml:"weight"`
	} `yaml:"app"`
}

type LoggerConfig struct {
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	Path          string `yaml:"path"`
	EnableConsole bool   `yaml:"enable_console"`
}

func LoadConfig(config *Config) error {
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		zap.S().Error("infrastructure-config LoadConfig()的 viper.ReadInConfig err  %v", err)
		return err
	}

	if err := viper.Unmarshal(config); err != nil {
		zap.S().Error("infrastructure-config LoadConfig()的 viper.Unmarshal err  %v", err)
		return err
	}
	fmt.Println(config)
	return nil
}
