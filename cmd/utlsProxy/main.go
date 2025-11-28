package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	projlogger "crawler-platform/logger"
	"crawler-platform/utlsclient"

	"github.com/quic-go/quic-go"
)

// ProxyConfig 代理服务器配置
type ProxyConfig struct {
	// TUIC服务器配置
	ListenAddr     string        // 监听地址，格式: host:port
	Token          string        // TUIC认证令牌
	Certificate    string        // TLS证书文件路径
	PrivateKey     string        // TLS私钥文件路径
	
	// 连接池配置
	PoolConfig     *utlsclient.PoolConfig
	
	// 日志配置
	LogLevel       string        // 日志级别: debug, info, warn, error
}

// UTLSProxy TUIC代理服务器
type UTLSProxy struct {
	config      *ProxyConfig
	connPool    *utlsclient.UTLSHotConnPool
	quicListener *quic.Listener
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

	quicConfig := &quic.Config{
		KeepAlivePeriod: 10 * time.Second,
		MaxIdleTimeout:  30 * time.Second,
	}

	quicListener, err := quic.Listen(udpConn, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("创建QUIC监听器失败: %w", err)
	}

	p.quicListener = quicListener
	projlogger.Info("TUIC代理服务器启动，监听地址: %s", p.config.ListenAddr)

	// 启动接受连接的goroutine
	go p.acceptConnections()

	return nil
}

// acceptConnections 接受客户端连接
func (p *UTLSProxy) acceptConnections() {
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			// 接受QUIC连接
			conn, err := p.quicListener.Accept(p.ctx)
			if err != nil {
				if p.ctx.Err() != nil {
					// 上下文已取消，正常退出
					return
				}
				projlogger.Error("接受QUIC连接失败: %v", err)
				continue
			}

			// 为每个连接启动处理goroutine
			go p.handleConnection(conn)
		}
	}
}

// handleConnection 处理单个客户端连接
func (p *UTLSProxy) handleConnection(conn *quic.Conn) {
	defer conn.CloseWithError(0, "连接关闭")

	projlogger.Debug("新客户端连接: %s", conn.RemoteAddr())

	// 处理TUIC协议握手和请求
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
			// 接收数据流
			stream, err := conn.AcceptStream(p.ctx)
			if err != nil {
				if p.ctx.Err() != nil {
					return
				}
				projlogger.Debug("接受流失败: %v", err)
				continue
			}

			// 处理每个流
			go p.handleStream(stream)
		}
	}
}

