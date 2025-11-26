package utlsclient

import (
	"bufio"
	"context"
	projconfig "crawler-platform/config"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	projlogger "crawler-platform/logger"

	"github.com/BurntSushi/toml"
	utls "github.com/refraction-networking/utls"
)

// HotConnPool 热连接池接口，提供简单明确的调用方式
type HotConnPool interface {
	// GetConnection 获取到目标主机的热连接
	// 输入: targetHost - 目标主机域名
	// 输出: *UTLSConnection - 热连接对象, error - 错误信息
	GetConnection(targetHost string) (*UTLSConnection, error)

	// GetConnectionWithValidation 获取热连接并验证指定路径的可用性
	// 输入: fullURL - 完整的URL (https://domain/path)
	// 输出: *UTLSConnection - 热连接对象, error - 错误信息
	GetConnectionWithValidation(fullURL string) (*UTLSConnection, error)

	// PutConnection 归还连接到池中
	// 输入: conn - 要归还的连接
	PutConnection(conn *UTLSConnection)

	// GetStats 获取连接池统计信息
	// 输出: PoolStats - 统计信息
	GetStats() PoolStats

	// IsHealthy 检查连接池是否健康
	// 输出: bool - 是否健康
	IsHealthy() bool

	// Close 关闭连接池并清理所有连接
	// 输出: error - 错误信息
	Close() error
}

// ConfigFile 完整配置文件结构
type ConfigFile struct {
	Pool      PoolConfigSection `toml:"pool"`
	Whitelist WhitelistSection  `toml:"whitelist"`
	Blacklist BlacklistSection  `toml:"blacklist"`
}

// PoolConfigSection 连接池配置段
type PoolConfigSection struct {
	MaxConnections         int `toml:"max_connections"`
	MaxConnsPerHost        int `toml:"max_conns_per_host"`
	MaxIdleConns           int `toml:"max_idle_conns"`
	ConnTimeout            int `toml:"conn_timeout"`
	IdleTimeout            int `toml:"idle_timeout"`
	MaxLifetime            int `toml:"max_lifetime"`
	TestTimeout            int `toml:"test_timeout"`
	HealthCheckInterval    int `toml:"health_check_interval"`
	CleanupInterval        int `toml:"cleanup_interval"`
	BlacklistCheckInterval int `toml:"blacklist_check_interval"`
	DNSUpdateInterval      int `toml:"dns_update_interval"`
	MaxRetries             int `toml:"max_retries"`
}

// WhitelistSection 白名单配置段
type WhitelistSection struct {
	IPs []string `toml:"ips"`
}

// BlacklistSection 黑名单配置段
type BlacklistSection struct {
	IPs []string `toml:"ips"`
}

// LoadConfigFromTOML 从TOML文件加载配置
func LoadConfigFromTOML(configPath string) (*PoolConfig, []string, []string, error) {
	var config ConfigFile

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析TOML
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, nil, nil, fmt.Errorf("解析TOML配置失败: %w", err)
	}

	// 转换为PoolConfig
	poolConfig := &PoolConfig{
		MaxConnections:         config.Pool.MaxConnections,
		MaxConnsPerHost:        config.Pool.MaxConnsPerHost,
		MaxIdleConns:           config.Pool.MaxIdleConns,
		ConnTimeout:            time.Duration(config.Pool.ConnTimeout) * time.Second,
		IdleTimeout:            time.Duration(config.Pool.IdleTimeout) * time.Second,
		MaxLifetime:            time.Duration(config.Pool.MaxLifetime) * time.Second,
		TestTimeout:            time.Duration(config.Pool.TestTimeout) * time.Second,
		HealthCheckInterval:    time.Duration(config.Pool.HealthCheckInterval) * time.Second,
		CleanupInterval:        time.Duration(config.Pool.CleanupInterval) * time.Second,
		BlacklistCheckInterval: time.Duration(config.Pool.BlacklistCheckInterval) * time.Second,
		DNSUpdateInterval:      time.Duration(config.Pool.DNSUpdateInterval) * time.Second,
		MaxRetries:             config.Pool.MaxRetries,
	}

	// 验证配置
	if poolConfig.MaxConnections <= 0 {
		return nil, nil, nil, fmt.Errorf("max_connections 必须大于0")
	}
	if poolConfig.MaxConnsPerHost <= 0 {
		return nil, nil, nil, fmt.Errorf("max_conns_per_host 必须大于0")
	}
	if poolConfig.ConnTimeout <= 0 {
		return nil, nil, nil, fmt.Errorf("conn_timeout 必须大于0")
	}

	return poolConfig, config.Whitelist.IPs, config.Blacklist.IPs, nil
}

// LoadPoolConfigFromFile 从文件加载连接池配置（简化版本）
func LoadPoolConfigFromFile(configPath string) (*PoolConfig, error) {
	poolConfig, _, _, err := LoadConfigFromTOML(configPath)
	return poolConfig, err
}

