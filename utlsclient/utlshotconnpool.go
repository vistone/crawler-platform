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

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"

	"crawler-platform/localippool"
	projlogger "crawler-platform/logger"
)

// DefaultHealthCheckPath 是默认的健康检查路径
// 用于在配置未指定健康检查路径时的回退值
const DefaultHealthCheckPath = "/rt/earth/PlanetoidMetadata"

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

	created    time.Time
	lastUsed   time.Time // 最后使用时间，用于空闲超时检查
	healthy    bool
	inUse      bool
	recovering bool // 快速健康检查正在进行中，防止重复触发

	requestCount int64
	errorCount   int64

	// on403 回调函数，当检测到403错误时调用，用于将IP加入黑名单
	on403 func(ip string)
	// onQuickHealthCheck 回调函数，当检测到连接不活跃时调用，用于触发快速健康检查
	onQuickHealthCheck func(conn *UTLSConnection)

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

// RequestCount 返回此连接已完成的请求总数。
func (c *UTLSConnection) RequestCount() int64 {
	return atomic.LoadInt64(&c.requestCount)
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
		return nil, fmt.Errorf("%w", ErrConnectionUnhealthy)
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
	if req.Header.Get("Origin") == "" {
		req.Header.Set("Origin", "https://"+targetHost)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", "https://"+targetHost+"/")
	}
	if req.Header.Get("Sec-Fetch-Dest") == "" {
		req.Header.Set("Sec-Fetch-Dest", "empty")
	}
	if req.Header.Get("Sec-Fetch-Mode") == "" {
		req.Header.Set("Sec-Fetch-Mode", "cors")
	}
	if req.Header.Get("Sec-Fetch-Site") == "" {
		req.Header.Set("Sec-Fetch-Site", "same-site")
	}
	if req.Header.Get("Sec-Purpose") == "" {
		req.Header.Set("Sec-Purpose", "prefetch")
	}
	if req.Header.Get("Priority") == "" {
		req.Header.Set("Priority", "u=6")
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
		return nil, fmt.Errorf("%w", ErrConnectionUnhealthy)
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
			// 创建 HTTP/2 连接失败，检查是否是连接已关闭的错误
			errStr := err.Error()
			if strings.Contains(errStr, "use of closed network connection") ||
				strings.Contains(errStr, "connection closed") ||
				strings.Contains(errStr, "broken pipe") ||
				strings.Contains(errStr, "EOF") {
				// 底层连接已关闭，触发快速健康检查恢复连接（不标记为不健康）
				// 连接保持健康状态，只有403才标记为不健康
				// 检查是否已经在恢复中，避免重复触发导致死循环
				c.mu.Lock()
				shouldTrigger := !c.recovering && c.onQuickHealthCheck != nil
				if shouldTrigger {
					c.recovering = true
				}
				c.mu.Unlock()
				if shouldTrigger {
					projlogger.Debug("检测到底层连接已关闭，触发快速恢复连接 %s（连接保持健康状态）", c.targetIP)
					// 触发快速健康检查（异步，不阻塞当前请求）
					go c.onQuickHealthCheck(c)
				}
			}
			return nil, fmt.Errorf("创建 HTTP/2 连接失败: %w", err)
		}
		c.h2ClientConn = clientConn
		projlogger.Debug("HTTP/2 连接已创建: %s", c.targetIP)
	}
	h2Conn := c.h2ClientConn
	c.h2Mu.Unlock()

	resp, err := h2Conn.RoundTrip(req)
	if err != nil {
		// 请求失败，关闭 HTTP/2 连接，避免 readLoop goroutine 泄漏
		c.h2Mu.Lock()
		if c.h2ClientConn == h2Conn {
			c.h2ClientConn.Close()
			c.h2ClientConn = nil
		}
		c.h2Mu.Unlock()

		// 检查是否是连接关闭的错误，如果是则触发快速恢复
		errStr := err.Error()
		if strings.Contains(errStr, "use of closed network connection") ||
			strings.Contains(errStr, "connection closed") ||
			strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "EOF") {
			// 底层连接已关闭，触发快速健康检查恢复连接（不标记为不健康）
			// 连接保持健康状态，只有403才标记为不健康
			// 检查是否已经在恢复中，避免重复触发导致死循环
			c.mu.Lock()
			shouldTrigger := !c.recovering && c.onQuickHealthCheck != nil
			if shouldTrigger {
				c.recovering = true
			}
			c.mu.Unlock()
			if shouldTrigger {
				projlogger.Debug("HTTP/2 请求失败，检测到底层连接已关闭，触发快速恢复连接 %s（连接保持健康状态）", c.targetIP)
				// 触发快速健康检查（异步，不阻塞当前请求）
				go c.onQuickHealthCheck(c)
			}
		}
		// 注意：只有403才标记为不健康，其他错误（包括连接关闭）不标记
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
	// 设置 TCP keep-alive 以保持长连接
	dialer := &net.Dialer{
		Timeout:   config.ConnTimeout,
		KeepAlive: 30 * time.Second, // 每30秒发送一次keep-alive探测包
	}

	// 记录使用的本地 IP 地址（用于日志）
	var localIPStr string
	var localIP net.IP

	// 如果配置了本地 IP 池，从池中获取一个本地 IP 并绑定
	// 这是反检测的关键：使用动态 IPv6 地址池，防止固定 IP 外泄
	if config.LocalIPPool != nil {
		targetIsIPv6 := strings.Contains(ip, ":")

		// 优先使用 IPv6 地址池（如果可用）
		// 这样可以防止固定 IPv4 地址外泄，提高匿名性
		if targetIsIPv6 {
			// 目标是 IPv6，从地址池获取 IPv6 地址
			candidateIP := config.LocalIPPool.GetIP()
			if candidateIP != nil && candidateIP.To4() == nil {
				// 获取到 IPv6 地址，直接使用
				localIP = candidateIP
				projlogger.Debug("使用 IPv6 地址池地址: %s", localIP.String())
			} else if candidateIP != nil {
				// 如果返回的是 IPv4，但目标是 IPv6，标记为未使用
				config.LocalIPPool.MarkIPUnused(candidateIP)
				projlogger.Debug("地址池返回 IPv4，但目标为 IPv6，跳过使用")
			}
			// 如果 candidateIP == nil，说明IP池处于隧道模式或无法提供IPv6地址
			// 此时直接返回错误，跳过IPv6目标，避免无效的网络不可达错误
			if localIP == nil {
				return nil, fmt.Errorf("跳过IPv6目标：本地IP池无法提供IPv6地址，目标 %s", ip)
			}
		} else {
			// 目标是 IPv4，优先尝试获取 IPv6 地址进行 NAT64 转换（如果支持）
			// 或者获取 IPv4 地址
			candidateIP := config.LocalIPPool.GetIP()
			if candidateIP != nil {
				if candidateIP.To4() == nil {
					// 获取到 IPv6 地址，但目标是 IPv4
					// 标记为未使用，因为无法直接用于 IPv4 连接
					config.LocalIPPool.MarkIPUnused(candidateIP)
					projlogger.Debug("地址池返回 IPv6，但目标为 IPv4，跳过使用")
				} else {
					// 获取到 IPv4 地址
					localIP = candidateIP
					projlogger.Debug("使用 IPv4 地址池地址: %s", localIP.String())
				}
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

	// 安全检查：如果没有获取到本地 IP 地址，且配置了 IP 池
	// 记录警告日志，提醒可能存在固定 IP 外泄风险
	if localIPStr == "" && config.LocalIPPool != nil {
		projlogger.Warn("未能从 IP 池获取地址，可能使用系统默认地址，存在固定 IP 外泄风险")
	}

	//projlogger.Debug("尝试TCP连接: %s", address)
	tcpConn, err := dialer.Dial("tcp", address)
	if err != nil {
		// 如果连接失败且使用了本地IPv6地址，可能是地址不可用导致的
		// 记录更详细的错误信息以便调试
		if localIPStr != "" {
			// 检查是否是地址绑定相关的错误
			errStr := err.Error()
			if strings.Contains(errStr, "cannot assign requested address") ||
				strings.Contains(errStr, "address not available") {
				// 明确的地址不可用错误，地址可能已被清理
				projlogger.Debug("TCP连接失败（本地IPv6地址 %s 不可用，可能已被清理）: %s, 错误: %v", localIPStr, address, err)
				// 如果配置了本地IP池，标记该地址为未使用，让系统重新创建
				if config.LocalIPPool != nil {
					localIP := net.ParseIP(localIPStr)
					if localIP != nil {
						config.LocalIPPool.MarkIPUnused(localIP)
					}
				}
			} else if strings.Contains(errStr, "bind: invalid argument") {
				// 链路本地地址(fe80::/10)或其他无效地址导致的绑定错误
				// 这种地址不能用于外部连接，标记为未使用并记录警告
				projlogger.Warn("TCP连接失败（本地IPv6地址 %s 无效，可能是链路本地地址或其他不可路由地址）: %s, 错误: %v", localIPStr, address, err)
				if config.LocalIPPool != nil {
					localIP := net.ParseIP(localIPStr)
					if localIP != nil {
						config.LocalIPPool.MarkIPUnused(localIP)
					}
				}
				// 尝试不绑定本地IP重新连接（传统模式回退）
				projlogger.Info("尝试使用传统模式（不绑定本地IP）重新连接: %s", address)
				dialer.LocalAddr = nil
				tcpConn, err = dialer.Dial("tcp", address)
				if err == nil {
					projlogger.Info("传统模式连接成功: %s", address)
					localIPStr = "" // 清除本地IP标记，表示使用系统默认地址
				} else {
					projlogger.Debug("传统模式连接也失败: %s, 错误: %v", address, err)
				}
			} else if strings.Contains(errStr, "network is unreachable") {
				// IPv6网络不可达，可能是系统没有IPv6路由
				// 尝试不绑定本地IP重新连接，让系统自动选择合适的网络
				projlogger.Warn("TCP连接失败（IPv6网络不可达）: %s, 错误: %v", address, err)
				projlogger.Info("尝试使用传统模式（不绑定本地IP）重新连接: %s", address)
				dialer.LocalAddr = nil
				tcpConn, err = dialer.Dial("tcp", address)
				if err == nil {
					projlogger.Info("传统模式连接成功: %s", address)
					localIPStr = "" // 清除本地IP标记，表示使用系统默认地址
				} else {
					projlogger.Debug("传统模式连接也失败: %s, 错误: %v", address, err)
				}
			} else if strings.Contains(errStr, "too many open files") {
				// "too many open files" 可能是地址不可用导致的累积失败
				projlogger.Debug("TCP连接失败（使用本地IPv6地址 %s）: %s, 错误: %v (可能是地址不可用或文件描述符耗尽)", localIPStr, address, err)
			} else {
				projlogger.Debug("TCP连接失败（使用本地IPv6地址 %s）: %s, 错误: %v", localIPStr, address, err)
			}
		} else {
			projlogger.Debug("TCP连接失败: %s, 错误: %v", address, err)
		}
		if err != nil {
			return nil, fmt.Errorf("TCP连接失败: %w", err)
		}
	}
	//projlogger.Debug("TCP连接成功: %s", address)

	// 设置TCP keep-alive以保持长连接
	// 这样可以防止连接在空闲时被服务器或中间设备（如NAT、防火墙）关闭
	var tcpConnPtr *net.TCPConn
	if tcpConn, ok := tcpConn.(*net.TCPConn); ok {
		tcpConnPtr = tcpConn
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second) // 每30秒发送一次keep-alive探测包
		// 注意：SetKeepAlivePeriod 在某些系统上可能不支持，如果失败也不影响连接建立
	}

	// 使用defer和标志位确保在任何后续错误发生时都关闭TCP连接
	success := false
	defer func() {
		if !success {
			tcpConn.Close()
		}
	}()

	// 获取浏览器指纹（包含 TLS 指纹和 User-Agent）
	fingerprint := fpLibrary.RandomProfile()

	// 应用 TCP 指纹伪装 - 确保与浏览器指纹来自同一平台
	// 这是反检测的关键：TLS 指纹、User-Agent、TCP 指纹必须同步
	if tcpConnPtr != nil {
		err := ApplyTCPFingerprintByProfile(tcpConnPtr, fingerprint)
		if err != nil {
			projlogger.Debug("应用 TCP 指纹失败 (平台: %s): %v", fingerprint.Platform, err)
			// TCP 指纹设置失败不阻止连接建立，继续尝试
		}
	}

	// 验证指纹一致性
	if consistent, msg := GetFingerprintConsistency(fingerprint); consistent {
		projlogger.Debug("指纹一致性检查通过: %s", msg)
	} else {
		projlogger.Warn("指纹一致性检查警告: %s", msg)
	}

	// 验证 IP 池使用情况，防止固定 IP 外泄
	if valid, msg := ValidateIPPoolUsage(localIPStr, config.LocalIPPool != nil); valid {
		projlogger.Debug("IP 池验证: %s", msg)
	} else {
		projlogger.Warn("IP 池验证: %s", msg)
	}

	// 记录完整的反检测配置信息
	LogFingerprintAndIP(fingerprint, localIPStr, ip)

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
		healthCheckPath = DefaultHealthCheckPath // 使用默认健康检查路径
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
		return nil, fmt.Errorf("%w: 健康检查失败，状态码: 403", ErrIPBlockedBy403)
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
