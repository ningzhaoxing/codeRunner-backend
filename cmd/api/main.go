package main

import (
	"codeRunner-siwu/internal/interfaces/adapter/initialize"
	"os"
)

func main() {
	mode := os.Getenv("APP_MODE")
	if mode == "server" {
		initialize.RunServer()
	} else {
		initialize.RunClient()
	}
}
