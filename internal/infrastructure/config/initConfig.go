package config

import "github.com/spf13/viper"

var configPath = "./configs/config.yaml"

type Config struct {
	Grpc Grpc `yaml:"grpc"`
	Etcd Etcd `yaml:"etcd"`
}

type Grpc struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

type Etcd struct {
	Endpoints string `yaml:"endpoints"`
	Key       string `yaml:"key"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
