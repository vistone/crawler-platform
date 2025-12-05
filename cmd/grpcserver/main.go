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

	// 初始化并启动本地 IP 池（如果启用）
	var localIPPool localippool.IPPool
	if config.LocalIPPool.Enable {
		localIPPool, err = localippool.NewLocalIPPool(config.LocalIPPool.StaticIPv4s, config.LocalIPPool.IPv6SubnetCIDR)
		if err != nil {
			log.Printf("错误: 创建本地 IP 池失败: %v", err)
			localIPPool = nil
		} else {
			// 如果配置了目标 IP 数量，设置它
			if config.LocalIPPool.TargetIPCount > 0 {
				localIPPool.SetTargetIPCount(config.LocalIPPool.TargetIPCount)
			}
			// 将 IP 池设置到 Server 中
			if localIPPool.GetIP() != nil {
				// 创建一个包装器以匹配 Server 的 IPPoolInterface
				ipPoolWrapper := &ipPoolWrapper{pool: localIPPool}
				logger.Info("已设置 IP 池", "ip_pool", ipPoolWrapper.GetIP())
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
