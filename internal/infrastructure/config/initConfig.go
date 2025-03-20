package config

import (
	"github.com/spf13/viper"
	"log"
)

var configPath = "./configs/config.yaml"

type Config struct {
	Server ServerConfig `yaml:"server"`
	Client ClientConfig `yaml:"client"`
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

func LoadConfig(config *Config) error {
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("infrastructure-config LoadConfig()的 viper.ReadInConfig err  %v", err)
		return err
	}

	if err := viper.Unmarshal(config); err != nil {
		log.Printf("infrastructure-config LoadConfig()的 viper.Unmarshal err  %v", err)
		return err
	}
	return nil
}