// LoadMergedPoolConfig 合并读取根目录 config.toml 与 config/config.toml，并转换为 PoolConfig 及白/黑名单
func LoadMergedPoolConfig() (*PoolConfig, []string, []string, error) {
	var cfgFile ConfigFile
	if err := projconfig.LoadMergedInto(&cfgFile); err != nil {
		return nil, nil, nil, err
	}
	poolConfig := &PoolConfig{
		MaxConnections:         cfgFile.Pool.MaxConnections,
		MaxConnsPerHost:        cfgFile.Pool.MaxConnsPerHost,
		MaxIdleConns:           cfgFile.Pool.MaxIdleConns,
		ConnTimeout:            time.Duration(cfgFile.Pool.ConnTimeout) * time.Second,
		IdleTimeout:            time.Duration(cfgFile.Pool.IdleTimeout) * time.Second,
		MaxLifetime:            time.Duration(cfgFile.Pool.MaxLifetime) * time.Second,
		TestTimeout:            time.Duration(cfgFile.Pool.TestTimeout) * time.Second,
		HealthCheckInterval:    time.Duration(cfgFile.Pool.HealthCheckInterval) * time.Second,
		CleanupInterval:        time.Duration(cfgFile.Pool.CleanupInterval) * time.Second,
		BlacklistCheckInterval: time.Duration(cfgFile.Pool.BlacklistCheckInterval) * time.Second,
		DNSUpdateInterval:      time.Duration(cfgFile.Pool.DNSUpdateInterval) * time.Second,
		MaxRetries:             cfgFile.Pool.MaxRetries,
	}
	// 基本校验复用
	if poolConfig.MaxConnections <= 0 {
		return nil, nil, nil, fmt.Errorf("max_connections 必须大于0")
	}
	if poolConfig.MaxConnsPerHost <= 0 {
		return nil, nil, nil, fmt.Errorf("max_conns_per_host 必须大于0")
	}
	if poolConfig.ConnTimeout <= 0 {
		return nil, nil, nil, fmt.Errorf("conn_timeout 必须大于0")
	}
	return poolConfig, cfgFile.Whitelist.IPs, cfgFile.Blacklist.IPs, nil
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MaxConnections         int           `json:"max_connections"`          // 最大连接数
	MaxConnsPerHost        int           `json:"max_conns_per_host"`       // 每个主机最大连接数
	MaxIdleConns           int           `json:"max_idle_conns"`           // 最大空闲连接数
	ConnTimeout            time.Duration `json:"conn_timeout"`             // 连接超时
	IdleTimeout            time.Duration `json:"idle_timeout"`             // 空闲超时
	MaxLifetime            time.Duration `json:"max_lifetime"`             // 连接最大生命周期
	TestTimeout            time.Duration `json:"test_timeout"`             // 测试请求超时
	HealthCheckInterval    time.Duration `json:"health_check_interval"`    // 健康检查间隔
	CleanupInterval        time.Duration `json:"cleanup_interval"`         // 清理间隔
	BlacklistCheckInterval time.Duration `json:"blacklist_check_interval"` // 黑名单检查间隔
	DNSUpdateInterval      time.Duration `json:"dns_update_interval"`      // DNS更新间隔
	MaxRetries             int           `json:"max_retries"`              // 最大重试次数
}

// DefaultPoolConfig 默认连接池配置
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxConnections:         100,
		MaxConnsPerHost:        10,
		MaxIdleConns:           20,
		ConnTimeout:            30 * time.Second,
		IdleTimeout:            60 * time.Second,
		MaxLifetime:            300 * time.Second,
		TestTimeout:            10 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		CleanupInterval:        60 * time.Second,
		BlacklistCheckInterval: 300 * time.Second,  // 5分钟检查一次黑名单
		DNSUpdateInterval:      1800 * time.Second, // 30分钟更新一次DNS
		MaxRetries:             3,
	}
}

// UTLSConnection uTLS连接包装器
type UTLSConnection struct {
	// 基础连接信息
	conn       net.Conn    // TCP连接
	tlsConn    *utls.UConn // uTLS连接
	targetIP   string      // 目标IP
	targetHost string      // 目标域名（用于Host头）

	// 指纹信息
	fingerprint    Profile // 使用的TLS指纹
	acceptLanguage string  // 随机生成的Accept-Language头

	// HTTP/2 支持
	h2ClientConn interface{} // HTTP/2客户端连接（*http2.ClientConn）
	h2Mu         sync.Mutex  // HTTP/2连接锁

	// 生命周期管理
	created     time.Time // 创建时间
	lastUsed    time.Time // 最后使用时间
	lastChecked time.Time // 最后检查时间
	inUse       bool      // 当前使用状态
	healthy     bool      // 连接健康状态

	// 使用统计
	requestCount int64 // 请求次数
	errorCount   int64 // 错误次数

	// 并发控制
	mu   sync.Mutex // 连接级锁
	cond *sync.Cond // 等待条件（用于连接复用）
}

// UTLSHotConnPool 热连接池 - 重构后使用组件化设计
type UTLSHotConnPool struct {
	// 核心组件
	connManager   *ConnectionManager   // 连接管理器
	healthChecker *HealthChecker       // 健康检查器
	validator     *ConnectionValidator // 连接验证器
	ipAccessCtrl  *IPAccessController  // IP访问控制器

	// 连接池配置
	config PoolConfig

	// 依赖模块
	fingerprintLib *Library
	ipPool         IPPoolProvider

	// 统计信息
	stats PoolStats

	// 并发控制
	mu   sync.RWMutex
	done chan struct{}
	wg   sync.WaitGroup
}

// PoolStats 连接池统计信息
type PoolStats struct {
	TotalConnections      int           // 总连接数
	ActiveConnections     int           // 活跃连接数
	IdleConnections       int           // 空闲连接数
	HealthyConnections    int           // 健康连接数
	WhitelistIPs          int           // 白名单IP数
	BlacklistIPs          int           // 黑名单IP数
	TotalRequests         int64         // 总请求数
	SuccessfulRequests    int64         // 成功请求数
	FailedRequests        int64         // 失败请求数
	SuccessRate           float64       // 成功率
	AvgResponseTime       time.Duration // 平均响应时间
	ConnReuseRate         float64       // 连接复用率
	WhitelistMoves        int64         // 黑名单移到白名单数量
	NewConnectionsFromDNS int64         // DNS更新新增连接数
	LastUpdateTime        time.Time     // 最后更新时间
}

