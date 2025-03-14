package config

import (
	"github.com/spf13/viper"
	"log"
)

var configPath = "./configs/configOS.yaml"

type Config struct {
	Grpc Grpc `yaml:"grpc"`
	Etcd Etcd `yaml:"etcd"`
	App  App  `yaml:"app"`
}

type Grpc struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

type Etcd struct {
	Endpoints string `yaml:"endpoints"`
	Key       string `yaml:"key"`
}

type App struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		log.Printf("infrastructure-config LoadConfig()的 viper.ReadInConfig err  %v", err)
		return nil, err
	}
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		log.Printf("infrastructure-config LoadConfig()的 viper.Unmarshal err  %v", err)
		return nil, err
	}
	return &cfg, nil
}
