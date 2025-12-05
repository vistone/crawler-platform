package utlsclient

import (
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	projlogger "crawler-platform/logger"
)

// Client 是 utlsclient 包的核心客户端和入口。
type Client struct {
	config      *PoolConfig
	connManager *ConnectionManager
	blacklist   *Blacklist
	poolManager *PoolManager

	stopChan chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.Mutex
}

// NewClient 创建并初始化所有组件。
func NewClient(config *PoolConfig, remotePool RemoteIPPool) (*Client, error) {
	if config == nil || remotePool == nil {
		// 修正：使用 fmt.Errorf 替代 errors.New
		return nil, fmt.Errorf("配置和远程IP池提供者不能为空")
	}

	// 1. 创建黑名单和连接管理器 (ConnectionManager 即为白名单)
	blacklist := NewBlacklist(config.IPBlacklistTimeout)
	connManager := NewConnectionManager(config)

	// 2. 根据配置创建验证器（使用 SessionIdPath 进行 POST 请求获取 sessionid）
	projlogger.Debug("创建验证器: SessionIdPath=%s, SessionIdBody长度=%d", config.SessionIdPath, len(config.SessionIdBody))
	validator := NewConfigurableValidator(
		config.SessionIdPath,
		"POST", // SessionIdPath 始终使用 POST 方法
		config.SessionIdBody,
	)

	// 3. 创建主动式池管理器，并注入所有依赖
	poolManager := NewPoolManager(remotePool, connManager, blacklist, validator, config)

	return &Client{
		config:      config,
		connManager: connManager,
		blacklist:   blacklist,
		poolManager: poolManager,
		stopChan:    make(chan struct{}),
	}, nil
}

// Start 启动所有后台服务。
func (c *Client) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return
	}
	c.running = true
	c.stopChan = make(chan struct{})
	c.poolManager.Start()
	c.wg.Add(1)
	go c.maintenanceLoop()
	projlogger.Info("UTLS 客户端已启动")
}

// Stop 优雅地停止所有后台任务。
func (c *Client) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopChan)
	c.mu.Unlock()

	c.poolManager.Stop()
	c.wg.Wait()
	c.connManager.Close()
	projlogger.Info("UTLS 客户端已停止")
}

// GetConnectionForHost 从“白名单”(ConnectionManager)中获取一个健康的连接。
func (c *Client) GetConnectionForHost(host string) (*UTLSConnection, error) {
	connections := c.connManager.GetConnectionsForHost(host)
	if len(connections) == 0 {
		return nil, fmt.Errorf("没有到主机 %s 的可用连接，请等待PoolManager预热", host)
	}

	// 为了让同一主机的多个预热连接都能参与请求，这里从随机位置开始尝试获取连接。
	n := len(connections)

	start := rand.Intn(n)
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		conn := connections[idx]
		projlogger.Info("获取热连接索引 %d，ip是 %s，已经完成请求数: %d", idx, conn.TargetIP(), conn.requestCount)
		if conn.TryAcquire() {
			return conn, nil
		}
	}

	return nil, fmt.Errorf("主机 %s 的所有连接当前都在使用中", host)
}

// ReleaseConnection 将使用完毕的连接交还给客户端处理。
func (c *Client) ReleaseConnection(conn *UTLSConnection) {
	if conn == nil {
		return
	}

	// 先获取连接锁，安全地读取状态和IP
	conn.mu.Lock()
	isHealthy := conn.healthy
	targetIP := conn.targetIP
	inUse := conn.inUse

	// 如果连接不健康，检查是否还在连接池中
	if !isHealthy {
		conn.mu.Unlock()
		// 检查连接是否还在连接池中（可能已经被健康检查移除了）
		if c.connManager.GetConnection(targetIP) != nil {
			projlogger.Warn("连接 %s 在使用中发生错误，将被销毁", targetIP)
			c.connManager.RemoveConnection(targetIP)
		} else {
			// 连接已经被移除，只记录调试日志
			projlogger.Debug("连接 %s 已被移除（可能在健康检查中被移除）", targetIP)
		}
		return
	}

	// 检查连接是否真的在使用中
	if !inUse {
		// 连接未被使用，可能是重复释放
		conn.mu.Unlock()
		projlogger.Debug("尝试释放未使用的连接 %s", targetIP)
		return
	}

	// 释放连接
	conn.inUse = false
	conn.mu.Unlock()
}

