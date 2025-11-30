package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	server "crawler-platform/cmd/grpcserver/internal"
	"crawler-platform/logger"
)

func main() {
	// 解析命令行参数
	address := flag.String("address", "0.0.0.0", "服务器监听地址")
	port := flag.String("port", "50051", "服务器监听端口")
	bootstrap := flag.String("bootstrap", "", "引导节点地址（多个地址用逗号分隔，例如: localhost:50051,localhost:50052）")
	flag.Parse()

	// 初始化日志记录器
	logger.InitGlobalLogger(logger.NewConsoleLogger(true, true, true, true))

	// 创建服务器实例
	srv := server.NewServer(*address, *port)

	// 解析引导节点地址
	var bootstrapAddresses []string
	if *bootstrap != "" {
		// 简单的逗号分隔解析
		addrs := *bootstrap
		for _, addr := range splitAndTrim(addrs, ",") {
			if addr != "" {
				bootstrapAddresses = append(bootstrapAddresses, addr)
			}
		}
		if len(bootstrapAddresses) > 0 {
			log.Printf("引导节点: %v", bootstrapAddresses)
			srv.SetBootstrapNodes(bootstrapAddresses)
		}
	}

	// 启动服务器（在 goroutine 中）
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("正在关闭服务器...")
	srv.Stop()
	log.Println("服务器已关闭")
}

// splitAndTrim 分割字符串并去除空白
func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// splitString 简单的字符串分割
func splitString(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	result = append(result, s[start:])
	return result
}

// trimSpace 去除字符串两端的空白字符
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
