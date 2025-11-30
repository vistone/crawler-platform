package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	projlogger "crawler-platform/logger"
	"crawler-platform/utlsclient"

	"github.com/quic-go/quic-go"
)

// ProxyConfig 代理服务器配置
type ProxyConfig struct {
	// TUIC服务器配置
	ListenAddr  string // 监听地址，格式: host:port
	Token       string // TUIC认证令牌
	Certificate string // TLS证书文件路径
	PrivateKey  string // TLS私钥文件路径

	// 连接池配置
	PoolConfig *utlsclient.PoolConfig

	// 日志配置
	LogLevel string // 日志级别: debug, info, warn, error
}

// UTLSProxy TUIC代理服务器
type UTLSProxy struct {
	config       *ProxyConfig
	connPool     *utlsclient.UTLSHotConnPool
	quicListener *quic.Listener
	handlers     map[*quic.Conn]interface{} // 可以是HybridHandler或IPTunnelHandler
	handlersMu   sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewUTLSProxy 创建新的TUIC代理服务器
// 输入: config - 代理服务器配置
// 输出: *UTLSProxy - 代理服务器实例, error - 错误信息
func NewUTLSProxy(config *ProxyConfig) (*UTLSProxy, error) {
	if config == nil {
		return nil, fmt.Errorf("配置不能为空")
	}

	// 初始化日志
	if err := initLogger(config.LogLevel); err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}

	// 创建连接池
	poolConfig := config.PoolConfig
	if poolConfig == nil {
		poolConfig = utlsclient.DefaultPoolConfig()
	}
	connPool := utlsclient.NewUTLSHotConnPool(poolConfig)

	ctx, cancel := context.WithCancel(context.Background())

	proxy := &UTLSProxy{
		config:   config,
		connPool: connPool,
		handlers: make(map[*quic.Conn]interface{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	return proxy, nil
}

// Start 启动代理服务器
// 输出: error - 错误信息
func (p *UTLSProxy) Start() error {
	// 加载TLS证书
	cert, err := tls.LoadX509KeyPair(p.config.Certificate, p.config.PrivateKey)
	if err != nil {
		return fmt.Errorf("加载TLS证书失败: %w", err)
	}

	// 创建QUIC配置
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"tuic"}, // TUIC协议标识
	}

	// 创建QUIC监听器
	addr, err := net.ResolveUDPAddr("udp", p.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("解析监听地址失败: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("创建UDP监听器失败: %w", err)
	}

	// 优化UDP缓冲区大小，提高传输效率
	// 设置接收缓冲区为8MB（QUIC推荐值）
	if err := udpConn.SetReadBuffer(8 * 1024 * 1024); err != nil {
		projlogger.Warn("设置UDP接收缓冲区失败，使用默认值: %v", err)
	}
	// 设置发送缓冲区为8MB
	if err := udpConn.SetWriteBuffer(8 * 1024 * 1024); err != nil {
		projlogger.Warn("设置UDP发送缓冲区失败，使用默认值: %v", err)
	}

	// 优化QUIC配置 - 高速传输和连接复用
	quicConfig := &quic.Config{
		// 保持连接活跃，支持长连接复用
		KeepAlivePeriod: 5 * time.Second,  // 更频繁的keepalive
		MaxIdleTimeout:  60 * time.Second, // 增加空闲超时，支持连接复用

		// 高并发支持
		MaxIncomingStreams:    1000, // 增加最大流数
		MaxIncomingUniStreams: 1000, // 增加单向流数

		// 接收窗口优化 - 提高吞吐量
		InitialStreamReceiveWindow:     8 * 1024 * 1024,  // 8MB 流接收窗口
		InitialConnectionReceiveWindow: 16 * 1024 * 1024, // 16MB 连接接收窗口

		// 允许0-RTT连接，减少握手延迟
		Allow0RTT: true,
	}

	quicListener, err := quic.Listen(udpConn, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("创建QUIC监听器失败: %w", err)
	}

	p.quicListener = quicListener
	projlogger.Info("TUIC代理服务器启动，监听地址: %s", p.config.ListenAddr)

	// 启动接受连接的goroutine
	projlogger.Info("UTLSProxy: 启动接受连接的goroutine")
	go p.acceptConnections()

	return nil
}

// acceptConnections 接受客户端连接
func (p *UTLSProxy) acceptConnections() {
	projlogger.Info("UTLSProxy: 开始接受客户端连接")
	for {
		select {
		case <-p.ctx.Done():
			projlogger.Info("UTLSProxy: 上下文已取消，停止接受连接")
			return
		default:
			// 接受QUIC连接
			projlogger.Info("UTLSProxy: 等待接受QUIC连接...")
			conn, err := p.quicListener.Accept(p.ctx)
			if err != nil {
				if p.ctx.Err() != nil {
					// 上下文已取消，正常退出
					projlogger.Info("UTLSProxy: 上下文已取消，停止接受连接")
					return
				}
				projlogger.Error("UTLSProxy: 接受QUIC连接失败: %v", err)
				continue
			}

			projlogger.Info("UTLSProxy: 接受新QUIC连接: %s", conn.RemoteAddr())
			// 为每个连接启动处理goroutine
			go p.handleConnection(conn)
		}
	}
}

// handleConnection 处理单个客户端连接
func (p *UTLSProxy) handleConnection(conn *quic.Conn) {
	defer func() {
		// 清理handler
		p.handlersMu.Lock()
		if handler, exists := p.handlers[conn]; exists {
			// 类型断言并调用Stop方法
			switch h := handler.(type) {
			case *IPTunnelHandler:
				h.Stop()
			}
			delete(p.handlers, conn)
		}
		p.handlersMu.Unlock()

		conn.CloseWithError(0, "连接关闭")
	}()

	projlogger.Info("UTLSProxy: 新客户端连接: %s", conn.RemoteAddr())

	// 创建IPTunnelHandler（已支持CONNECT和PACKET两种模式）
	handler := NewIPTunnelHandler(p, conn)

	// 注册handler
	p.handlersMu.Lock()
	p.handlers[conn] = handler
	p.handlersMu.Unlock()

	// 启动处理器
	handler.Start()

	// 等待连接关闭
	<-handler.ctx.Done()
}

// Stop 停止代理服务器
func (p *UTLSProxy) Stop() error {
	p.cancel()

	// 关闭QUIC监听器
	if p.quicListener != nil {
		if err := p.quicListener.Close(); err != nil {
			projlogger.Error("关闭QUIC监听器失败: %v", err)
		}
	}

	// 关闭连接池
	if p.connPool != nil {
		if err := p.connPool.Close(); err != nil {
			projlogger.Error("关闭连接池失败: %v", err)
		}
	}

	projlogger.Info("TUIC代理服务器已停止")
	return nil
}

// initLogger 初始化日志系统
func initLogger(level string) error {
	if level == "" {
		level = "info"
	}

	// 根据日志级别配置日志记录器
	// debug: 输出所有级别
	// info: 输出 info, warn, error
	// warn: 输出 warn, error
	// error: 只输出 error
	var enableDebug, enableInfo, enableWarn, enableError bool
	switch level {
	case "debug":
		enableDebug = true
		enableInfo = true
		enableWarn = true
		enableError = true
	case "info":
		enableDebug = false
		enableInfo = true
		enableWarn = true
		enableError = true
	case "warn":
		enableDebug = false
		enableInfo = false
		enableWarn = true
		enableError = true
	case "error":
		enableDebug = false
		enableInfo = false
		enableWarn = false
		enableError = true
	default:
		// 默认使用 info 级别
		enableDebug = false
		enableInfo = true
		enableWarn = true
		enableError = true
	}

	// 使用控制台日志记录器，输出到标准输出（stdout/stderr）
	logger := projlogger.NewConsoleLogger(enableDebug, enableInfo, enableWarn, enableError)
	projlogger.SetGlobalLogger(logger)

	return nil
}

func main() {
	// 解析命令行参数
	var (
		listenAddr = flag.String("listen", "0.0.0.0:443", "监听地址 (host:port)")
		token      = flag.String("token", "", "TUIC认证令牌")
		certFile   = flag.String("cert", "server.crt", "TLS证书文件路径")
		keyFile    = flag.String("key", "server.key", "TLS私钥文件路径")
		configFile = flag.String("config", "", "配置文件路径 (可选)")
		logLevel   = flag.String("log", "info", "日志级别: debug, info, warn, error")
	)
	flag.Parse()

	// 验证必需参数
	if *token == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须提供TUIC认证令牌 (-token)\n")
		os.Exit(1)
	}

	// 加载连接池配置
	var poolConfig *utlsclient.PoolConfig
	if *configFile != "" {
		config, err := utlsclient.LoadPoolConfigFromFile(*configFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
		poolConfig = config
	} else {
		// 使用合并配置
		config, _, _, err := utlsclient.LoadMergedPoolConfig()
		if err != nil {
			// 如果加载失败，使用默认配置
			projlogger.Warn("加载配置失败，使用默认配置: %v", err)
			poolConfig = utlsclient.DefaultPoolConfig()
		} else {
			poolConfig = config
		}
	}

	// 创建代理服务器配置
	proxyConfig := &ProxyConfig{
		ListenAddr:  *listenAddr,
		Token:       *token,
		Certificate: *certFile,
		PrivateKey:  *keyFile,
		PoolConfig:  poolConfig,
		LogLevel:    *logLevel,
	}

	// 创建代理服务器
	proxy, err := NewUTLSProxy(proxyConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建代理服务器失败: %v\n", err)
		os.Exit(1)
	}

	// 启动服务器
	if err := proxy.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "启动代理服务器失败: %v\n", err)
		os.Exit(1)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	// 优雅关闭
	projlogger.Info("收到停止信号，正在关闭服务器...")
	if err := proxy.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "停止代理服务器失败: %v\n", err)
		os.Exit(1)
	}
}
