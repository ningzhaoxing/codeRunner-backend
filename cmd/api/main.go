package main

import (
	"codeRunner-siwu/internal/interfaces/adapter/initialize"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // 加载 .env 文件（不存在时忽略）
	mode := os.Getenv("APP_MODE")
	if mode == "server" {
		initialize.RunServer()
	} else {
		initialize.RunClient()
	}
}