// ConnectionStats 单个连接统计信息
type ConnectionStats struct {
	TargetHost   string             // 目标主机
	TargetIP     string             // 目标IP
	Created      time.Time          // 创建时间
	LastUsed     time.Time          // 最后使用时间
	RequestCount int64              // 请求次数
	ErrorCount   int64              // 错误次数
	IsHealthy    bool               // 健康状态
	Fingerprint  utls.ClientHelloID // TLS指纹标识
}

// NewUTLSHotConnPool 创建新的热连接池
func NewUTLSHotConnPool(config *PoolConfig) *UTLSHotConnPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	// 确保全局日志已初始化
	projlogger.GetGlobalLogger()

	// 创建核心组件
	connManager := NewConnectionManager(config)
	healthChecker := NewHealthChecker(connManager, config)
	validator := NewConnectionValidator(config)
	ipAccessCtrl := NewIPAccessController()

	pool := &UTLSHotConnPool{
		connManager:   connManager,
		healthChecker: healthChecker,
		validator:     validator,
		ipAccessCtrl:  ipAccessCtrl,
		config:        *config,
		done:          make(chan struct{}),
	}

	// 启动后台维护任务
	pool.startMaintenanceTasks()

	return pool
}

// SetDependencies 设置依赖模块
func (p *UTLSHotConnPool) SetDependencies(
	fingerprintLib *Library,
	ipPool IPPoolProvider,
	accessControl AccessController,
	logger projlogger.Logger,
) {
	p.fingerprintLib = fingerprintLib
	p.ipPool = ipPool

	// 如果提供了日志记录器，设置为全局日志记录器
	if logger != nil {
		projlogger.SetGlobalLogger(logger)
		projlogger.Info("已设置全局日志记录器")
	}

	// 重新创建组件以使用新的日志记录器
	p.connManager = NewConnectionManager(&p.config)
	p.healthChecker = NewHealthChecker(p.connManager, &p.config)
	p.validator = NewConnectionValidator(&p.config)
	p.ipAccessCtrl = NewIPAccessController()

	// 如果提供了自定义的访问控制器，需要适配
	if accessControl != nil {
		// 这里可以使用适配器模式将现有的AccessController适配到IPAccessController
		// 为了简化，这里暂时使用内置的IPAccessController
		projlogger.Info("使用内置IPAccessController，忽略提供的AccessController")
	}
}

// GetConnection 获取连接
func (p *UTLSHotConnPool) GetConnection(targetHost string) (*UTLSConnection, error) {
	// 1. 尝试获取现有热连接
	if conn := p.getExistingConnection(targetHost); conn != nil {
		return conn, nil
	}

	// 2. 创建新的热连接
	return p.createNewHotConnection(targetHost)
}

// GetConnectionWithValidation 获取热连接并验证指定路径的可用性
func (p *UTLSHotConnPool) GetConnectionWithValidation(fullURL string) (*UTLSConnection, error) {
	// 解析URL
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("解析URL失败: %w", err)
	}

	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("只支持HTTPS协议")
	}

	targetHost := parsedURL.Host
	if targetHost == "" {
		return nil, fmt.Errorf("无效的主机名")
	}

	// 1. 尝试获取现有热连接
	if conn := p.getExistingConnection(targetHost); conn != nil {
		// 验证连接对指定路径的可用性
		if err := p.validateConnectionWithPath(conn, parsedURL.Path); err == nil {
			return conn, nil
		}
		// 如果验证失败，标记连接为不健康并移除
		p.removeFromPool(conn)
	}

	// 2. 创建新的热连接并验证
	conn, err := p.createNewHotConnectionWithValidation(targetHost, parsedURL.Path)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// GetConnectionToIP 获取到指定IP的连接（用于IP池测试）
func (p *UTLSHotConnPool) GetConnectionToIP(fullURL, targetIP string) (*UTLSConnection, error) {
	// 解析URL获取主机名
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("解析URL失败: %w", err)
	}

	if parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("只支持HTTPS协议")
	}

	targetHost := parsedURL.Host
	if targetHost == "" {
		return nil, fmt.Errorf("无效的主机名")
	}

	// 1. 尝试从池中获取到该IP的现有连接
	if conn := p.getExistingConnectionToIP(targetHost, targetIP); conn != nil {
		// 验证连接对指定路径的可用性
		if err := p.validateConnectionWithPath(conn, parsedURL.Path); err == nil {
			projlogger.Debug("复用IP池连接: %s -> %s", targetHost, targetIP)
			return conn, nil
		}
		// 如果验证失败，标记连接为不健康并移除
		p.removeFromPool(conn)
	}

	// 2. 直接创建到指定IP的新连接
	projlogger.Debug("创建到指定IP的连接: %s -> %s", targetHost, targetIP)

	// 选择TLS指纹
	fingerprint := p.selectFingerprint()

	// 创建并验证连接
	conn, err := p.createAndValidateConnection(targetIP, targetHost, fingerprint, parsedURL.Path)
	if err != nil {
		return nil, fmt.Errorf("创建到IP %s 的连接失败: %w", targetIP, err)
	}

	return conn, nil
}