// handleStream 处理单个数据流
func (p *UTLSProxy) handleStream(stream *quic.Stream) {
	defer stream.Close()

	// 读取完整的请求数据
	// TUIC协议可能需要多次读取，这里简化处理
	buffer := make([]byte, 64*1024) // 64KB缓冲区
	var requestData []byte
	
	for {
		n, err := stream.Read(buffer)
		if n > 0 {
			requestData = append(requestData, buffer[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			projlogger.Debug("读取流数据失败: %v", err)
			return
		}
		// 如果读取的数据少于缓冲区大小，可能已经读取完毕
		if n < len(buffer) {
			break
		}
	}

	if len(requestData) == 0 {
		projlogger.Debug("收到空请求")
		return
	}

	// 验证TUIC token（简化实现，实际应该从请求中提取）
	// TODO: 实现完整的TUIC token验证

	// 解析TUIC请求
	request, err := p.parseTUICRequest(requestData)
	if err != nil {
		projlogger.Error("解析TUIC请求失败: %v", err)
		return
	}

	// 使用utlsclient处理HTTP请求
	response, err := p.handleHTTPRequest(request)
	if err != nil {
		projlogger.Error("处理HTTP请求失败: %v", err)
		// 发送错误响应
		errorResp := fmt.Sprintf("HTTP/1.1 502 Bad Gateway\r\nContent-Length: %d\r\n\r\n%s", 
			len(err.Error()), err.Error())
		stream.Write([]byte(errorResp))
		return
	}

	// 发送响应
	if _, err := stream.Write(response); err != nil {
		projlogger.Error("发送响应失败: %v", err)
		return
	}
}

// TUICRequest TUIC协议请求结构
type TUICRequest struct {
	Command   byte   // 命令类型
	Target    string // 目标地址
	Data      []byte // 请求数据
}

// parseTUICRequest 解析TUIC协议请求
// TUIC协议格式参考: https://github.com/EAimTY/tuic
// 简化实现：支持基本的TUIC v5协议格式
// 输入: data - 原始数据
// 输出: *TUICRequest - 解析后的请求, error - 错误信息
func (p *UTLSProxy) parseTUICRequest(data []byte) (*TUICRequest, error) {
	if len(data) < 3 {
		return nil, fmt.Errorf("请求数据太短")
	}

	// TUIC v5 基本格式: [版本(1字节)][命令(1字节)][长度(2字节)][数据]
	version := data[0]
	if version != 5 {
		return nil, fmt.Errorf("不支持的TUIC版本: %d", version)
	}

	command := data[1]
	
	// 读取长度（大端序）
	if len(data) < 5 {
		return nil, fmt.Errorf("数据长度不足")
	}
	dataLen := binary.BigEndian.Uint16(data[2:4])
	
	if len(data) < 4+int(dataLen) {
		return nil, fmt.Errorf("数据不完整，期望长度: %d, 实际: %d", 4+int(dataLen), len(data))
	}

	// 解析目标地址和请求数据
	payload := data[4 : 4+dataLen]
	
	// 简化实现：假设payload格式为 [地址类型(1字节)][地址长度(1字节)][地址][端口(2字节)][HTTP请求数据]
	if len(payload) < 4 {
		return nil, fmt.Errorf("payload数据太短")
	}

	_ = payload[0] // addrType 暂时未使用，保留用于未来扩展
	addrLen := int(payload[1])
	
	if len(payload) < 2+addrLen+2 {
		return nil, fmt.Errorf("地址数据不完整")
	}

	targetAddr := string(payload[2 : 2+addrLen])
	port := binary.BigEndian.Uint16(payload[2+addrLen : 2+addrLen+2])
	target := fmt.Sprintf("%s:%d", targetAddr, port)
	
	requestData := payload[2+addrLen+2:]

	return &TUICRequest{
		Command: command,
		Target:  target,
		Data:    requestData,
	}, nil
}

// handleHTTPRequest 使用utlsclient处理HTTP请求
// 输入: request - TUIC请求
// 输出: []byte - HTTP响应数据, error - 错误信息
func (p *UTLSProxy) handleHTTPRequest(request *TUICRequest) ([]byte, error) {
	// 解析目标地址（格式: host:port）
	host, port, err := net.SplitHostPort(request.Target)
	if err != nil {
		return nil, fmt.Errorf("解析目标地址失败: %w", err)
	}

	// 从连接池获取连接（使用主机名）
	conn, err := p.connPool.GetConnection(host)
	if err != nil {
		return nil, fmt.Errorf("获取连接失败: %w", err)
	}
	defer p.connPool.PutConnection(conn)

	// 创建UTLS客户端
	client := utlsclient.NewUTLSClient(conn)

	// 解析HTTP请求
	httpReq, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(request.Data)))
	if err != nil {
		return nil, fmt.Errorf("解析HTTP请求失败: %w", err)
	}

	// 设置正确的Host头
	if httpReq.Host == "" {
		httpReq.Host = host
		if port != "443" && port != "80" {
			httpReq.Host = net.JoinHostPort(host, port)
		}
	}

	// 设置URL（如果未设置或无效）
	if httpReq.URL == nil || httpReq.URL.Host == "" {
		scheme := "https"
		if port == "80" {
			scheme = "http"
		}
		// 解析URL
		parsedURL, err := url.Parse(fmt.Sprintf("%s://%s%s", scheme, httpReq.Host, httpReq.RequestURI))
		if err == nil && parsedURL != nil {
			httpReq.URL = parsedURL
		} else {
			// 如果解析失败，创建一个简单的URL
			httpReq.URL = &url.URL{
				Scheme: scheme,
				Host:   httpReq.Host,
				Path:   httpReq.RequestURI,
			}
		}
	}

	// 执行HTTP请求
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("执行HTTP请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %w", err)
	}

	// 序列化HTTP响应
	var buf bytes.Buffer
	if err := httpResp.Write(&buf); err != nil {
		return nil, fmt.Errorf("序列化HTTP响应失败: %w", err)
	}

	// 将响应体追加到缓冲区
	if _, err := buf.Write(body); err != nil {
		return nil, fmt.Errorf("写入响应体失败: %w", err)
	}

	return buf.Bytes(), nil
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

	// 使用项目级日志系统
	logger := &projlogger.DefaultLogger{}
	projlogger.SetGlobalLogger(logger)

	return nil
}

func main() {
	// 解析命令行参数
	var (
		listenAddr  = flag.String("listen", "0.0.0.0:443", "监听地址 (host:port)")
		token       = flag.String("token", "", "TUIC认证令牌")
		certFile    = flag.String("cert", "server.crt", "TLS证书文件路径")
		keyFile     = flag.String("key", "server.key", "TLS私钥文件路径")
		configFile  = flag.String("config", "", "配置文件路径 (可选)")
		logLevel    = flag.String("log", "info", "日志级别: debug, info, warn, error")
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
