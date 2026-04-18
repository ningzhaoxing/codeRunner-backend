package config

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"github.com/spf13/viper"
	"codeRunner-siwu/internal/agent"
)

var configPath = "./configs/product.yaml"

type Config struct {
	Server ServerConfig      `yaml:"server"`
	Client ClientConfig      `yaml:"client"`
	Logger LoggerConfig      `yaml:"log"`
	Agent  agent.AgentConfig `yaml:"agent"`
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
	ContainerPool ContainerPoolConfig `yaml:"container_pool"`
}

type ContainerPoolConfig struct {
	Golang     int `yaml:"golang"`
	Python     int `yaml:"python"`
	JavaScript int `yaml:"javascript"`
	Java       int `yaml:"java"`
	C          int `yaml:"c"`
}

// ToPoolSizes 转换为 map[string]int，供 NewContainerPool 使用
func (c ContainerPoolConfig) ToPoolSizes() map[string]int {
	m := map[string]int{
		"golang":     c.Golang,
		"python":     c.Python,
		"javascript": c.JavaScript,
		"java":       c.Java,
		"c":          c.C,
	}
	// 默认值：未配置时每种语言 1 个容器（退化为当前行为）
	for k, v := range m {
		if v <= 0 {
			m[k] = 1
		}
	}
	return m
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

	// 展开配置中的 ${ENV_VAR} 引用
	for _, key := range viper.AllKeys() {
		val := viper.GetString(key)
		if val != "" && val != os.ExpandEnv(val) {
			viper.Set(key, os.ExpandEnv(val))
		}
	}

	if err := viper.Unmarshal(config); err != nil {
		zap.S().Error("infrastructure-config LoadConfig()的 viper.Unmarshal err  %v", err)
		return err
	}
	fmt.Println(config)
	return nil
}
