package utlsclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crawler-platform/localippool"
	projlogger "crawler-platform/logger"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// UTLSConnection uTLS连接包装器
type UTLSConnection struct {
	conn       net.Conn
	tlsConn    *utls.UConn
	targetIP   string
	targetHost string
	localIP    string // 本地源 IP 地址（如果使用了本地 IP 池）

	fingerprint    Profile
	acceptLanguage string
	sessionID      string

	h2ClientConn *http2.ClientConn
	h2Mu         sync.Mutex

	created  time.Time
	lastUsed time.Time // 最后使用时间，用于空闲超时检查
	healthy  bool
	inUse    bool

	requestCount int64
	errorCount   int64

	// on403 回调函数，当检测到403错误时调用，用于将IP加入黑名单
	on403 func(ip string)

	mu sync.Mutex
}

// TargetIP 返回此连接当前绑定的目标 IP。
func (c *UTLSConnection) TargetIP() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.targetIP
}

// TargetHost 返回此连接当前绑定的目标主机名（域名）。
func (c *UTLSConnection) TargetHost() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.targetHost
}

// LocalIP 返回此连接使用的本地源 IP 地址（如果使用了本地 IP 池）。
func (c *UTLSConnection) LocalIP() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localIP
}

// IsHealthy 检查连接是否被标记为健康。
func (c *UTLSConnection) IsHealthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.healthy
}

// TryAcquire 尝试以非阻塞方式获取连接。如果成功，返回true。
func (c *UTLSConnection) TryAcquire() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inUse || !c.healthy {
		return false
	}
	c.inUse = true
	return true
}

// SetSessionID 设置连接的 Session ID。
func (c *UTLSConnection) SetSessionID(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sessionID = sessionID
}

// Close 关闭底层连接并标记为不健康。
func (c *UTLSConnection) Close() error {
	c.mu.Lock()
	if !c.healthy {
		c.mu.Unlock()
		return nil // 避免重复关闭
	}
	c.healthy = false
	c.mu.Unlock()

	// 先关闭 HTTP/2 连接，确保 readLoop goroutine 能够退出
	// 使用 h2Mu 锁保护，避免与 roundTripH2 并发
	c.h2Mu.Lock()
	h2ClientConn := c.h2ClientConn
	c.h2ClientConn = nil // 先清空引用，防止其他 goroutine 继续使用
	c.h2Mu.Unlock()

	var firstErr error
	if h2ClientConn != nil {
		// 关闭 HTTP/2 连接，这会停止 readLoop goroutine
		// 注意：关闭 HTTP/2 连接会触发底层连接的关闭，readLoop 会自然退出
		if err := h2ClientConn.Close(); err != nil {
			firstErr = err
		}
	}

	// 然后关闭 TLS 和 TCP 连接
	// 注意：如果 h2ClientConn 已经关闭，底层连接可能已经被关闭
	// 但为了确保资源完全释放，我们仍然尝试关闭它们
	c.mu.Lock()
	tlsConn := c.tlsConn
	conn := c.conn
	c.mu.Unlock()

	if tlsConn != nil {
		if err := tlsConn.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if conn != nil {
		if err := conn.Close(); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// RoundTrip 执行一个完整的HTTP请求-响应周期。
func (c *UTLSConnection) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt64(&c.requestCount, 1)

	// 安全地读取共享字段，并检查连接健康状态
	c.mu.Lock()
	if !c.healthy {
		c.mu.Unlock()
		return nil, fmt.Errorf("连接已标记为不健康")
	}
	// 更新最后使用时间
	c.lastUsed = time.Now()
	fingerprint := c.fingerprint
	sessionID := c.sessionID
	targetHost := c.targetHost
	acceptLanguage := c.acceptLanguage
	tlsConn := c.tlsConn
	c.mu.Unlock()

	// 统一设置必要的请求头，确保所有请求都有一致的请求头
	if req.Header.Get("User-Agent") == "" {
		// projlogger.Debug("设置User-Agent: %s", fingerprint.UserAgent)
		req.Header.Set("User-Agent", fingerprint.UserAgent)
	}
	if req.Header.Get("Host") == "" {
		// 如果没有设置 Host 头，使用连接的 targetHost
		req.Header.Set("Host", targetHost)
	}
	if req.Header.Get("Accept") == "" {
		// 设置 Accept 头，与 curl 测试一致
		req.Header.Set("Accept", "*/*")
	}
	if sessionID != "" {
		// projlogger.Debug("设置Cookie: %s", sessionID)
		req.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", sessionID))
	}
	if acceptLanguage != "" && req.Header.Get("Accept-Language") == "" {
		// projlogger.Debug("设置Accept-Language: %s", acceptLanguage)
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "keep-alive")
	}
	// 对于有请求体的请求，如果没有设置 Content-Type，则设置默认值
	// 注意：只有当请求体不为空时才设置，避免 GET 请求被错误设置
	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	}
	//projlogger.Info("请求的远程ip是: %v,请求次数: %v", c.targetIP, c.requestCount)
	// 调试：记录请求详细信息（仅在 DEBUG 级别）
	//projlogger.Debug("请求URL: %s, 请求头: %v", req.URL.String(), req.Header)

	// ConnectionState() 是线程安全的，但为了保持一致性，我们在锁外调用
	negotiatedProto := tlsConn.ConnectionState().NegotiatedProtocol
	if negotiatedProto == "h2" {
		return c.roundTripH2(req)
	}
	return c.roundTripH1(req)
}