func (c *Client) maintenanceLoop() {
	defer c.wg.Done()
	interval := c.config.HealthCheckInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 不再清理空闲连接，只有系统关闭时才清理
	// 失效的连接会被去激活（标记为不健康），但不会清理，等待恢复

	for {
		select {
		case <-ticker.C:
			projlogger.Debug("开始对白名单连接进行健康检查...")
			c.healthCheck()

			cleanedBlacklist := c.blacklist.Cleanup()
			if cleanedBlacklist > 0 {
				projlogger.Info("从黑名单中移除了 %d 个过期的IP", cleanedBlacklist)
			}
		case <-c.stopChan:
			return
		}
	}
}

// healthCheck 遍历所有白名单中的连接，检查其健康状况。
func (c *Client) healthCheck() {
	allConns := c.connManager.GetAllConnections()
	if len(allConns) == 0 {
		return
	}

	// 使用信号量限制并发数，避免创建过多goroutine
	// 默认最大并发数为10，可以根据配置调整
	maxConcurrency := 10
	if c.config.MaxConcurrentPreWarms > 0 && c.config.MaxConcurrentPreWarms < maxConcurrency {
		maxConcurrency = c.config.MaxConcurrentPreWarms
	}

	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for _, conn := range allConns {
		// 健康检查时，如果连接正在使用中，跳过检查（避免干扰正在使用的连接）
		// 注意：不健康的连接也需要检查，以便恢复它们
		conn.mu.Lock()
		inUse := conn.inUse
		wasHealthy := conn.healthy
		conn.mu.Unlock()

		if inUse {
			// 连接正在使用中，跳过健康检查
			continue
		}

		// 标记为使用中，避免并发问题
		conn.mu.Lock()
		if conn.inUse {
			conn.mu.Unlock()
			continue // 在检查期间被其他goroutine获取了
		}
		conn.inUse = true
		conn.mu.Unlock()

		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		go func(conn *UTLSConnection, wasHealthy bool) {
			defer func() {
				<-semaphore // 释放信号量
				wg.Done()
				// 释放连接
				conn.mu.Lock()
				conn.inUse = false
				conn.mu.Unlock()
			}()

			// 安全地读取targetHost
			conn.mu.Lock()
			targetHost := conn.targetHost
			targetIP := conn.targetIP
			conn.mu.Unlock()

			// 记录是否检查不健康的连接
			if !wasHealthy {
				projlogger.Debug("健康检查：尝试恢复不健康的连接 %s", targetIP)
			}

			// 使用配置的健康检查路径，如果没有配置则使用默认路径
			healthCheckPath := c.config.HealthCheckPath
			if healthCheckPath == "" {
				healthCheckPath = "/rt/earth/PlanetoidMetadata" // 默认健康检查路径
			}

			// 使用 GET 方法进行健康检查（因为需要验证返回 200）
			req, err := http.NewRequest("GET", "https://"+targetHost+healthCheckPath, nil)
			if err != nil {
				projlogger.Warn("健康检查构建请求失败: %v, 连接 %s", err, targetIP)
				return
			}

			resp, err := conn.RoundTrip(req)
			if err != nil {
				// 网络错误：去激活连接（标记为不健康），但不清理，等待恢复
				projlogger.Debug("健康检查失败(网络错误)，连接 %s 去激活，错误详情: %v（连接保持，等待恢复）", targetIP, err)
				conn.markAsUnhealthy()
				return
			}
			defer resp.Body.Close()

			// 只有返回 200 才是健康的，其他状态码的处理
			switch resp.StatusCode {
			case http.StatusOK:
				// 连接健康，如果之前被标记为不健康，现在恢复健康（激活）
				conn.mu.Lock()
				if !conn.healthy {
					conn.healthy = true
					projlogger.Info("✅ 连接 %s 已激活（恢复健康）", targetIP)
				}
				conn.mu.Unlock()
				projlogger.Debug("健康检查通过，连接 %s 状态正常", targetIP)
			case http.StatusForbidden:
				// 只有403错误才加入黑名单并移除连接
				projlogger.Warn("健康检查发现403，将IP %s 从白名单降级到黑名单", targetIP)
				c.blacklist.Add(targetIP)
				conn.markAsUnhealthy()
				// 立即移除不健康的连接（只有403才移除）
				c.connManager.RemoveConnection(targetIP)
			default:
				// 其他非 200 状态码：去激活连接（标记为不健康），但不清理，等待恢复
				projlogger.Debug("健康检查发现状态码 %d（非200），连接 %s 去激活（连接保持，等待恢复）", resp.StatusCode, targetIP)
				conn.markAsUnhealthy()
			}
		}(conn, wasHealthy)
	}

	wg.Wait()
}
