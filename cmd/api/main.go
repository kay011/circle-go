package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"circle-go/api"
	"circle-go/config"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config/config.yaml", "Path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 创建API服务器
	server := api.NewServer(cfg)

	// 启动服务器
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// 优雅关闭
	if err := server.Stop(); err != nil {
		log.Fatalf("Failed to stop server: %v", err)
	}

	log.Println("Server stopped gracefully")
}