func (c *UTLSConnection) roundTripH1(req *http.Request) (*http.Response, error) {
	err := req.Write(c.tlsConn)
	if err != nil {
		// 网络错误不标记为不健康，允许重试（只有403才标记为不健康）
		// 连接断开是正常的，下次使用时会自动恢复
		return nil, err
	}
	resp, err := http.ReadResponse(bufio.NewReader(c.tlsConn), req)
	if err != nil {
		// 网络错误不标记为不健康，允许重试（只有403才标记为不健康）
		// 连接断开是正常的，下次使用时会自动恢复
		return nil, err
	}

	// 检测403错误，将IP加入黑名单（只有403才标记为不健康）
	if resp.StatusCode == http.StatusForbidden {
		c.handle403()
	}

	//projlogger.Debug("Http1.1响应头: %v", resp.Header)
	return resp, nil
}

func (c *UTLSConnection) roundTripH2(req *http.Request) (*http.Response, error) {
	// 先检查连接是否健康
	c.mu.Lock()
	if !c.healthy {
		c.mu.Unlock()
		return nil, fmt.Errorf("连接已标记为不健康")
	}
	c.mu.Unlock()

	c.h2Mu.Lock()
	// 检查 HTTP/2 连接是否存在且可用
	if c.h2ClientConn == nil || !c.h2ClientConn.CanTakeNewRequest() {
		// 如果连接不存在或不可用，先关闭旧连接（如果存在）
		if c.h2ClientConn != nil {
			c.h2ClientConn.Close()
			c.h2ClientConn = nil
		}

		// 创建新的 HTTP/2 连接
		t := &http2.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return c.tlsConn, nil
			},
		}
		clientConn, err := t.NewClientConn(c.tlsConn)
		if err != nil {
			c.h2Mu.Unlock()
			// 创建 HTTP/2 连接失败，不标记为不健康（只有403才标记为不健康）
			// 连接关闭是正常的，返回错误让上层重试，重试时会获取新的连接
			return nil, fmt.Errorf("创建 HTTP/2 连接失败: %w", err)
		}
		c.h2ClientConn = clientConn
	}
	h2Conn := c.h2ClientConn
	//projlogger.Info("请求的远程ip是: %v", c.targetIP)
	c.h2Mu.Unlock()

	resp, err := h2Conn.RoundTrip(req)
	if err != nil {
		// 请求失败，关闭 HTTP/2 连接，避免 readLoop goroutine 泄漏
		// 但不标记为不健康，下次使用时会自动重建连接（只有403才标记为不健康）
		c.h2Mu.Lock()
		if c.h2ClientConn == h2Conn {
			c.h2ClientConn.Close()
			c.h2ClientConn = nil
		}
		c.h2Mu.Unlock()
		return nil, err
	}

	// 检测403错误，将IP加入黑名单
	if resp != nil && resp.StatusCode == http.StatusForbidden {
		c.handle403()
	}

	//if resp != nil {
	//	projlogger.Debug("Http2响应头: %v", resp.Header)
	//}
	return resp, err
}

// markAsUnhealthy 是一个内部方法，用于在发生错误时将连接标记为不健康（去激活）。
// 注意：去激活不等于清理，连接仍然保留在连接池中，等待恢复。
func (c *UTLSConnection) markAsUnhealthy() {
	c.mu.Lock()
	defer c.mu.Unlock()
	atomic.AddInt64(&c.errorCount, 1)
	c.healthy = false
	// 注意：不清理连接，只标记为不健康，等待恢复
}

// handle403 处理403错误：标记连接为不健康，并调用回调函数将IP加入黑名单
func (c *UTLSConnection) handle403() {
	c.mu.Lock()
	c.healthy = false
	on403 := c.on403
	targetIP := c.targetIP
	c.mu.Unlock()

	atomic.AddInt64(&c.errorCount, 1)
	projlogger.Warn("检测到403错误，将IP加入黑名单: %s", targetIP)

	// 调用回调函数将IP加入黑名单（如果设置了回调）
	if on403 != nil {
		on403(targetIP)
	}
}