// getExistingConnection 获取现有连接
func (p *UTLSHotConnPool) getExistingConnection(targetHost string) *UTLSConnection {
	// 使用ConnectionManager获取该域名的所有连接
	connections := p.connManager.GetConnectionsForHost(targetHost)

	// 随机选择一个健康的连接
	for _, conn := range connections {
		conn.mu.Lock()
		// 先检查基本条件
		if conn.inUse || !conn.healthy {
			conn.mu.Unlock()
			continue
		}

		// 解锁后再进行健康检查（避免死锁）
		conn.mu.Unlock()

		// 健康检查
		if !p.healthChecker.CheckConnection(conn) {
			continue
		}

		// 再次加锁并标记为使用中
		conn.mu.Lock()
		// 双重检查：可能在解锁期间被其他goroutine获取
		if conn.inUse || !conn.healthy {
			conn.mu.Unlock()
			continue
		}

		// 标记为使用中
		conn.inUse = true
		conn.lastUsed = time.Now()
		atomic.AddInt64(&conn.requestCount, 1)
		conn.mu.Unlock()
		projlogger.Debug("复用现有连接: %s -> %s", targetHost, conn.targetIP)
		return conn
	}

	return nil
}

// getExistingConnectionToIP 获取到指定IP的现有连接
func (p *UTLSHotConnPool) getExistingConnectionToIP(targetHost, targetIP string) *UTLSConnection {
	// 使用ConnectionManager获取该域名的所有连接
	connections := p.connManager.GetConnectionsForHost(targetHost)

	// 查找到指定IP的健康连接
	for _, conn := range connections {
		// 检查IP是否匹配
		if conn.targetIP != targetIP {
			continue
		}

		conn.mu.Lock()
		// 检查基本条件
		if conn.inUse || !conn.healthy {
			conn.mu.Unlock()
			continue
		}

		// 解锁后再进行健康检查（避免死锁）
		conn.mu.Unlock()

		// 健康检查
		if !p.healthChecker.CheckConnection(conn) {
			continue
		}

		// 再次加锁并标记为使用中
		conn.mu.Lock()
		// 双重检查：可能在解锁期间被其他goroutine获取
		if conn.inUse || !conn.healthy {
			conn.mu.Unlock()
			continue
		}

		// 标记为使用中
		conn.inUse = true
		conn.lastUsed = time.Now()
		atomic.AddInt64(&conn.requestCount, 1)
		conn.mu.Unlock()
		return conn
	}

	return nil
}

// createNewHotConnection 创建新的热连接
func (p *UTLSHotConnPool) createNewHotConnection(targetHost string) (*UTLSConnection, error) {
	return p.createNewHotConnectionWithPath(targetHost, "")
}

// createNewHotConnectionWithPath 创建新的热连接并验证指定路径（内部方法）
func (p *UTLSHotConnPool) createNewHotConnectionWithPath(targetHost, path string) (*UTLSConnection, error) {
	// 使用有界重试避免递归导致的无限尝试
	maxAttempts := p.config.MaxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	var lastErr error
	for attempts := 0; attempts < maxAttempts; attempts++ {
		// 1. 获取IP
		ip, err := p.acquireIP(targetHost)
		if err != nil {
			lastErr = err
			continue
		}

		// 2. 检查IP是否允许访问
		if !p.validateIPAccess(ip) {
			// IP在黑名单中，跳过并尝试下一次获取
			lastErr = fmt.Errorf("IP被拒绝: %s", ip)
			continue
		}

		// 3. 选择TLS指纹
		fingerprint := p.selectFingerprint()

		// 4. 创建并验证连接
		conn, err := p.createAndValidateConnection(ip, targetHost, fingerprint, path)
		if err != nil {
			lastErr = err
			continue
		}
		return conn, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("无法创建连接: 尝试次数耗尽")
	}
	return nil, lastErr
}

// createNewHotConnectionWithValidation 创建新的热连接并验证指定路径
func (p *UTLSHotConnPool) createNewHotConnectionWithValidation(targetHost, path string) (*UTLSConnection, error) {
	return p.createNewHotConnectionWithPath(targetHost, path)
}

// establishConnection 建立连接
func (p *UTLSHotConnPool) establishConnection(ip, targetHost string, fingerprint Profile) (*UTLSConnection, error) {
	// 1. 建立TCP连接
	// 处理IPv6地址格式：需要用方括号包裹
	var address string
	if strings.Contains(ip, ":") {
		// IPv6地址
		address = fmt.Sprintf("[%s]:%d", ip, DefaultHTTPSPort)
	} else {
		// IPv4地址
		address = fmt.Sprintf("%s:%d", ip, DefaultHTTPSPort)
	}

	tcpConn, err := net.DialTimeout("tcp", address, p.config.ConnTimeout)
	if err != nil {
		return nil, fmt.Errorf("TCP连接失败: %v", err)
	}

	// 2. 建立TLS连接
	tlsConn := utls.UClient(tcpConn, &utls.Config{
		ServerName:         targetHost,
		InsecureSkipVerify: false,
		NextProtos:         []string{"h2", "http/1.1"},
		OmitEmptyPsk:       true, // 避免空 PSK 问题
	}, fingerprint.HelloID)

	// 4. 执行TLS握手
	if err := tlsConn.Handshake(); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("TLS握手失败: %v", err)
	}

	// 5. 检测协商的协议
	negotiatedProto := tlsConn.ConnectionState().NegotiatedProtocol
	projlogger.Debug("TLS握手成功，协商协议: %s", negotiatedProto)

	// 6. 包装连接对象
	conn := &UTLSConnection{
		conn:           tcpConn,
		tlsConn:        tlsConn,
		fingerprint:    fingerprint,
		acceptLanguage: fpLibrary.RandomAcceptLanguage(), // 随机生成Accept-Language
		targetIP:       ip,
		targetHost:     targetHost,
		created:        time.Now(),
		lastUsed:       time.Now(),
		lastChecked:    time.Now(),
		inUse:          true,
		healthy:        true,
		requestCount:   1,
	}

	// 初始化条件变量（修复并发安全问题）
	conn.cond = sync.NewCond(&conn.mu)

	return conn, nil
}

