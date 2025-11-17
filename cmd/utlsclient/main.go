package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	projlogger "crawler-platform/logger"
	"crawler-platform/utlsclient"
)

// Version 项目版本号
const Version = "0.0.12"

func main() {
	// 命令行参数
	url := flag.String("url", "", "目标 HTTPS URL，例如 https://example.com/path")
	method := flag.String("X", "GET", "HTTP 方法：GET/POST/HEAD 等")
	ua := flag.String("ua", "", "自定义 User-Agent")
	timeout := flag.Duration("timeout", 30*time.Second, "请求超时时间")
	head := flag.Bool("head", false, "使用 HEAD 请求进行快速探测")
	version := flag.Bool("version", false, "显示版本号")
	flag.Parse()

	if *version {
		fmt.Printf("crawler-platform v%s\n", Version)
		os.Exit(0)
	}

	if *url == "" {
		fmt.Fprintln(os.Stderr, "必须提供 --url")
		os.Exit(2)
	}

	// 初始化日志（可按需定制）
	projlogger.SetGlobalLogger(&projlogger.DefaultLogger{})

	// 创建热连接池（使用默认配置）
	pool := utlsclient.NewUTLSHotConnPool(nil)

	// 进程退出时优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-quit
		pool.Close()
		os.Exit(0)
	}()

	// 从池中获取并验证连接（按完整URL）
	conn, err := pool.GetConnectionWithValidation(*url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取连接失败: %v\n", err)
		os.Exit(1)
	}

	// 在该热连接上创建客户端并发起请求
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(*timeout)
	if *ua != "" {
		client.SetUserAgent(*ua)
	}

	var req *http.Request
	if *head {
		req, err = http.NewRequest("HEAD", *url, nil)
	} else {
		req, err = http.NewRequest(*method, *url, nil)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造请求失败: %v\n", err)
		pool.PutConnection(conn)
		os.Exit(1)
	}
	if *ua != "" {
		req.Header.Set("User-Agent", *ua)
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
		pool.PutConnection(conn)
		os.Exit(1)
	}
	defer resp.Body.Close()

	fmt.Printf("状态: %s\n", resp.Status)
	// 读取并显示响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取响应体失败: %v\n", err)
	} else {
		fmt.Printf("响应体长度: %d 字节\n", len(body))
		if len(body) > 0 && len(body) <= 200 {
			fmt.Printf("响应体内容: %q\n", string(body))
		} else if len(body) > 200 {
			fmt.Printf("响应体前200字节: %q\n", string(body[:200]))
		}
	}
	fmt.Println()

	// 用后显式归还热连接，确保可复用
	pool.PutConnection(conn)

	// 可选：示例程序结束时关闭连接池
	pool.Close()
}