// establishConnection 负责建立新连接，并使用defer确保资源在失败时被释放。
// on403 是可选的回调函数，当检测到403错误时调用，用于将IP加入黑名单。
func establishConnection(ip, domain string, config *PoolConfig, on403 func(string)) (*UTLSConnection, error) {
	var address string
	if strings.Contains(ip, ":") {
		address = fmt.Sprintf("[%s]:443", ip)
	} else {
		address = fmt.Sprintf("%s:443", ip)
	}

	//projlogger.Debug("开始建立连接: %s -> %s (地址: %s)", domain, ip, address)

	// 创建 Dialer，支持绑定本地 IP 地址
	dialer := &net.Dialer{
		Timeout: config.ConnTimeout,
	}

	// 记录使用的本地 IP 地址（用于日志）
	var localIPStr string

	// 如果配置了本地 IP 池，从池中获取一个本地 IP 并绑定
	if config.LocalIPPool != nil {
		targetIsIPv6 := strings.Contains(ip, ":")
		var localIP net.IP

		// 如果目标是 IPv6，优先获取 IPv6 本地地址（最多重试 5 次，确保获取到 IPv6）
		if targetIsIPv6 {
			maxRetries := 5
			for retry := 0; retry < maxRetries; retry++ {
				candidateIP := config.LocalIPPool.GetIP()
				if candidateIP == nil {
					// 地址池返回 nil，可能是地址池未准备好，短暂等待后重试
					if retry < maxRetries-1 {
						time.Sleep(20 * time.Millisecond)
					}
					continue
				}
				// 检查是否是 IPv6 地址
				if candidateIP.To4() == nil {
					localIP = candidateIP
					break // 找到 IPv6 地址，停止重试
				}
				// 如果返回的是 IPv4，但目标是 IPv6，标记为未使用并重试
				// 注意：这里不调用 ReleaseIP，因为可能影响地址池状态
				// 而是继续重试，让地址池自己管理
				if retry < maxRetries-1 {
					time.Sleep(20 * time.Millisecond) // 短暂等待后重试
				}
			}
		} else {
			// 如果目标是 IPv4，获取本地 IP（可能是 IPv4 或 nil）
			// 注意：如果本地 IP 池只支持 IPv6，这里会返回 nil，使用系统默认
			localIP = config.LocalIPPool.GetIP()
			// 如果返回的是 IPv6，但目标是 IPv4，不能使用，返回 nil
			if localIP != nil && localIP.To4() == nil {
				localIP = nil // IPv6 地址不能用于 IPv4 目标
			}
		}

		if localIP != nil {
			localIsIPv6 := localIP.To4() == nil

			// 只有当本地 IP 类型与目标 IP 类型匹配时才绑定
			if targetIsIPv6 == localIsIPv6 {
				dialer.LocalAddr = &net.TCPAddr{
					IP:   localIP,
					Port: 0, // 0 表示让系统自动分配端口
				}
				localIPStr = localIP.String()
			}
		}
	}

	//projlogger.Debug("尝试TCP连接: %s", address)
	tcpConn, err := dialer.Dial("tcp", address)
	if err != nil {
		projlogger.Debug("TCP连接失败: %s, 错误: %v", address, err)
		return nil, fmt.Errorf("TCP连接失败: %w", err)
	}
	//projlogger.Debug("TCP连接成功: %s", address)

	// 使用defer和标志位确保在任何后续错误发生时都关闭TCP连接
	success := false
	defer func() {
		if !success {
			tcpConn.Close()
		}
	}()

	fingerprint := fpLibrary.RandomProfile()

	uconn := utls.UClient(tcpConn, &utls.Config{
		ServerName:         domain,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
		OmitEmptyPsk:       true,
	}, fingerprint.HelloID)

	//projlogger.Debug("开始TLS握手: %s -> %s", domain, ip)
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnTimeout)
	defer cancel()
	if err := uconn.HandshakeContext(ctx); err != nil {
		projlogger.Debug("TLS握手失败: %s -> %s, 错误: %v", domain, ip, err)
		return nil, fmt.Errorf("TLS握手失败: %w", err)
	}

	//projlogger.Debug("TLS握手成功: %s -> %s", domain, ip)

	// 创建连接对象（用于健康检查）
	conn := &UTLSConnection{
		conn:           tcpConn,
		tlsConn:        uconn,
		targetIP:       ip,
		targetHost:     domain,
		localIP:        localIPStr, // 保存使用的本地 IP 地址
		fingerprint:    fingerprint,
		acceptLanguage: fpLibrary.RandomAcceptLanguage(),
		created:        time.Now(),
		lastUsed:       time.Now(), // 初始化时设置最后使用时间
		healthy:        true,
		on403:          on403, // 设置403回调函数
	}

	// TLS握手成功后进行健康检查（使用 HealthCheckPath，GET 方法）
	healthCheckPath := config.HealthCheckPath
	if healthCheckPath == "" {
		healthCheckPath = "/rt/earth/PlanetoidMetadata" // 默认健康检查路径
	}

	healthCheckURL := "https://" + domain + healthCheckPath
	healthCheckReq, err := http.NewRequest("GET", healthCheckURL, nil)
	if err != nil {
		projlogger.Debug("构建健康检查请求失败: %s -> %s, 错误: %v", domain, ip, err)
		return nil, fmt.Errorf("构建健康检查请求失败: %w", err)
	}
	// 确保请求 URL 的 Host 字段正确（HTTP/2 的 :authority 伪头会从 req.URL.Host 提取）
	healthCheckReq.URL.Host = domain
	// RoundTrip 会自动设置 Host、Accept、User-Agent、Accept-Language 等请求头

	// 执行健康检查
	healthCheckResp, err := conn.RoundTrip(healthCheckReq)
	if err != nil {
		projlogger.Debug("健康检查失败(网络错误): %s -> %s, 错误: %v", domain, ip, err)
		return nil, fmt.Errorf("健康检查失败: %w", err)
	}
	defer healthCheckResp.Body.Close()

	// 处理健康检查结果
	switch healthCheckResp.StatusCode {
	case http.StatusOK:
		// 连接健康，可以继续使用
		//projlogger.Debug("TLS握手和健康检查成功: %s -> %s,返回的数据长度: %d", domain, ip, healthCheckResp.ContentLength)
	case http.StatusForbidden:
		// 403 错误，将 IP 加入黑名单
		if on403 != nil {
			on403(ip)
		}
		projlogger.Debug("健康检查返回403，IP %s 已加入黑名单", ip)
		return nil, fmt.Errorf("健康检查失败，状态码: 403 (IP已加入黑名单)")
	case http.StatusNotFound:
		// 404 错误，记录详细的请求信息用于调试
		// 注意：curl 测试显示同一个 IP 可以返回 200，所以 404 可能是请求头或协议问题
		projlogger.Debug("健康检查返回404: %s -> %s, URL: %s, 请求头: %v, 协议: %s (可能是IP限制或请求头问题，连接仍可用，后续验证阶段会进一步检查)",
			domain, ip, healthCheckURL, healthCheckReq.Header, conn.tlsConn.ConnectionState().NegotiatedProtocol)
	default:
		// 其他非 200 状态码，可能是临时性问题，允许连接继续
		// 因为连接本身是好的（TLS握手成功），只是路径访问有问题
		projlogger.Debug("健康检查返回状态码 %d: %s -> %s (连接仍可用，后续验证阶段会进一步检查)", healthCheckResp.StatusCode, domain, ip)
	}

	// 所有步骤都成功了，设置标志位，防止defer关闭连接
	success = true
	projlogger.Debug("TLS握手和健康检查成功: %s -> %s,返回的数据长度: %d", domain, ip, healthCheckResp.ContentLength)
	return conn, nil
}