// validateConnection 验证连接
func (p *UTLSHotConnPool) validateConnection(conn *UTLSConnection) error {
	return p.validateConnectionWithPath(conn, "/")
}

// validateConnectionWithPath 验证连接对指定路径的可用性
func (p *UTLSHotConnPool) validateConnectionWithPath(conn *UTLSConnection, path string) error {
	// 对于已经建立的连接，只做简单的健康检查
	// 检测协商的协议
	negotiatedProto := conn.tlsConn.ConnectionState().NegotiatedProtocol

	// HTTP/2 连接的验证：只检查连接状态，不发送验证请求
	if negotiatedProto == "h2" {
		conn.mu.Lock()
		defer conn.mu.Unlock()

		// 检查连接是否健康
		if !conn.healthy {
			return fmt.Errorf("连接不健康")
		}

		// 检查是否超时
		if time.Since(conn.created) > p.config.MaxLifetime {
			conn.healthy = false
			return fmt.Errorf("连接已超时")
		}

		// HTTP/2 连接验证通过
		conn.lastChecked = time.Now()
		projlogger.Debug("HTTP/2连接验证通过: %s", conn.targetIP)
		return nil
	}

	// HTTP/1.1 连接的验证：发送 HEAD 请求
	// 确保path以/开头
	if path == "" {
		path = "/"
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// 构造最小化的 HEAD 原始请求（不通过客户端栈，不影响统计）
	raw := strings.Builder{}
	raw.WriteString("HEAD ")
	raw.WriteString(path)
	raw.WriteString(" HTTP/1.1\r\n")
	raw.WriteString("Host: ")
	raw.WriteString(conn.targetHost)
	raw.WriteString("\r\n")
	raw.WriteString("User-Agent: ")
	raw.WriteString(conn.fingerprint.UserAgent)
	raw.WriteString("\r\n")
	raw.WriteString("Accept-Language: ")
	raw.WriteString(conn.acceptLanguage) // 使用随机生成的Accept-Language
	raw.WriteString("\r\n")
	raw.WriteString("Connection: keep-alive\r\n")
	raw.WriteString("Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8\r\n")
	raw.WriteString("\r\n")

	// 发送请求并读取响应（直接在 tlsConn 上进行）
	// 不修改 requestCount/lastUsed，保持非侵入
	// 为避免竞态，验证阶段该连接已处于 inUse=true
	if _, err := conn.tlsConn.Write([]byte(raw.String())); err != nil {
		conn.mu.Lock()
		conn.healthy = false
		conn.errorCount++
		conn.mu.Unlock()
		return fmt.Errorf("连接测试失败: %v", err)
	}

	reader := bufio.NewReader(conn.tlsConn)
	// 读取状态行
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		conn.mu.Lock()
		conn.healthy = false
		conn.errorCount++
		conn.mu.Unlock()
		return fmt.Errorf("连接测试失败: %v", err)
	}
	parts := strings.Split(strings.TrimSpace(statusLine), " ")
	if len(parts) < 3 {
		conn.mu.Lock()
		conn.healthy = false
		conn.mu.Unlock()
		return fmt.Errorf("连接测试失败: 无效状态行: %s", statusLine)
	}
	var code int
	if _, err := fmt.Sscanf(parts[1], "%d", &code); err != nil {
		conn.mu.Lock()
		conn.healthy = false
		conn.mu.Unlock()
		return fmt.Errorf("连接测试失败: 解析状态码失败: %v", err)
	}

	// 跳过响应头（读到空行）
	for {
		line, e := reader.ReadString('\n')
		if e != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
	}

	// 根据状态码判断连接质量
	switch code {
	case StatusOK, StatusNoContent:
		conn.mu.Lock()
		conn.healthy = true
		conn.lastChecked = time.Now()
		conn.mu.Unlock()
		return nil
	case StatusForbidden:
		conn.mu.Lock()
		conn.healthy = false
		conn.mu.Unlock()
		return fmt.Errorf("%w: IP %s 返回403", ErrIPBlocked, conn.targetIP)
	default:
		conn.mu.Lock()
		conn.healthy = false
		conn.mu.Unlock()
		return fmt.Errorf("连接验证失败，状态码: %d", code)
	}
}

// addToPool 将连接加入连接池
func (p *UTLSHotConnPool) addToPool(conn *UTLSConnection) {
	// 使用ConnectionManager添加连接
	p.connManager.AddConnection(conn)

	// 更新黑白名单
	if p.ipAccessCtrl != nil {
		p.ipAccessCtrl.AddIP(conn.targetIP, true)
		projlogger.Debug("连接已添加到白名单: %s", conn.targetIP)
	}

	projlogger.Info("新连接已添加到连接池: %s -> %s", conn.targetHost, conn.targetIP)
}

