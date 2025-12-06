package main

import (
	"context"
	server "crawler-platform/cmd/grpcserver/internal"
	"crawler-platform/localippool"
	"crawler-platform/logger"
	"crawler-platform/remotedomainippool"
	"crawler-platform/utlsclient"
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 获取配置文件路径（可通过环境变量 GRPCSERVER_CONFIG 指定，默认为 config.toml）
	configPath := os.Getenv("GRPCSERVER_CONFIG")
	if configPath == "" {
		// 默认使用当前目录下的 config.toml
		configPath = "./cmd/grpcserver/config.toml"
	}

	// 检查配置文件是否存在
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Printf("警告: 配置文件 %s 不存在，将使用默认配置", configPath)
		configPath = "" // 使用默认配置
	}

	// 加载配置文件
	config, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	if configPath != "" {
		log.Printf("已加载配置文件: %s", configPath)
	} else {
		log.Println("使用默认配置")
	}

	// 初始化日志记录器（根据配置文件）
	logger.InitGlobalLogger(logger.NewConsoleLogger(
		config.Logger.EnableDebug,
		config.Logger.EnableInfo,
		config.Logger.EnableWarn,
		config.Logger.EnableError,
	))

	// 加载 TLS 证书（根据配置）
	var tlsConfig *tls.Config
	if config.TLS.Enable {
		certsDir := config.GetCertsDir()
		if certsDir != "" {
			tlsConfig, err = server.LoadTLSConfigFromCertsDir(certsDir)
			if err != nil {
				log.Fatalf("加载 TLS 证书失败: %v", err)
			}
			log.Printf("已加载 TLS 证书，证书目录: %s", certsDir)
		} else {
			log.Fatal("TLS 已启用但未指定证书目录")
		}
	}

	// 创建服务器实例
	var srv *server.Server
	if tlsConfig != nil {
		srv = server.NewServerWithTLS(config.Server.Address, config.Server.Port, tlsConfig)
	} else {
		srv = server.NewServer(config.Server.Address, config.Server.Port)
	}

	// 设置引导节点（根据配置文件）
	if len(config.Server.Bootstrap) > 0 {
		log.Printf("引导节点: %v", config.Server.Bootstrap)
		srv.SetBootstrapNodes(config.Server.Bootstrap)
	}

	// 初始化并启动本地 IP 池
	// 如果配置启用，或配置了IPv6子网（即使enable=false），则创建本地IP池
	// localippool 包内置了完整的自动检测和管理功能，包括：
	// - 自动检测 IPv6 子网（如果 ipv6SubnetCIDR 为空）
	// - 自动创建、删除、管理 IPv6 地址
	// - 自动实现地址轮流使用
	var localIPPool localippool.IPPool
	shouldEnableLocalIPPool := config.LocalIPPool.Enable

	// 如果配置未启用，但配置了IPv6子网，自动启用本地IP池
	if !shouldEnableLocalIPPool && config.LocalIPPool.IPv6SubnetCIDR != "" {
		log.Printf("检测到IPv6子网配置，将自动启用本地IP池")
		shouldEnableLocalIPPool = true
	}

	if shouldEnableLocalIPPool {
		// 直接使用 localippool.NewLocalIPPool，它内置了所有自动检测和管理功能
		// 如果 ipv6SubnetCIDR 为空，会自动检测可用的IPv6子网并启用IPv6池
		localIPPool, err = localippool.NewLocalIPPool(config.LocalIPPool.StaticIPv4s, config.LocalIPPool.IPv6SubnetCIDR)
		if err != nil {
			log.Printf("错误: 创建本地 IP 池失败: %v", err)
			localIPPool = nil
		} else {
			// 如果配置了目标 IP 数量，设置它
			if config.LocalIPPool.TargetIPCount > 0 {
				log.Printf("设置本地IP池目标数量: %d", config.LocalIPPool.TargetIPCount)
				localIPPool.SetTargetIPCount(config.LocalIPPool.TargetIPCount)

				// 等待IPv6地址池准备就绪（在goroutine中异步执行，不阻塞主线程）
				// SetTargetIPCount 会立即批量创建地址，但创建需要时间，需要等待地址创建完成
				log.Printf("等待IPv6地址池准备就绪（目标数量: %d）...", config.LocalIPPool.TargetIPCount)
				go waitForIPPoolReady(localIPPool, config.LocalIPPool.TargetIPCount)
			}
			// 将 IP 池设置到 Server 中
			ipPoolWrapper := &ipPoolWrapper{pool: localIPPool}
			testIP := ipPoolWrapper.GetIP()
			if testIP != nil {
				log.Printf("已设置本地 IP 池，测试IP: %s (IPv6: %v)", testIP.String(), testIP.To4() == nil)
				srv.SetIPPool(ipPoolWrapper)
			} else {
				// 即使GetIP返回nil（隧道模式），也设置IP池
				log.Printf("已设置本地 IP 池（隧道模式，由系统自动选择路由）")
				srv.SetIPPool(ipPoolWrapper)
			}
		}
	}
	// 设置任务执行配置到服务器（直接使用 config.go 中定义的配置）
	srv.SetRockTreeDataConfig(
		config.RockTreeData.Enable,
		config.RockTreeData.HostName,
		config.RockTreeData.BulkMetadataPath,
		config.RockTreeData.NodeDataPath,
		config.RockTreeData.ImageryDataPath,
	)
	srv.SetGoogleEarthDesktopDataConfig(
		config.GoogleEarthDesktopData.Enable,
		config.GoogleEarthDesktopData.HostName,
		config.GoogleEarthDesktopData.Q2Path,
		config.GoogleEarthDesktopData.ImageryPath,
		config.GoogleEarthDesktopData.TerrainPath,
	)
	// 初始化并启动域名 IP 监控器（如果启用）
	var domainMonitor remotedomainippool.DomainMonitor
	var utlsClient *utlsclient.Client
	if config.DomainMonitor.Enable && len(config.DNSDomain.HostName) > 0 {
		// 从 JSON 文件加载 DNS 服务器列表
		dnsServers, err := LoadDNSServersFromJSON(config.DomainMonitor.DNSServersFile)
		if err != nil {
			log.Printf("警告: 加载 DNS 服务器列表失败: %v，将使用默认服务器", err)
			dnsServers = []string{"8.8.8.8", "1.1.1.1"} // 使用默认 DNS 服务器
		} else {
			log.Printf("已从 %s 加载 %d 个 DNS 服务器", config.DomainMonitor.DNSServersFile, len(dnsServers))
		}

		// 创建监控器配置
		monitorConfig := remotedomainippool.MonitorConfig{
			Domains:        config.DNSDomain.HostName,
			DNSServers:     dnsServers,
			IPInfoToken:    config.IPInfo.Token,
			UpdateInterval: time.Duration(config.DomainMonitor.UpdateIntervalMinutes) * time.Minute,
			StorageDir:     config.DomainMonitor.StorageDir,
			StorageFormat:  config.DomainMonitor.StorageFormat,
		}

		// 创建并启动监控器
		domainMonitor, err = remotedomainippool.NewRemoteIPMonitor(monitorConfig)
		if err != nil {
			log.Printf("错误: 创建域名 IP 监控器失败: %v", err)
		} else {
			domainMonitor.Start()
			log.Printf("域名 IP 监控器已启动，监控 %d 个域名，更新间隔: %d 分钟",
				len(config.DNSDomain.HostName), config.DomainMonitor.UpdateIntervalMinutes)

			// 如果启用了本地 IP 池，根据域名 IP 池的 IP 数量设置本地 IP 池的目标数量
			if localIPPool != nil {
				// 等待一小段时间让域名监控器完成首次更新
				go func() {
					time.Sleep(5 * time.Second)
					// 统计所有域名的 IP 总数
					totalIPCount := 0
					for _, domain := range config.DNSDomain.HostName {
						pool, found := domainMonitor.GetDomainPool(domain)
						if found {
							// 统计 IPv4 和 IPv6 的总数
							if ipv4Records, ok := pool["ipv4"]; ok {
								totalIPCount += len(ipv4Records)
							}
							if ipv6Records, ok := pool["ipv6"]; ok {
								totalIPCount += len(ipv6Records)
							}
						}
					}
					// 如果配置的目标 IP 数量为 0，且域名监控器已获取到 IP 数据，则设置为域名 IP 池的总数
					if config.LocalIPPool.TargetIPCount == 0 && totalIPCount > 0 {
						localIPPool.SetTargetIPCount(totalIPCount)
						log.Printf("已根据域名 IP 池自动设置本地 IP 池目标数量: %d", totalIPCount)
					}
				}()
			}

			// 基于 DomainMonitor 创建 UTLS 客户端的 RemoteIPPool。
			// 预热只针对启用的数据类型对应的 HostName，而不是 DNSDomain 中的所有域名。
			// 为了避免 "远程IP池为空" 的情况，这里启动一个后台协程，等待这些域名首次加载到 IP 后再创建 UTLS 客户端。
			var prewarmDomains []string
			if config.RockTreeData.Enable && config.RockTreeData.HostName != "" {
				prewarmDomains = append(prewarmDomains, config.RockTreeData.HostName)
			}
			if config.GoogleEarthDesktopData.Enable && config.GoogleEarthDesktopData.HostName != "" {
				prewarmDomains = append(prewarmDomains, config.GoogleEarthDesktopData.HostName)
			}

			if len(prewarmDomains) > 0 {
				go func() {
					const maxWait = 60 * time.Second
					const interval = 2 * time.Second
					start := time.Now()

					for {
						ready := false
						for _, domain := range prewarmDomains {
							pool, found := domainMonitor.GetDomainPool(domain)
							if !found || pool == nil {
								continue
							}
							ipCount := 0
							if ipv4Records, ok := pool["ipv4"]; ok {
								ipCount += len(ipv4Records)
							}
							if ipv6Records, ok := pool["ipv6"]; ok {
								ipCount += len(ipv6Records)
							}
							if ipCount > 0 {
								ready = true
								break
							}
						}

						if ready {
							log.Printf("域名 IP 池已准备就绪，将启动 UTLS 客户端进行预热")
							break
						}
						if time.Since(start) > maxWait {
							log.Printf("警告: 等待域名 IP 池超时（%v），UTLS 客户端将使用当前空 IP 池启动", maxWait)
							break
						}
						time.Sleep(interval)
					}

					// RemoteIPPool 只暴露 prewarmDomains 的 IP，用于 UTLS 预热
					remotePool := server.NewDomainMonitorRemotePool(domainMonitor, prewarmDomains)
					// 将 UtlsClientConfig 转换为 utlsclient.PoolConfig
					poolConfig := config.UtlsClient.ToPoolConfig()
					// 设置本地 IP 池（如果已启用）
					if localIPPool != nil {
						poolConfig.LocalIPPool = localIPPool
						log.Printf("已设置本地 IP 池到 UTLS 客户端，将使用本地地址池作为源 IP")
					}
					if config.GoogleEarthDesktopData.Enable {
						log.Println("已启用 GoogleEarthDesktopData，将使用 UTLS 池")
						poolConfig.HealthCheckPath = config.GoogleEarthDesktopData.HealthCheckPath
						poolConfig.SessionIdPath = config.GoogleEarthDesktopData.SessionIdPath
						log.Printf("设置配置: HealthCheckPath=%s, SessionIdPath=%s", poolConfig.HealthCheckPath, poolConfig.SessionIdPath)
					}
					if config.RockTreeData.Enable {
						log.Println("已启用 RockTreeData，将使用 UTLS 池")
						poolConfig.HealthCheckPath = config.RockTreeData.HealthCheckPath
						poolConfig.SessionIdPath = config.RockTreeData.SessionIdPath
					}

					client, cerr := utlsclient.NewClient(poolConfig, remotePool)
					if cerr != nil {
						log.Printf("错误: 创建 UTLS 客户端失败: %v", cerr)
						return
					}
					client.Start()
					log.Printf("UTLS 客户端已启动，使用域名 IP 池进行连接预热")

					// 赋值给外层变量并注入到服务器
					utlsClient = client
					srv.SetUTLSClient(client)
				}()
			}
		}
	}

	log.Println("已设置任务执行配置到服务器")

	// 启动服务器（在 goroutine 中）
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Println("服务器已启动，按 Ctrl+C 退出...")
	<-quit

	log.Println("\n收到退出信号，正在关闭服务器...")

	// 创建关闭超时上下文（最多等待 30 秒）
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 在 goroutine 中执行关闭操作
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		// 停止域名 IP 监控器
		if domainMonitor != nil {
			log.Println("正在停止域名 IP 监控器...")
			domainMonitor.Stop()
			log.Println("域名 IP 监控器已停止")
		}

		// 停止 UTLS 客户端
		if utlsClient != nil {
			log.Println("正在停止 UTLS 客户端...")
			utlsClient.Stop()
			log.Println("UTLS 客户端已停止")
		}

		// 关闭本地 IP 池
		if localIPPool != nil {
			log.Println("正在关闭本地 IP 池...")
			if err := localIPPool.Close(); err != nil {
				log.Printf("关闭本地 IP 池时出错: %v", err)
			} else {
				log.Println("本地 IP 池已关闭")
			}
		}

		// 关闭服务器（会自动关闭 IP 池）
		srv.Stop()
	}()

	// 等待关闭完成或超时
	select {
	case <-shutdownDone:
		log.Println("服务器已成功关闭")
	case <-shutdownCtx.Done():
		log.Println("警告: 关闭超时，强制退出")
		os.Exit(1)
	}
}

