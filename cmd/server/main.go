package main

import (
	"log"

	"modelgate/internal/config"
	"modelgate/internal/server"
)

func main() {
	// 1. 加载基础配置
	cfgPath := "config.yaml"
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. 初始化并启动服务器
	srv := server.NewServer(cfg, cfgPath)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