// PutConnection 归还连接
func (p *UTLSHotConnPool) PutConnection(conn *UTLSConnection) {
	if conn == nil {
		return
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	// 更新使用状态
	conn.inUse = false
	conn.lastUsed = time.Now()

	// 检查连接健康状态
	if !conn.healthy {
		p.connManager.RemoveConnection(conn.targetIP)
		return
	}

	// 唤醒等待的goroutine
	if conn.cond != nil {
		conn.cond.Broadcast()
	}

	projlogger.Debug("连接已归还: %s", conn.targetIP)
}

// removeFromPool 从连接池移除连接
func (p *UTLSHotConnPool) removeFromPool(conn *UTLSConnection) {
	p.connManager.RemoveConnection(conn.targetIP)
	conn.Close()
}

// isConnectionValid 检查连接是否有效
func (p *UTLSHotConnPool) isConnectionValid(conn *UTLSConnection) bool {
	// 检查连接年龄
	if time.Since(conn.created) > p.config.MaxLifetime {
		return false
	}

	// 检查空闲时间
	if time.Since(conn.lastUsed) > p.config.IdleTimeout {
		return false
	}

	// 检查健康状态
	return conn.healthy
}

// startMaintenanceTasks 启动后台维护任务
func (p *UTLSHotConnPool) startMaintenanceTasks() {
	// 健康检查任务
	p.wg.Add(1)
	go p.healthCheckLoop()

	// 清理任务
	p.wg.Add(1)
	go p.cleanupLoop()

	// 黑名单检查任务
	p.wg.Add(1)
	go p.blacklistCheckLoop()

	// DNS更新任务
	p.wg.Add(1)
	go p.dnsUpdateLoop()
}

// healthCheckLoop 健康检查循环
func (p *UTLSHotConnPool) healthCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performHealthCheck()
		case <-p.done:
			return
		}
	}
}

// cleanupLoop 清理循环
func (p *UTLSHotConnPool) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performCleanup()
		case <-p.done:
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (p *UTLSHotConnPool) performHealthCheck() {
	// 使用HealthChecker执行健康检查
	p.healthChecker.CheckAllConnections()
}

// checkSingleConnection 检查单个连接
func (p *UTLSHotConnPool) checkSingleConnection(conn *UTLSConnection) {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.inUse {
		return // 正在使用的连接跳过检查
	}

	// 重新验证连接
	err := p.validateConnection(conn)
	if err != nil {
		conn.healthy = false
		p.removeFromPool(conn)
	}
}

// performCleanup 执行清理
func (p *UTLSHotConnPool) performCleanup() {
	// 使用ConnectionManager清理过期连接
	p.connManager.CleanupExpiredConnections(p.config.MaxLifetime)
}

// blacklistCheckLoop 黑名单检查循环
func (p *UTLSHotConnPool) blacklistCheckLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.BlacklistCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performBlacklistCheck()
		case <-p.done:
			return
		}
	}
}

// performBlacklistCheck 执行黑名单检查
func (p *UTLSHotConnPool) performBlacklistCheck() {
	if p.ipAccessCtrl == nil {
		return
	}

	// 获取黑名单IP列表
	blacklistIPs := p.ipAccessCtrl.GetBlockedIPs()

	if len(blacklistIPs) == 0 {
		return
	}

	// 清理黑名单中的连接
	for _, ip := range blacklistIPs {
		p.connManager.RemoveConnection(ip)
		projlogger.Debug("清理黑名单连接: %s", ip)
	}
}

// getRandomFingerprint 获取随机TLS指纹
func (p *UTLSHotConnPool) getRandomFingerprint() Profile {
	var fingerprint Profile
	if p.fingerprintLib != nil {
		fingerprint = p.fingerprintLib.RandomRecommendedProfile()
	} else {
		// 默认指纹
		fingerprint = Profile{
			Name:      "Chrome",
			HelloID:   utls.HelloChrome_102,
			UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/102.0.0.0 Safari/537.36",
		}
	}
	return fingerprint
}

// testIPForWhitelist 测试IP是否可以移到白名单
func (p *UTLSHotConnPool) testIPForWhitelist(ip string) bool {
	// 获取该IP对应的目标域名
	hostMapping := p.connManager.GetHostMapping()
	var targetHost string
	for host, ipList := range hostMapping {
		for _, listedIP := range ipList {
			if listedIP == ip {
				targetHost = host
				break
			}
		}
		if targetHost != "" {
			break
		}
	}

	if targetHost == "" {
		return false
	}

	// 尝试建立连接并测试
	conn, err := p.establishConnection(ip, targetHost, p.getRandomFingerprint())
	if err != nil {
		return false
	}
	defer conn.Close()

	// 发送HEAD请求测试
	testReq, _ := http.NewRequest("HEAD", "https://"+targetHost, nil)
	testReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	testReq.Header.Set("Host", targetHost)

	ctx, cancel := context.WithTimeout(context.Background(), p.config.TestTimeout)
	defer cancel()

	client := &http.Client{
		Transport: &http.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return conn.conn, nil
			},
		},
	}

	resp, err := client.Do(testReq.WithContext(ctx))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 返回200表示可以移到白名单
	return resp.StatusCode == 200
}

// dnsUpdateLoop DNS更新循环
func (p *UTLSHotConnPool) dnsUpdateLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.DNSUpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performDNSUpdate()
		case <-p.done:
			return
		}
	}
}