// PoolConfig 定义了整个客户端和连接池的配置。
type PoolConfig struct {
	MaxConnsPerHost        int           `mapstructure:"MaxConnsPerHost"`
	PreWarmInterval        time.Duration `mapstructure:"PreWarmInterval"`
	MaxConcurrentPreWarms  int           `mapstructure:"MaxConcurrentPreWarms"`
	ConnTimeout            time.Duration `mapstructure:"ConnTimeout"`
	IdleTimeout            time.Duration `mapstructure:"IdleTimeout"`
	MaxConnLifetime        time.Duration `mapstructure:"MaxConnLifetime"`
	HealthCheckInterval    time.Duration `mapstructure:"HealthCheckInterval"`
	IPBlacklistTimeout     time.Duration `mapstructure:"IPBlacklistTimeout"`
	BlacklistCheckInterval time.Duration `mapstructure:"BlacklistCheckInterval"` // 黑名单恢复检查间隔
	HealthCheckPath        string        `mapstructure:"HealthCheckPath"`        // 健康检查路径（GET方法）
	SessionIdPath          string        `mapstructure:"SessionIdPath"`          // 获取SessionID的路径（POST方法）
	SessionIdBody          []byte        `mapstructure:"SessionIdBody"`          // 获取SessionID的请求体（POST方法使用）

	// LocalIPPool 本地 IP 地址池，用于绑定本地源 IP 地址
	// 如果设置了此字段，建立连接时会从池中获取一个本地 IP 并绑定
	// 支持 IPv4 和 IPv6 地址池
	LocalIPPool localippool.IPPool `mapstructure:"-"`
}
