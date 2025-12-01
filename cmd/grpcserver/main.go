package main

import (
	"crypto/tls"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	server "crawler-platform/cmd/grpcserver/internal"
	"crawler-platform/localippool"
	"crawler-platform/logger"
	"crawler-platform/remotedomainippool"
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
			log.Printf("本地 IP 池已初始化")
			// 如果配置了目标 IP 数量，设置它
			if config.LocalIPPool.TargetIPCount > 0 {
				localIPPool.SetTargetIPCount(config.LocalIPPool.TargetIPCount)
				log.Printf("已设置本地 IP 池目标 IP 数量: %d", config.LocalIPPool.TargetIPCount)
			}
			// 将 IP 池设置到 Server 中
			if localIPPool != nil {
				// 创建一个包装器以匹配 Server 的 IPPoolInterface
				ipPoolWrapper := &ipPoolWrapper{pool: localIPPool}
				srv.SetIPPool(ipPoolWrapper)
			}
		}
	}

	// 初始化并启动域名 IP 监控器（如果启用）
	var domainMonitor remotedomainippool.DomainMonitor
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
		}
	}

	// 初始化热连接池管理器并创建对应的热连接池（如果启用）
	var hotConnPoolManager *server.HotConnPoolManager
	if domainMonitor != nil {
		hotConnPoolManager = server.NewHotConnPoolManager(domainMonitor)

		// 等待域名 IP 池数据加载完成
		go func() {
			// 等待域名监控器完成首次更新
			time.Sleep(10 * time.Second)

			// 检查 RockTreeData 配置，如果启用则创建热连接池
			if config.RockTreeData.Enable && config.RockTreeData.HostName != "" && config.RockTreeData.HotConnectPath != "" {
				err := hotConnPoolManager.CreatePoolForDataType("RockTreeData", config.RockTreeData.HostName, config.RockTreeData.HotConnectPath, "GET")
				if err != nil {
					log.Printf("创建 RockTreeData 热连接池失败: %v", err)
				} else {
					// 预热连接（预热 10 个连接）
					if err := hotConnPoolManager.PreloadConnections("RockTreeData", 10); err != nil {
						log.Printf("预热 RockTreeData 连接失败: %v", err)
					}
				}
			}

			// 检查 GoogleEarthDesktopData 配置，如果启用则创建热连接池
			if config.GoogleEarthDesktopData.Enable && config.GoogleEarthDesktopData.HostName != "" && config.GoogleEarthDesktopData.HotConnectPath != "" {
				err := hotConnPoolManager.CreatePoolForDataType("GoogleEarthDesktopData", config.GoogleEarthDesktopData.HostName, config.GoogleEarthDesktopData.HotConnectPath, "POST")
				if err != nil {
					log.Printf("创建 GoogleEarthDesktopData 热连接池失败: %v", err)
				} else {
					// 预热连接（预热 10 个连接）
					if err := hotConnPoolManager.PreloadConnections("GoogleEarthDesktopData", 10); err != nil {
						log.Printf("预热 GoogleEarthDesktopData 连接失败: %v", err)
					}
				}
			}
		}()

		// 设置热连接池管理器到服务器
		srv.SetHotConnPoolManager(hotConnPoolManager)

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
		log.Println("已设置热连接池管理器和任务执行配置到服务器")
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

	// 停止域名 IP 监控器
	if domainMonitor != nil {
		log.Println("正在停止域名 IP 监控器...")
		domainMonitor.Stop()
	}

	// 关闭所有热连接池
	if hotConnPoolManager != nil {
		log.Println("正在关闭所有热连接池...")
		hotConnPoolManager.CloseAll()
	}

	// 关闭服务器（会自动关闭 IP 池）
	srv.Stop()
	log.Println("服务器已关闭")
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
