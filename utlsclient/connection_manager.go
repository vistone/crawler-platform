package utlsclient

import (
	"sync"
	"time"
)

// ConnectionManager 连接管理器，负责连接的生命周期管理
type ConnectionManager struct {
	mu          sync.RWMutex              // 读写锁
	connections map[string]*UTLSConnection // IP到连接的映射
	hostMapping map[string][]string        // 域名到IP列表的映射
	config      *PoolConfig                // 连接池配置
}

// NewConnectionManager 创建新的连接管理器
func NewConnectionManager(config *PoolConfig) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*UTLSConnection),
		hostMapping: make(map[string][]string),
		config:      config,
	}
}

// AddConnection 添加连接到管理器
func (cm *ConnectionManager) AddConnection(conn *UTLSConnection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.connections[conn.targetIP] = conn

	// 更新域名映射
	if _, exists := cm.hostMapping[conn.targetHost]; !exists {
		cm.hostMapping[conn.targetHost] = []string{}
	}
	cm.hostMapping[conn.targetHost] = append(cm.hostMapping[conn.targetHost], conn.targetIP)

	Debug("连接已添加到管理器: %s -> %s", conn.targetHost, conn.targetIP)
}

// GetConnection 获取指定IP的连接
func (cm *ConnectionManager) GetConnection(ip string) *UTLSConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return cm.connections[ip]
}

// RemoveConnection 移除连接
func (cm *ConnectionManager) RemoveConnection(ip string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if conn, exists := cm.connections[ip]; exists {
		// 从域名映射中移除
		if hostList, exists := cm.hostMapping[conn.targetHost]; exists {
			newList := []string{}
			for _, hostIP := range hostList {
				if hostIP != ip {
					newList = append(newList, hostIP)
				}
			}
			cm.hostMapping[conn.targetHost] = newList
		}

		// 关闭连接
		conn.Close()

		// 从连接映射中移除
		delete(cm.connections, ip)

		Debug("连接已从管理器移除: %s", ip)
	}
}

// GetConnectionsForHost 获取指定域名的所有连接
func (cm *ConnectionManager) GetConnectionsForHost(host string) []*UTLSConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var connections []*UTLSConnection
	if ipList, exists := cm.hostMapping[host]; exists {
		for _, ip := range ipList {
			if conn, exists := cm.connections[ip]; exists {
				connections = append(connections, conn)
			}
		}
	}

	return connections
}

// GetConnectionCount 获取连接总数
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return len(cm.connections)
}

// GetHostMapping 获取域名映射
func (cm *ConnectionManager) GetHostMapping() map[string][]string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 返回副本
	mapping := make(map[string][]string)
	for host, ips := range cm.hostMapping {
		mapping[host] = append([]string{}, ips...)
	}

	return mapping
}

// Close 关闭所有连接并清理资源
func (cm *ConnectionManager) Close() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var errors []error

	// 关闭所有连接
	for ip, conn := range cm.connections {
		if err := conn.Close(); err != nil {
			errors = append(errors, err)
			Error("关闭连接失败 %s: %v", ip, err)
		}
	}

	// 清理映射
	cm.connections = make(map[string]*UTLSConnection)
	cm.hostMapping = make(map[string][]string)

	if len(errors) > 0 {
		return errors[0] // 返回第一个错误
	}

	return nil
}

// CleanupIdleConnections 清理空闲连接
func (cm *ConnectionManager) CleanupIdleConnections() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var cleaned int
	now := time.Now()

	for ip, conn := range cm.connections {
		conn.mu.Lock()
		isIdle := !conn.inUse && now.Sub(conn.lastUsed) > cm.config.IdleTimeout
		conn.mu.Unlock()

		if isIdle {
			// 从域名映射中移除
			if hostList, exists := cm.hostMapping[conn.targetHost]; exists {
				newList := []string{}
				for _, hostIP := range hostList {
					if hostIP != ip {
						newList = append(newList, hostIP)
					}
				}
				cm.hostMapping[conn.targetHost] = newList
			}

			// 关闭连接
			conn.Close()

			// 从连接映射中移除
			delete(cm.connections, ip)
			cleaned++

			Debug("清理空闲连接: %s", ip)
		}
	}

	return cleaned
}

// CleanupExpiredConnections 清理过期连接
func (cm *ConnectionManager) CleanupExpiredConnections(maxLifetime time.Duration) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var cleaned int
	now := time.Now()

	for ip, conn := range cm.connections {
		conn.mu.Lock()
		isExpired := !conn.inUse && now.Sub(conn.created) > maxLifetime
		conn.mu.Unlock()

		if isExpired {
			// 从域名映射中移除
			if hostList, exists := cm.hostMapping[conn.targetHost]; exists {
				newList := []string{}
				for _, hostIP := range hostList {
					if hostIP != ip {
						newList = append(newList, hostIP)
					}
				}
				cm.hostMapping[conn.targetHost] = newList
			}

			// 关闭连接
			conn.Close()

			// 从连接映射中移除
			delete(cm.connections, ip)
			cleaned++

			Debug("清理过期连接: %s", ip)
		}
	}

	return cleaned
}