// waitForIPPoolReady 等待IPv6地址池准备就绪
// 通过尝试获取IP地址来验证地址池是否已准备好
func waitForIPPoolReady(pool localippool.IPPool, targetCount int) {
	const maxWait = 60 * time.Second
	const initialWait = 2 * time.Second // 初始等待时间，让批量创建完成
	const interval = 500 * time.Millisecond
	start := time.Now()

	// 如果不支持动态池，不需要等待
	if !pool.SupportsDynamicPool() {
		return
	}

	// 先等待一段时间，让SetTargetIPCount中的批量创建完成
	// 使用短时间的多次sleep，避免长时间阻塞（虽然现在在goroutine中）
	log.Printf("等待IPv6地址创建完成...")
	for i := 0; i < 4; i++ {
		time.Sleep(500 * time.Millisecond)
	}

	// 需要同时满足以下条件才算就绪：
	// 1. 能连续多次成功获取到IP（至少连续5次）
	// 2. 获取到的IP都是IPv6地址
	// 3. 每次获取后立即释放，确保有足够的地址可用
	successCount := 0
	requiredSuccess := 5   // 连续5次成功获取IP才算就绪
	requiredIPv6Count := 3 // 至少要有3个不同的IPv6地址可用
	collectedIPs := make(map[string]bool)

	for {
		// 尝试获取IP
		ip := pool.GetIP()
		if ip != nil {
			// 检查是否是IPv6地址
			if ip.To4() == nil {
				// 是IPv6地址
				successCount++
				ipStr := ip.String()
				collectedIPs[ipStr] = true

				// 立即释放IP，避免占用
				pool.MarkIPUnused(ip)

				// 检查是否满足就绪条件
				if successCount >= requiredSuccess && len(collectedIPs) >= requiredIPv6Count {
					// 连续多次成功，且有足够的IPv6地址可用，说明地址池已就绪
					elapsed := time.Since(start)
					log.Printf("IPv6地址池已准备就绪（耗时: %v，目标数量: %d，已验证可用IPv6地址: %d）",
						elapsed, targetCount, len(collectedIPs))
					return
				}
			} else {
				// 获取到的是IPv4地址，但不是我们需要的，释放它
				pool.MarkIPUnused(ip)
				// IPv4地址不计入成功计数
			}
		} else {
			// 获取失败，重置计数
			if successCount > 0 {
				log.Printf("IPv6地址池准备中: 获取IP失败，重置计数（之前成功: %d次，已验证地址: %d个）",
					successCount, len(collectedIPs))
			}
			successCount = 0
			collectedIPs = make(map[string]bool) // 重置收集的IP
		}

		// 定期输出进度信息
		elapsed := time.Since(start)
		if elapsed > 10*time.Second && elapsed%10*time.Second < interval {
			log.Printf("IPv6地址池准备中: 已等待 %v，成功获取: %d次，已验证地址: %d个",
				elapsed.Round(time.Second), successCount, len(collectedIPs))
		}

		// 检查超时
		if elapsed > maxWait {
			log.Printf("警告: 等待IPv6地址池准备就绪超时（%v），将继续启动但地址池可能未完全就绪（已验证: %d个IPv6地址）",
				maxWait, len(collectedIPs))
			log.Printf("提示: 如果地址池未就绪，可能是IPv6接口配置问题或权限不足，将使用系统默认地址")
			return
		}

		// 使用可中断的 sleep，但这里使用简单的方式
		// 由于函数现在在goroutine中运行，不会阻塞主线程
		time.Sleep(interval)
	}
}

// ipPoolWrapper 包装 localippool.IPPool 以实现 Server 的 IPPoolInterface 接口
type ipPoolWrapper struct {
	pool localippool.IPPool
}

func (w *ipPoolWrapper) GetIP() net.IP {
	return w.pool.GetIP()
}

func (w *ipPoolWrapper) ReleaseIP(ip net.IP) {
	w.pool.ReleaseIP(ip)
}

func (w *ipPoolWrapper) MarkIPUnused(ip net.IP) {
	w.pool.MarkIPUnused(ip)
}

func (w *ipPoolWrapper) SetTargetIPCount(count int) {
	w.pool.SetTargetIPCount(count)
}

func (w *ipPoolWrapper) Close() error {
	return w.pool.Close()
}
