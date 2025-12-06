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
		return nil, fmt.Errorf("%w: 配置和远程IP池提供者不能为空", ErrInvalidConfig)
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

	// 为所有现有连接设置快速健康检查回调
	c.connManager.SetQuickHealthCheckCallback(c.quickHealthCheck)

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
		// 如果没有健康连接，尝试获取所有连接（包括不健康的），并尝试激活它们
		allConns := c.connManager.GetAllConnectionsForHost(host)
		if len(allConns) == 0 {
			return nil, fmt.Errorf("%w: 没有到主机 %s 的可用连接，请等待PoolManager预热", ErrNoAvailableConnection, host)
		}

		// 所有连接都不健康，尝试快速激活不健康的连接
		projlogger.Debug("主机 %s 的所有连接都不健康，尝试快速激活连接...", host)
		activatedCount := 0
		maxActivate := 5 // 最多同时激活5个连接
		for _, conn := range allConns {
			conn.mu.Lock()
			inUse := conn.inUse
			wasHealthy := conn.healthy
			conn.mu.Unlock()

			if !inUse && !wasHealthy && activatedCount < maxActivate {
				// 触发快速健康检查，尝试激活这个连接
				if conn.onQuickHealthCheck != nil {
					// 异步触发快速健康检查
					go conn.onQuickHealthCheck(conn)
					activatedCount++
				}
			}
		}

		if activatedCount > 0 {
			// 等待更长时间，让快速健康检查有机会完成
			// 使用指数退避：第一次等待200ms，如果还没恢复，再等待
			waitTime := 200 * time.Millisecond
			for retry := 0; retry < 3; retry++ {
				time.Sleep(waitTime)
				// 再次尝试获取健康连接
				connections := c.connManager.GetConnectionsForHost(host)
				if len(connections) > 0 {
					// 有连接恢复了，尝试获取
					for _, conn := range connections {
						if conn.TryAcquire() {
							projlogger.Debug("快速激活成功，获取到连接 %s (等待了 %v)", conn.TargetIP(), time.Duration(retry+1)*waitTime)
							return conn, nil
						}
					}
				}
				// 如果还没恢复，增加等待时间
				waitTime *= 2
			}
		}

		// 如果所有连接都不健康且无法激活，返回错误
		// 但这种情况应该很少发生，因为连接应该能够恢复
		return nil, fmt.Errorf("%w: 没有到主机 %s 的可用连接，所有连接都不健康，正在尝试激活，请稍后重试", ErrNoAvailableConnection, host)
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

	return nil, fmt.Errorf("%w: 主机 %s 的所有连接当前都在使用中", ErrConnectionInUse, host)
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

	// 如果连接不健康，触发快速健康检查恢复连接（不立即移除）
	// 只有403错误才会在健康检查中移除连接
	if !isHealthy {
		conn.mu.Unlock()
		// 检查连接是否还在连接池中
		if c.connManager.GetConnection(targetIP) != nil {
			// 连接不健康，触发快速健康检查恢复（不立即移除）
			// 只有快速健康检查确认是403时，才会移除连接
			projlogger.Debug("连接 %s 不健康，触发快速健康检查恢复（连接保持，等待恢复）", targetIP)
			if conn.onQuickHealthCheck != nil {
				go conn.onQuickHealthCheck(conn)
			}
		} else {
			// 连接已经被移除（可能是健康检查中确认了403），只记录调试日志
			projlogger.Debug("连接 %s 已被移除（可能在健康检查中被移除）", targetIP)
		}
		// 即使连接不健康，也要释放 inUse 标志，允许其他操作
		conn.mu.Lock()
		conn.inUse = false
		conn.mu.Unlock()
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

			// 确保所有连接都有快速健康检查回调（包括新添加的连接）
			c.connManager.SetQuickHealthCheckCallback(c.quickHealthCheck)

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
				// 网络错误：不标记为不健康，只记录日志
				// 连接断开是正常的（可能是服务器空闲超时），下次使用时会自动恢复
				// 只有403才标记为不健康
				projlogger.Debug("健康检查失败(网络错误)，连接 %s 暂时不可用，错误详情: %v（连接保持健康状态，下次使用时会自动恢复）", targetIP, err)
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
				//projlogger.Debug("健康检查通过，连接 %s 状态正常", targetIP)
			case http.StatusForbidden:
				// 只有403错误才加入黑名单并移除连接
				projlogger.Warn("健康检查发现403，将IP %s 从白名单降级到黑名单", targetIP)
				c.blacklist.Add(targetIP)
				conn.markAsUnhealthy()
				// 立即移除不健康的连接（只有403才移除）
				c.connManager.RemoveConnection(targetIP)
			default:
				// 其他非 200 状态码：不标记为不健康，只记录日志
				// 可能是临时性问题（如404、500等），连接保持健康状态，下次使用时会自动恢复
				projlogger.Debug("健康检查发现状态码 %d（非200），连接 %s 暂时不可用（连接保持健康状态，下次使用时会自动恢复）", resp.StatusCode, targetIP)
			}
		}(conn, wasHealthy)
	}

	wg.Wait()
}

