//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"time"

	"crawler-platform/utlsclient"
)

func main() {
	// 创建连接池配置
	config := utlsclient.DefaultPoolConfig()
	
	// 可以自定义配置
	config.MaxConnections = 50
	config.BlacklistCheckInterval = 5 * time.Minute  // 5分钟检查一次黑名单
	config.DNSUpdateInterval = 30 * time.Minute       // 30分钟更新一次DNS
	
	// 创建热连接池
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()
	
	fmt.Println("=== UTLS热连接池示例 ===")
	
	// 模拟设置依赖（实际使用中需要注入真实的依赖）
	// pool.SetDependencies(fingerprintLib, ipPool, accessControl)
	
	// 获取连接池统计信息
	stats := pool.GetStats()
	fmt.Printf("初始连接池状态:\n")
	fmt.Printf("  总连接数: %d\n", stats.TotalConnections)
	fmt.Printf("  黑名单IP数: %d\n", stats.BlacklistIPs)
	fmt.Printf("  白名单IP数: %d\n", stats.WhitelistIPs)
	fmt.Printf("  黑名单移到白名单数量: %d\n", stats.WhitelistMoves)
	fmt.Printf("  DNS更新新增连接数: %d\n", stats.NewConnectionsFromDNS)
	
	// 演示智能维护机制
	fmt.Println("\n=== 智能维护机制 ===")
	fmt.Println("1. 黑名单定期检查:")
	fmt.Println("   - 每5分钟检查一次黑名单IP")
	fmt.Println("   - 如果IP返回200状态码，自动移到白名单")
	fmt.Println("   - 统计移动的IP数量")
	
	fmt.Println("\n2. DNS热更新机制:")
	fmt.Println("   - 每30分钟获取域名的最新IP")
	fmt.Println("   - 为新发现的IP自动建立热连接")
	fmt.Println("   - 统计新增的连接数量")
	
	// 演示连接获取（需要真实的依赖才能工作）
	fmt.Println("\n=== 连接使用示例 ===")
	fmt.Println("获取连接示例代码:")
	fmt.Println("  conn, err := pool.GetConnection(\"www.example.com\")")
	fmt.Println("  if err != nil {")
	fmt.Println("      log.Printf(\"获取连接失败: %v\", err)")
	fmt.Println("      return")
	fmt.Println("  }")
	fmt.Println("  defer pool.PutConnection(conn)")
	fmt.Println("")
	fmt.Println("  // 使用连接进行HTTP请求")
	fmt.Println("  resp, err := conn.Do(request)")
	
	// 演示监控功能
	fmt.Println("\n=== 监控统计 ===")
	fmt.Println("实时监控连接池状态:")
	for i := 0; i < 3; i++ {
		time.Sleep(1 * time.Second)
		stats := pool.GetStats()
		fmt.Printf("  [%d] 连接数: %d, 成功率: %.2f%%\n", 
			i+1, stats.TotalConnections, stats.SuccessRate*100)
	}
	
	fmt.Println("\n=== 热加载优势 ===")
	fmt.Println("✓ 实时更新: 无需重启服务即可更新IP池")
	fmt.Println("✓ 增量处理: 只处理新增和变化的IP，提高效率")
	fmt.Println("✓ 自动恢复: 自动恢复之前被误封的IP")
	fmt.Println("✓ 智能调度: 后台异步处理，不影响主业务流程")
	
	fmt.Println("\n示例程序结束")
}
