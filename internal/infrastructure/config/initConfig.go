package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"go.uber.org/zap"
	"github.com/spf13/viper"
	"codeRunner-siwu/internal/agent"
)

var configPath = "./configs/product.yaml"

type Config struct {
	Server   ServerConfig      `yaml:"server"`
	Client   ClientConfig      `yaml:"client"`
	Logger   LoggerConfig      `yaml:"log"`
	Agent    agent.AgentConfig `yaml:"agent"`
	Mail     MailConfig        `yaml:"mail"`
	Feedback FeedbackConfig    `yaml:"feedback"`
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

type MailConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Host        string        `yaml:"host"`
	Port        int           `yaml:"port"`
	Username    string        `yaml:"username"`
	Password    string        `yaml:"password"`
	From        string        `yaml:"from"`
	To          string        `yaml:"to"`
	SendTimeout time.Duration `yaml:"send_timeout"`
}

type FeedbackConfig struct {
	RateLimitPerMin int `yaml:"rate_limit_per_min"`
	RateLimitPerDay int `yaml:"rate_limit_per_day"`
}

func LoadConfig(config *Config) error {
	// 读取 yaml 文件并展开 ${ENV_VAR} 引用
	raw, err := os.ReadFile(configPath)
	if err != nil {
		zap.S().Error("infrastructure-config LoadConfig() read file err %v", err)
		return err
	}
	expanded := os.ExpandEnv(string(raw))

	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(expanded)); err != nil {
		zap.S().Error("infrastructure-config LoadConfig()的 viper.ReadConfig err %v", err)
		return err
	}

	if err := viper.Unmarshal(config, func(dc *mapstructure.DecoderConfig) {
		dc.TagName = "yaml"
	}); err != nil {
		zap.S().Error("infrastructure-config LoadConfig()的 viper.Unmarshal err  %v", err)
		return err
	}
	fmt.Println(config)
	return nil
}