// quickHealthCheck 快速健康检查单个连接（用于立即恢复不活跃的连接）
func (c *Client) quickHealthCheck(conn *UTLSConnection) {
	// 如果连接正在使用中，跳过检查
	conn.mu.Lock()
	inUse := conn.inUse
	targetIP := conn.targetIP
	targetHost := conn.targetHost
	conn.mu.Unlock()

	if inUse {
		// 连接正在使用中，跳过快速健康检查
		return
	}

	// 标记为使用中，避免并发问题
	conn.mu.Lock()
	if conn.inUse {
		conn.mu.Unlock()
		return // 在检查期间被其他goroutine获取了
	}
	conn.inUse = true
	conn.mu.Unlock()

	// 异步执行快速健康检查
	go func() {
		defer func() {
			conn.mu.Lock()
			conn.inUse = false
			conn.mu.Unlock()
		}()

		// 使用配置的健康检查路径
		healthCheckPath := c.config.HealthCheckPath
		if healthCheckPath == "" {
			healthCheckPath = "/rt/earth/PlanetoidMetadata" // 默认健康检查路径
		}

		// 使用 GET 方法进行健康检查
		req, err := http.NewRequest("GET", "https://"+targetHost+healthCheckPath, nil)
		if err != nil {
			projlogger.Debug("快速健康检查构建请求失败: %v, 连接 %s", err, targetIP)
			return
		}

		resp, err := conn.RoundTrip(req)
		if err != nil {
			// 网络错误：不标记为不健康，只记录日志
			// 连接断开是正常的（可能是服务器空闲超时），下次使用时会自动恢复
			// 只有403才标记为不健康
			projlogger.Debug("快速健康检查失败(网络错误)，连接 %s 暂时不可用，错误详情: %v（连接保持健康状态，下次使用时会自动恢复）", targetIP, err)
			return
		}
		defer resp.Body.Close()

		// 只有返回 200 才恢复健康
		switch resp.StatusCode {
		case http.StatusOK:
			// 连接恢复健康（激活）
			conn.mu.Lock()
			if !conn.healthy {
				conn.healthy = true
				projlogger.Info("✅ 快速健康检查：连接 %s 已激活（恢复健康）", targetIP)
			}
			conn.mu.Unlock()
		case http.StatusForbidden:
			// 403错误，加入黑名单并移除连接
			projlogger.Warn("快速健康检查发现403，将IP %s 从白名单降级到黑名单", targetIP)
			c.blacklist.Add(targetIP)
			conn.markAsUnhealthy()
			c.connManager.RemoveConnection(targetIP)
		default:
			// 其他状态码：不标记为不健康，只记录日志
			// 可能是临时性问题（如404、500等），连接保持健康状态，下次使用时会自动恢复
			projlogger.Debug("快速健康检查发现状态码 %d（非200），连接 %s 暂时不可用（连接保持健康状态，下次使用时会自动恢复）", resp.StatusCode, targetIP)
		}
	}()
}
