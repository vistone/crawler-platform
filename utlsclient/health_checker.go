package utlsclient

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// HealthChecker 健康检查器，负责连接健康状态检查
type HealthChecker struct {
	connManager *ConnectionManager
	config      *PoolConfig
}

// NewHealthChecker 创建新的健康检查器
func NewHealthChecker(connManager *ConnectionManager, config *PoolConfig) *HealthChecker {
	return &HealthChecker{
		connManager: connManager,
		config:      config,
	}
}

// CheckConnection 检查单个连接的健康状态
func (hc *HealthChecker) CheckConnection(conn *UTLSConnection) bool {
	if conn == nil {
		return false
	}

	// 检查连接是否超时
	conn.mu.Lock()
	isHealthy := conn.healthy
	lastUsed := conn.lastUsed
	errorCount := conn.errorCount
	inUse := conn.inUse
	conn.mu.Unlock()

	// 如果连接正在使用中，跳过健康检查（避免干扰正在进行的请求）
	if inUse {
		return isHealthy
	}

	// 如果错误次数过多，标记为不健康（错误次数超过10次）
	const maxErrorCount = 10
	if errorCount > maxErrorCount {
		Debug("连接错误次数过多，标记为不健康: %s (错误: %d)", conn.targetIP, errorCount)
		conn.mu.Lock()
		conn.healthy = false
		conn.mu.Unlock()
		return false
	}

	// 如果连接空闲时间过长，进行健康检查
	if time.Since(lastUsed) > hc.config.HealthCheckInterval {
		if isHealthy {
			// 再次检查连接是否正在使用（双重检查，避免竞态条件）
			conn.mu.Lock()
			if conn.inUse {
				conn.mu.Unlock()
				return isHealthy // 如果正在使用，跳过健康检查
			}
			conn.mu.Unlock()

			// 执行简单的健康检查
			if err := hc.performHealthCheck(conn); err != nil {
				Debug("健康检查失败: %s -> %v", conn.targetIP, err)
				conn.mu.Lock()
				conn.healthy = false
				conn.mu.Unlock()
				return false
			}
		}
	}

	return isHealthy
}

// performHealthCheck 执行实际的健康检查
func (hc *HealthChecker) performHealthCheck(conn *UTLSConnection) error {
	// 检测协商的协议
	conn.mu.Lock()
	negotiatedProto := conn.tlsConn.ConnectionState().NegotiatedProtocol
	conn.mu.Unlock()

	// 对于 HTTP/2 连接，只检查连接状态，不发送请求（避免关闭连接）
	if negotiatedProto == "h2" {
		conn.mu.Lock()
		defer conn.mu.Unlock()

		// 检查连接是否健康
		if !conn.healthy {
			return fmt.Errorf("连接不健康")
		}

		// 检查是否超时
		if time.Since(conn.created) > hc.config.MaxLifetime {
			conn.healthy = false
			return fmt.Errorf("连接已超时")
		}

		// 更新最后检查时间
		conn.lastChecked = time.Now()
		conn.lastUsed = time.Now()
		Debug("HTTP/2 连接健康检查通过: %s", conn.targetIP)
		return nil
	}

	// 对于 HTTP/1.1 连接，发送简单的 HEAD 请求
	req := &http.Request{
		Method: "HEAD",
		URL:    &url.URL{Scheme: HTTPSProtocol, Host: conn.targetHost, Path: "/"},
		Header: make(http.Header),
		Host:   conn.targetHost,
	}

	// 使用UTLSClient进行健康检查
	client := NewUTLSClient(conn)
	client.SetTimeout(5 * time.Second) // 健康检查使用较短的超时时间

	_, err := client.Do(req)
	if err != nil {
		return err
	}

	// 更新连接的最后使用时间
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.mu.Unlock()

	Debug("健康检查通过: %s", conn.targetIP)
	return nil
}

// CheckAllConnections 检查所有连接的健康状态
func (hc *HealthChecker) CheckAllConnections() {
	// 获取所有连接的副本
	connections := make(map[string]*UTLSConnection)
	hostMapping := hc.connManager.GetHostMapping()

	for _, ips := range hostMapping {
		for _, ip := range ips {
			if conn := hc.connManager.GetConnection(ip); conn != nil {
				connections[ip] = conn
			}
		}
	}

	// 检查每个连接
	for ip, conn := range connections {
		if !hc.CheckConnection(conn) {
			Debug("连接不健康，将被移除: %s", ip)
			hc.connManager.RemoveConnection(ip)
		}
	}
}

// GetHealthyConnections 获取所有健康的连接
func (hc *HealthChecker) GetHealthyConnections() []*UTLSConnection {
	var healthyConnections []*UTLSConnection

	hostMapping := hc.connManager.GetHostMapping()
	for _, ips := range hostMapping {
		for _, ip := range ips {
			if conn := hc.connManager.GetConnection(ip); conn != nil {
				if hc.CheckConnection(conn) {
					healthyConnections = append(healthyConnections, conn)
				}
			}
		}
	}

	return healthyConnections
}

// GetUnhealthyConnections 获取所有不健康的连接
func (hc *HealthChecker) GetUnhealthyConnections() []*UTLSConnection {
	var unhealthyConnections []*UTLSConnection

	hostMapping := hc.connManager.GetHostMapping()
	for _, ips := range hostMapping {
		for _, ip := range ips {
			if conn := hc.connManager.GetConnection(ip); conn != nil {
				conn.mu.Lock()
				isHealthy := conn.healthy
				conn.mu.Unlock()

				if !isHealthy {
					unhealthyConnections = append(unhealthyConnections, conn)
				}
			}
		}
	}

	return unhealthyConnections
}

// CleanupUnhealthyConnections 清理不健康的连接
func (hc *HealthChecker) CleanupUnhealthyConnections() int {
	unhealthyConns := hc.GetUnhealthyConnections()

	for _, conn := range unhealthyConns {
		hc.connManager.RemoveConnection(conn.targetIP)
		Debug("清理不健康连接: %s", conn.targetIP)
	}

	return len(unhealthyConns)
}
