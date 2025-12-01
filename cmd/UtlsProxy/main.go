package main

import (
	"context"
	proxylog "crawler-platform/logger"
	"crawler-platform/utlsclient"
	"crypto/rand"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	singbox "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/adapter/outbound"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

var ProxyConfig Config

// generateTUICUUID generates a new UUID for TUIC connections
func generateTUICUUID() string {
	return uuid.New().String()
}

// generatePassword generates a random password for TUIC connections
func generatePassword() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return fmt.Sprintf("%x", bytes)
}

// parseListenAddress 从 listen 字符串中解析地址和端口
// 输入: listen - 监听地址字符串，格式如 "0.0.0.0:7457" 或 ":7457"
// 输出: host - 主机地址, port - 端口号, error - 错误信息
func parseListenAddress(listen string) (string, uint16, error) {
	host, portStr, err := net.SplitHostPort(listen)
	if err != nil {
		return "", 0, fmt.Errorf("解析监听地址失败: %v", err)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return "", 0, fmt.Errorf("解析端口失败: %v", err)
	}

	// 如果 host 为空，使用 "0.0.0.0"
	if host == "" {
		host = "0.0.0.0"
	}

	return host, uint16(port), nil
}

// startTUICServer starts a TUIC server with the given configuration
// 输入: node - TUIC 节点配置, pool - utlsclient 连接池（用于转发请求）, ipPool - IP 池
func startTUICServer(node TUICNodeConfig, pool *utlsclient.UTLSHotConnPool, ipPool utlsclient.IPPoolProvider) {
	// Use configured UUID if provided, otherwise generate a new one
	if node.UUID == "" {
		node.UUID = generateTUICUUID()
	}

	// Use configured password if provided, otherwise generate a new one
	// For sing-box, if password is not set, use UUID as password
	if node.Password == "" {
		node.Password = node.UUID
	}

	// 解析监听地址和端口
	var listenHost string
	var listenPort uint16 = 7457

	if node.Listen != "" {
		host, port, err := parseListenAddress(node.Listen)
		if err != nil {
			proxylog.Error("解析 TUIC 服务器 %s 的监听地址失败: %v", node.Name, err)
			return
		}
		listenHost = host
		listenPort = port
	} else if node.Port > 0 {
		listenPort = uint16(node.Port)
		listenHost = "0.0.0.0"
	} else {
		proxylog.Error("TUIC 服务器 %s 未配置监听地址或端口", node.Name)
		return
	}

	// Log TUIC server info
	proxylog.Info("Starting TUIC server: %s on %s:%d with UUID: %s",
		node.Name, listenHost, listenPort, node.UUID)

	inboundOptions := &option.TUICInboundOptions{
		Users: []option.TUICUser{{
			UUID:     node.UUID,
			Password: node.Password,
		}},
		CongestionControl: node.CongestionControl,
		ZeroRTTHandshake:  node.ZeroRTTHandshake,
	}

	// Set listen address and port
	addr, err := netip.ParseAddr(listenHost)
	if err != nil {
		proxylog.Error("解析 TUIC 服务器 %s 的监听地址失败: %v", node.Name, err)
		return
	}
	listenAddr := badoption.Addr(addr)
	inboundOptions.ListenOptions.Listen = &listenAddr
	inboundOptions.ListenOptions.ListenPort = listenPort

	// Set TLS options if certificate and private key are provided
	if node.Certificate != "" && node.PrivateKey != "" {
		inboundOptions.InboundTLSOptionsContainer = option.InboundTLSOptionsContainer{
			TLS: &option.InboundTLSOptions{
				Enabled:         true,
				CertificatePath: node.Certificate,
				KeyPath:         node.PrivateKey,
			},
		}
	} else {
		proxylog.Error("TLS certificate or key path is missing for TUIC server %s", node.Name)
		return
	}

	// 创建自定义 outbound registry，注册我们的 UTLS outbound
	outboundRegistry := include.OutboundRegistry()

	// 注册自定义 UTLS outbound
	// 使用 outbound.Register 函数注册我们的自定义 outbound
	// 定义一个空的选项类型
	type UTLSOutboundOptions struct{}
	// 通过闭包捕获 pool 和 ipPool
	ipPoolForOutbound := ipPool
	outbound.Register[UTLSOutboundOptions](outboundRegistry, "utls", func(ctx context.Context, router adapter.Router, logger log.ContextLogger, tag string, options UTLSOutboundOptions) (adapter.Outbound, error) {
		// 创建 UTLS outbound 实例
		// 注意：options 参数在这里被忽略，因为我们不需要任何配置
		// pool 和 ipPool 通过闭包捕获
		_ = options // 避免未使用变量警告
		return NewUTLSOutbound(tag, pool, ipPoolForOutbound), nil
	})

	// 创建 context，使用我们的自定义 registry
	ctx := singbox.Context(context.Background(), include.InboundRegistry(), outboundRegistry, include.EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry())

	// 配置 TUIC inbound，使用我们的自定义 UTLS outbound
	options := singbox.Options{
		Context: ctx,
		Options: option.Options{
			Inbounds: []option.Inbound{
				{
					Type:    "tuic",
					Tag:     node.Name,
					Options: inboundOptions,
				},
			},
			Outbounds: []option.Outbound{
				{
					Type:    "utls",
					Tag:     "utls-out",
					Options: &UTLSOutboundOptions{}, // 使用指针类型
				},
			},
			Route: &option.RouteOptions{
				Final: "utls-out", // 使用我们的自定义 UTLS outbound
			},
		},
	}

	// 创建 sing-box 实例
	box, err := singbox.New(options)
	if err != nil {
		proxylog.Error("Failed to create TUIC server %s: %v", node.Name, err)
		return
	}

	if err := box.Start(); err != nil {
		proxylog.Error("Failed to start TUIC server %s: %v", node.Name, err)
		return
	}

	proxylog.Info("TUIC server %s started successfully on %s:%d", node.Name, listenHost, listenPort)

	// Keep the server running
	<-ctx.Done()

	// Clean up when shutting down
	box.Close()
}

