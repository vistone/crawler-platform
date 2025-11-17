//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"io"
	"log"
	"time"

	"crawler-platform/utlsclient"
)

// 示例：如何使用 UTLSClient 进行 HTTP 请求
func main() {
	// 1. 从配置文件加载连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("config.toml")
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	// 2. 创建热连接池
	pool := utlsclient.NewUTLSHotConnPool(config)

	// 3. 获取连接到测试服务器
	conn, err := pool.GetConnection("kh.google.com")
	if err != nil {
		log.Fatalf("获取连接失败: %v", err)
	}
	defer pool.PutConnection(conn)

	// 4. 创建 UTLSClient
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(10 * time.Second)
	client.SetDebug(true) // 开启调试模式查看详细请求

	// 5. 测试指定地址 - 这个地址应该返回13个字节
	fmt.Println("\n=== 测试指定地址 ===")
	testURL := "https://kh.google.com/rt/earth/PlanetoidMetadata"
	resp, err := client.Get(testURL)
	if err != nil {
		log.Printf("测试请求失败: %v", err)
	} else {
		defer resp.Body.Close()
		fmt.Printf("状态码: %d\n", resp.StatusCode)

		// 读取响应体
		body, err := io.ReadAll(resp.Body)
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
				fmt.Printf("响应内容前100字节: %s\n", string(body[:min(100, bodyLen)]))
			}
		}
	}

	// 6. 使用带路径验证的连接获取
	fmt.Println("\n=== 带路径验证的连接示例 ===")
	conn2, err := pool.GetConnectionWithValidation(testURL)
	if err != nil {
		log.Printf("获取带验证的连接失败: %v", err)
	} else {
		fmt.Printf("成功获取已验证的连接: %s -> %s\n",
			conn2.TargetHost(), conn2.TargetIP())

		// 使用新连接再次请求
		client2 := utlsclient.NewUTLSClient(conn2)
		client2.SetDebug(false) // 关闭调试模式
		resp2, err := client2.Get(testURL)
		if err != nil {
			log.Printf("第二次请求失败: %v", err)
		} else {
			defer resp2.Body.Close()
			body2, _ := io.ReadAll(resp2.Body)
			fmt.Printf("第二次请求 - 状态码: %d, 长度: %d字节\n",
				resp2.StatusCode, len(body2))

			if len(body2) == 13 {
				fmt.Printf("✅ 带验证连接测试成功！\n")
			}
		}

		pool.PutConnection(conn2)
	}

	// 9. 查看连接统计信息
	stats := conn.Stats()
	fmt.Printf("\n=== 连接统计 ===\n")
	fmt.Printf("目标主机: %s\n", stats.TargetHost)
	fmt.Printf("目标IP: %s\n", stats.TargetIP)
	fmt.Printf("TLS指纹: %s\n", stats.Fingerprint)
	fmt.Printf("创建时间: %v\n", stats.Created)
	fmt.Printf("最后使用: %v\n", stats.LastUsed)
	fmt.Printf("请求次数: %d\n", stats.RequestCount)
	fmt.Printf("错误次数: %d\n", stats.ErrorCount)
	fmt.Printf("健康状态: %v\n", stats.IsHealthy)

	// 10. 关闭连接池
	pool.Close()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