// performDNSUpdate 执行DNS更新
func (p *UTLSHotConnPool) performDNSUpdate() {
	if p.ipPool == nil {
		return
	}

	// 获取所有已知的域名
	hostMapping := p.connManager.GetHostMapping()
	var domains []string
	for host := range hostMapping {
		domains = append(domains, host)
	}

	if len(domains) == 0 {
		return
	}

	// 并发更新每个域名的IP
	var wg sync.WaitGroup
	var mu sync.Mutex
	var newConnections []*UTLSConnection

	for _, domain := range domains {
		wg.Add(1)
		go func(d string) {
			defer wg.Done()

			// 获取新的IP列表
			newIPs := p.getUpdatedIPsForDomain(d)

			// 为新IP建立热连接
			for _, ip := range newIPs {
				if p.isNewIPForDomain(ip, d) {
					conn, err := p.createNewHotConnectionWithHost(ip, d)
					if err != nil {
						continue
					}

					mu.Lock()
					newConnections = append(newConnections, conn)
					mu.Unlock()
				}
			}
		}(domain)
	}
	wg.Wait()

	// 将新连接加入连接池
	for _, conn := range newConnections {
		p.PutConnection(conn)
		atomic.AddInt64(&p.stats.NewConnectionsFromDNS, 1)
	}
}

// getUpdatedIPsForDomain 获取域名的最新IP列表
func (p *UTLSHotConnPool) getUpdatedIPsForDomain(domain string) []string {
	if p.ipPool == nil {
		return nil
	}

	// 通过IP池获取最新IP
	if p.ipPool != nil {
		return p.ipPool.GetIPsForDomain(domain)
	}

	return nil
}

// isNewIPForDomain 检查IP是否是该域名的新IP
func (p *UTLSHotConnPool) isNewIPForDomain(ip, domain string) bool {
	// 检查连接是否已存在
	if p.connManager.GetConnection(ip) != nil {
		return false
	}

	// 检查域名映射中是否已包含该IP
	if ipList, exists := p.connManager.GetHostMapping()[domain]; exists {
		for _, existingIP := range ipList {
			if existingIP == ip {
				return false
			}
		}
	}

	return true
}

// createNewHotConnectionWithHost 为指定IP和域名创建热连接
func (p *UTLSHotConnPool) createNewHotConnectionWithHost(ip, host string) (*UTLSConnection, error) {
	fingerprint := p.getRandomFingerprint()
	conn, err := p.establishConnection(ip, host, fingerprint)
	if err != nil {
		return nil, err
	}

	// 验证连接
	if err := p.validateConnection(conn); err != nil {
		conn.Close()
		return nil, err
	}

	// 添加到连接池
	p.addToPool(conn)

	return conn, nil
}

// GetStats 获取连接池统计信息
func (p *UTLSHotConnPool) GetStats() PoolStats {
	// 获取所有连接
	connections := make(map[string]*UTLSConnection)
	hostMapping := p.connManager.GetHostMapping()

	for _, ips := range hostMapping {
		for _, ip := range ips {
			if conn := p.connManager.GetConnection(ip); conn != nil {
				connections[ip] = conn
			}
		}
	}

	stats := PoolStats{
		TotalConnections:      len(connections),
		WhitelistMoves:        atomic.LoadInt64(&p.stats.WhitelistMoves),
		NewConnectionsFromDNS: atomic.LoadInt64(&p.stats.NewConnectionsFromDNS),
		LastUpdateTime:        time.Now(),
	}

	// 统计连接状态
	for _, conn := range connections {
		conn.mu.Lock()
		if conn.inUse {
			stats.ActiveConnections++
		} else {
			stats.IdleConnections++
		}

		if conn.healthy {
			stats.HealthyConnections++
		}

		stats.TotalRequests += conn.requestCount
		stats.FailedRequests += conn.errorCount
		conn.mu.Unlock()
	}

	// 统计黑白名单
	if p.ipAccessCtrl != nil {
		stats.WhitelistIPs = len(p.ipAccessCtrl.GetAllowedIPs())
		stats.BlacklistIPs = len(p.ipAccessCtrl.GetBlockedIPs())
	}

	// 计算成功率
	if stats.TotalRequests > 0 {
		stats.SuccessfulRequests = stats.TotalRequests - stats.FailedRequests
		stats.SuccessRate = float64(stats.SuccessfulRequests) / float64(stats.TotalRequests)
	}

	return stats
}

// Close 关闭连接池
func (p *UTLSHotConnPool) Close() error {
	close(p.done)

	// 等待后台任务结束
	p.wg.Wait()

	// 关闭所有连接
	return p.connManager.Close()
}

// TargetHost 返回目标主机名
func (conn *UTLSConnection) TargetHost() string {
	return conn.targetHost
}

// TargetIP 返回目标IP地址
func (conn *UTLSConnection) TargetIP() string {
	return conn.targetIP
}

// Fingerprint 返回使用的TLS指纹
func (conn *UTLSConnection) Fingerprint() Profile {
	return conn.fingerprint
}

// AcceptLanguage 返回连接的Accept-Language头部值
func (conn *UTLSConnection) AcceptLanguage() string {
	return conn.acceptLanguage
}

// Created 返回连接创建时间
func (conn *UTLSConnection) Created() time.Time {
	return conn.created
}

// LastUsed 返回最后使用时间
func (conn *UTLSConnection) LastUsed() time.Time {
	return conn.lastUsed
}

// RequestCount 返回请求次数
func (conn *UTLSConnection) RequestCount() int64 {
	return atomic.LoadInt64(&conn.requestCount)
}

// ErrorCount 返回错误次数
func (conn *UTLSConnection) ErrorCount() int64 {
	return atomic.LoadInt64(&conn.errorCount)
}

// IsHealthy 返回连接健康状态
func (conn *UTLSConnection) IsHealthy() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.healthy
}

// Stats 返回连接统计信息
func (conn *UTLSConnection) Stats() ConnectionStats {
	return ConnectionStats{
		TargetHost:   conn.targetHost,
		TargetIP:     conn.targetIP,
		Created:      conn.created,
		LastUsed:     conn.lastUsed,
		RequestCount: conn.requestCount,
		ErrorCount:   conn.errorCount,
		IsHealthy:    conn.healthy,
		Fingerprint:  conn.fingerprint.HelloID,
	}
}

