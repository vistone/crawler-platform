//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"crawler-platform/utlsclient"
)

// 示例：如何使用 HotConnPool 接口和外部配置文件
func main() {
	// 1. 从外部配置文件加载配置
	config, whitelist, blacklist, err := utlsclient.LoadConfigFromTOML("config.toml")
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	fmt.Printf("加载配置成功: 最大连接数=%d, 连接超时=%v\n",
		config.MaxConnections, config.ConnTimeout)

	// 2. 创建热连接池（实现了 HotConnPool 接口）
	pool := utlsclient.NewUTLSHotConnPool(config)

	// 3. 如果有白名单，设置白名单
	if len(whitelist) > 0 {
		// 这里需要添加设置白名单的方法
		fmt.Printf("设置白名单: %v\n", whitelist)
	}

	// 4. 如果有黑名单，设置黑名单
	if len(blacklist) > 0 {
		// 这里需要添加设置黑名单的方法
		fmt.Printf("设置黑名单: %v\n", blacklist)
	}

	// 5. 使用接口进行操作
	var hotPool utlsclient.HotConnPool = pool // 接口赋值

	// 6. 方式1：获取普通连接（只验证根路径）
	conn, err := hotPool.GetConnection("kh.google.com")
	if err != nil {
		log.Printf("获取连接失败: %v", err)
		return
	}

	fmt.Printf("获取到连接: %s -> %s\n", conn.TargetHost(), conn.TargetIP())

	// 7. 创建 UTLSClient 进行HTTP请求
	client := utlsclient.NewUTLSClient(conn)
	client.SetDebug(true) // 开启调试模式

	// 8. 测试指定地址 - 这个地址应该返回13个字节
	testURL := "https://kh.google.com/rt/earth/PlanetoidMetadata"
	resp, err := client.Get(testURL)
	if err != nil {
		log.Printf("HTTP GET请求失败: %v", err)
	} else {
		fmt.Printf("HTTP响应状态: %s\n", resp.Status)
		// 读取响应体
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("读取响应体失败: %v", err)
		} else {
			bodyLen := len(body)
			fmt.Printf("响应体长度: %d 字节\n", bodyLen)

			if bodyLen == 13 {
				fmt.Printf("✅ 测试成功！响应体长度正确 (13字节)\n")
				fmt.Printf("响应内容: %v\n", body)
			} else {
				fmt.Printf("❌ 测试失败！期望13字节，实际%d字节\n", bodyLen)
			}
		}
	}

	// 9. 方式2：获取连接并验证指定路径（推荐）
	conn2, err := hotPool.GetConnectionWithValidation(testURL)
	if err != nil {
		log.Printf("获取带验证的连接失败: %v", err)
	} else {
		fmt.Printf("获取到带验证的连接: %s -> %s (已验证 /rt/earth/PlanetoidMetadata 路径)\n",
			conn2.TargetHost(), conn2.TargetIP())

		// 创建新的客户端
		client2 := utlsclient.NewUTLSClient(conn2)
		client2.SetTimeout(10 * time.Second)

		// 使用客户端再次请求
		resp, err := client2.Get(testURL)
		if err != nil {
			log.Printf("第二次请求失败: %v", err)
		} else {
			fmt.Printf("第二次响应状态: %s\n", resp.Status)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if len(body) == 13 {
				fmt.Printf("✅ 带验证连接测试成功！\n")
			} else {
				fmt.Printf("❌ 带验证连接测试失败，长度: %d\n", len(body))
			}
		}

		hotPool.PutConnection(conn2)
	}

	// 10. 归还连接
	hotPool.PutConnection(conn)

	// 11. 查看统计信息
	stats := hotPool.GetStats()
	fmt.Printf("连接池统计: 总连接数=%d, 健康连接数=%d\n",
		stats.TotalConnections, stats.HealthyConnections)

	// 12. 检查健康状态
	if hotPool.IsHealthy() {
		fmt.Println("连接池状态健康")
	} else {
		fmt.Println("连接池状态不健康")
	}

	// 13. 关闭连接池
	err = hotPool.Close()
	if err != nil {
		log.Printf("关闭连接池失败: %v", err)
	}
}

// 简化版本：只加载连接池配置
func simplifiedExample() {
	// 只加载连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("config.toml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建连接池
	pool := utlsclient.NewUTLSHotConnPool(config)

	// 使用连接池...
	defer pool.Close()
}

// 路径验证示例
func pathValidationExample() {
	// 加载配置
	config, err := utlsclient.LoadPoolConfigFromFile("config.toml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	// 测试不同路径的连接
	testURLs := []string{
		"https://api.example.com/health",
		"https://api.example.com/api/v1/data",
		"https://api.example.com/api/v2/status",
	}

	for _, testURL := range testURLs {
		conn, err := pool.GetConnectionWithValidation(testURL)
		if err != nil {
			log.Printf("URL %s 连接失败: %v", testURL, err)
			continue
		}

		fmt.Printf("URL %s 连接成功: %s -> %s\n",
			testURL, conn.TargetHost(), conn.TargetIP())

		// 使用连接进行实际请求
		req := &http.Request{
			Method: "GET",
			URL:    parseURL(testURL),
			Header: make(http.Header),
			Host:   conn.TargetHost(),
		}

		// 创建 UTLSClient 进行HTTP请求
		client := utlsclient.NewUTLSClient(conn)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("请求失败: %v", err)
		} else {
			fmt.Printf("  响应状态: %s\n", resp.Status)
			resp.Body.Close()
		}

		pool.PutConnection(conn)
	}
}

// 高级用法：自定义请求头和超时
func advancedExample() {
	config, err := utlsclient.LoadPoolConfigFromFile("config.toml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	// 获取连接
	conn, err := pool.GetConnection("api.example.com")
	if err != nil {
		log.Fatalf("获取连接失败: %v", err)
	}
	defer pool.PutConnection(conn)

	// 创建自定义请求
	req := &http.Request{
		Method: "GET",
		URL:    &url.URL{Scheme: "https", Host: "api.example.com", Path: "/api/v1/data"},
		Header: make(http.Header),
		Host:   "api.example.com",
	}

	// 设置自定义请求头
	req.Header.Set("Authorization", "Bearer your-token")
	req.Header.Set("X-API-Key", "your-api-key")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", conn.Fingerprint().UserAgent)

	// 创建 UTLSClient 进行HTTP请求
	client := utlsclient.NewUTLSClient(conn)
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("请求失败: %v", err)
		return
	}
	defer resp.Body.Close()

	// 处理响应
	fmt.Printf("响应状态: %s\n", resp.Status)
	fmt.Printf("响应头: %v\n", resp.Header)

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("读取响应体失败: %v", err)
		return
	}

	fmt.Printf("响应体: %s\n", string(body))

	// 查看连接统计
	stats := conn.Stats()
	fmt.Printf("连接统计: 请求次数=%d, 错误次数=%d, 健康状态=%t\n",
		stats.RequestCount, stats.ErrorCount, stats.IsHealthy)
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseURL(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("解析URL失败: %v", err)
		return &url.URL{}
	}
	return u
}