func main() {
	// 初始化日志（可按需定制）
	proxylog.SetGlobalLogger(&proxylog.DefaultLogger{})
	proxylog.Info("系统初始化中")

	// 加载配置文件
	_, err := toml.DecodeFile("config.toml", &ProxyConfig)
	if err != nil {
		proxylog.Error("配置文件加载失败: %v", err)
		return
	}

	proxylog.Info("代理服务器端口：%d", ProxyConfig.UTLSProxyServer.Port)

	// 创建热连接池（使用默认配置）
	pool := utlsclient.NewUTLSHotConnPool(&utlsclient.PoolConfig{
		MaxConnections:         ProxyConfig.HotConnPool.MaxConnections,
		MaxIdleConns:           ProxyConfig.HotConnPool.MaxIdleConns,
		MaxConnsPerHost:        ProxyConfig.HotConnPool.MaxConnsPerHost,
		ConnTimeout:            time.Duration(ProxyConfig.HotConnPool.ConnTimeout) * time.Second,
		IdleTimeout:            time.Duration(ProxyConfig.HotConnPool.IdleTimeout) * time.Second,
		CleanupInterval:        time.Duration(ProxyConfig.HotConnPool.CleanupInterval) * time.Second,
		MaxLifetime:            time.Duration(ProxyConfig.HotConnPool.MaxLifetime) * time.Second,
		TestTimeout:            time.Duration(ProxyConfig.HotConnPool.TestTimeout) * time.Second,
		HealthCheckInterval:    time.Duration(ProxyConfig.HotConnPool.HealthCheckInterval) * time.Second,
		BlacklistCheckInterval: time.Duration(ProxyConfig.HotConnPool.BlacklistCheckInterval) * time.Second,
		DNSUpdateInterval:      time.Duration(ProxyConfig.HotConnPool.DNSUpdateInterval) * time.Second,
		MaxRetries:             ProxyConfig.HotConnPool.MaxRetries,
	})
	defer pool.Close()

	// 从 JSON 文件加载 IP 池并设置到连接池
	jsonIPPool, err := NewJSONIPPool("kh_google_com.json")
	if err != nil {
		proxylog.Error("加载 IP 池失败: %v", err)
	} else {
		// 使用 SetDependencies 设置 IP 池
		pool.SetDependencies(nil, jsonIPPool, nil, nil)
		proxylog.Info("已设置 JSON IP 池")
	}

	// 从配置文件加载黑白名单并设置到连接池
	_, whitelist, blacklist, err := utlsclient.LoadMergedPoolConfig()
	if err == nil {
		// 设置白名单和黑名单
		// 注意：由于不能修改 utlshotconnpool，我们直接通过 SetDependencies 传递访问控制器
		// 或者使用反射访问 ipAccessCtrl 字段
		// 暂时跳过，因为连接池会在连接建立时自动添加到白名单
		proxylog.Info("已加载白名单 %d 个IP，黑名单 %d 个IP（连接池会在连接建立时自动管理）", len(whitelist), len(blacklist))
	} else {
		proxylog.Debug("加载黑白名单配置失败（使用默认配置）: %v", err)
	}

	proxylog.Info("系统初始化成功")

	// 启动 TUIC 服务
	if ProxyConfig.TUIC.Enable {
		for _, node := range ProxyConfig.TUIC.Nodes {
			if !node.Disabled {
				go startTUICServer(node, pool, jsonIPPool)
			}
		}
	}

	// 保持程序运行
	select {}
}