// Close 关闭单个连接
func (conn *UTLSConnection) Close() error {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	// 关闭 HTTP/2 客户端连接
	conn.h2Mu.Lock()
	if conn.h2ClientConn != nil {
		if cc, ok := conn.h2ClientConn.(interface{ Close() error }); ok {
			cc.Close()
		}
		conn.h2ClientConn = nil
	}
	conn.h2Mu.Unlock()

	if conn.tlsConn != nil {
		conn.tlsConn.Close()
	}
	if conn.conn != nil {
		conn.conn.Close()
	}

	conn.healthy = false
	return nil
}

// RoundTripRaw 发送原始HTTP请求并返回一个可读响应流（封装连接层细节）
func (conn *UTLSConnection) RoundTripRaw(ctx context.Context, rawReq []byte) (io.Reader, error) {
	// 写入请求
	_, err := conn.tlsConn.Write(rawReq)
	if err != nil {
		// 连接错误时更新健康状态与错误计数
		conn.mu.Lock()
		conn.healthy = false
		conn.errorCount++
		conn.mu.Unlock()
		return nil, err
	}

	// 更新使用统计
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.requestCount++
	conn.mu.Unlock()

	// 返回读取器用于解析响应
	return bufio.NewReader(conn.tlsConn), nil
}

// WaitForAvailable 等待连接可用
func (conn *UTLSConnection) WaitForAvailable(timeout time.Duration) error {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	// 条件变量应该在创建连接时初始化，这里不应该为nil
	if conn.cond == nil {
		// 如果确实为nil（不应该发生），创建一个新的
		conn.cond = sync.NewCond(&conn.mu)
	}

	timeoutChan := time.After(timeout)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for conn.inUse {
			conn.cond.Wait()
		}
	}()

	select {
	case <-done:
		return nil
	case <-timeoutChan:
		return fmt.Errorf("等待连接超时")
	}
}

// ExtractHostname 从目标URL或域名中提取主机名（导出以供测试使用）
func ExtractHostname(target string) string {
	// 如果包含协议，直接解析
	if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
		u, err := url.Parse(target)
		if err != nil {
			return target
		}
		return u.Hostname()
	}

	// 如果不包含协议，添加https协议前缀进行解析
	u, err := url.Parse("https://" + target)
	if err != nil {
		return target
	}
	return u.Hostname()
}

// GetWhitelist 获取白名单IP列表
func (p *UTLSHotConnPool) GetWhitelist() []string {
	if p.ipAccessCtrl != nil {
		return p.ipAccessCtrl.GetAllowedIPs()
	}
	return []string{}
}

// GetBlacklist 获取黑名单IP列表
func (p *UTLSHotConnPool) GetBlacklist() []string {
	if p.ipAccessCtrl != nil {
		return p.ipAccessCtrl.GetBlockedIPs()
	}
	return []string{}
}

// GetConnectionCount 获取指定域名的连接数
func (p *UTLSHotConnPool) GetConnectionCount(host string) int {
	connections := p.connManager.GetConnectionsForHost(host)

	count := 0
	for _, conn := range connections {
		conn.mu.Lock()
		if conn.healthy {
			count++
		}
		conn.mu.Unlock()
	}

	return count
}

// IsHealthy 检查连接池是否健康
func (p *UTLSHotConnPool) IsHealthy() bool {
	stats := p.GetStats()
	return stats.TotalConnections > 0 &&
		stats.HealthyConnections > 0 &&
		stats.SuccessRate >= MinSuccessRate
}

// ForceCleanup 强制清理所有连接
func (p *UTLSHotConnPool) ForceCleanup() {
	// 获取所有连接并关闭
	hostMapping := p.connManager.GetHostMapping()
	for _, ips := range hostMapping {
		for _, ip := range ips {
			p.connManager.RemoveConnection(ip)
		}
	}
}

// PreWarmConnections 预热连接到指定域名
func (p *UTLSHotConnPool) PreWarmConnections(host string, count int) error {
	var connections []*UTLSConnection
	var errors []error

	// 创建指定数量的连接
	for i := 0; i < count; i++ {
		conn, err := p.createNewHotConnection(host)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		connections = append(connections, conn)
	}

	// 如果所有连接都失败，返回错误
	if len(connections) == 0 && len(errors) > 0 {
		return fmt.Errorf("预热连接失败: %v", errors)
	}

	// 将连接标记为空闲状态
	for _, conn := range connections {
		p.PutConnection(conn)
	}

	return nil
}

// UpdateConfig 更新连接池配置
func (p *UTLSHotConnPool) UpdateConfig(newConfig *PoolConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.config = *newConfig
}

// GetConnectionInfo 获取连接详细信息
func (p *UTLSHotConnPool) GetConnectionInfo(ip string) map[string]interface{} {
	conn := p.connManager.GetConnection(ip)
	if conn != nil {
		conn.mu.Lock()
		defer conn.mu.Unlock()

		return map[string]interface{}{
			"target_ip":     conn.targetIP,
			"target_host":   conn.targetHost,
			"created":       conn.created,
			"last_used":     conn.lastUsed,
			"last_checked":  conn.lastChecked,
			"in_use":        conn.inUse,
			"healthy":       conn.healthy,
			"request_count": conn.requestCount,
			"error_count":   conn.errorCount,
			"fingerprint":   conn.fingerprint.Name,
		}
	}

	return nil
}
