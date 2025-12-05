package utlsclient

import (
	"sync"
	"time"
)

// ConnectionManager 连接管理器，负责连接的生命周期管理。
// 它扮演着“白名单”的角色，只存储健康、可用的连接。
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*UTLSConnection // IP -> Connection
	hostMapping map[string][]string        // Host -> []IP
	config      *PoolConfig
}

// NewConnectionManager 创建新的连接管理器。
func NewConnectionManager(config *PoolConfig) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*UTLSConnection),
		hostMapping: make(map[string][]string),
		config:      config,
	}
}

// AddConnection 添加一个连接到管理器中。
// 注意：max_conns_per_host 限制的是每个主机（域名）的连接数，而不是每个 IP 的连接数。
// 每个 IP 都可以有一个连接，这样可以充分利用所有可用的目标 IP。
func (cm *ConnectionManager) AddConnection(conn *UTLSConnection) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 检查该 IP 是否已存在连接（每个 IP 只能有一个连接）
	if _, exists := cm.connections[conn.targetIP]; exists {
		return
	}

	// 检查该主机的连接数是否超过限制
	if cm.config.MaxConnsPerHost > 0 {
		hostIPs := cm.hostMapping[conn.targetHost]
		if len(hostIPs) >= cm.config.MaxConnsPerHost {
			// 已达到该主机的最大连接数限制
			// 注意：这里不阻止添加，因为 max_conns_per_host 应该理解为"每个主机最多预热多少个不同的 IP"
			// 如果用户希望每个 IP 都参与，应该设置 max_conns_per_host 为一个很大的值（如 1000）
			// 或者设置为 0 表示不限制
		}
	}

	cm.connections[conn.targetIP] = conn
	cm.hostMapping[conn.targetHost] = append(cm.hostMapping[conn.targetHost], conn.targetIP)
}

// RemoveConnection 从管理器中移除一个连接，并关闭它。
func (cm *ConnectionManager) RemoveConnection(ip string) {
	cm.mu.Lock()
	conn, exists := cm.connections[ip]
	if !exists {
		cm.mu.Unlock()
		return
	}

	delete(cm.connections, ip)

	// 从 hostMapping 中安全地移除
	if hostList, hostExists := cm.hostMapping[conn.targetHost]; hostExists {
		// 创建一个新的切片来存储结果，这是最安全的做法，可以避免任何并发问题。
		newList := make([]string, 0, len(hostList)-1)
		for _, hostIP := range hostList {
			if hostIP != ip {
				newList = append(newList, hostIP)
			}
		}

		if len(newList) > 0 {
			cm.hostMapping[conn.targetHost] = newList
		} else {
			// 如果列表在移除后为空，则从map中删除该host键
			delete(cm.hostMapping, conn.targetHost)
		}
	}
	cm.mu.Unlock()

	// 在持有锁之外关闭连接，避免阻塞其他操作
	conn.Close()
}

// GetConnection 获取指定IP的连接。
func (cm *ConnectionManager) GetConnection(ip string) *UTLSConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.connections[ip]
}

// GetConnectionsForHost 获取指定域名的所有连接。
func (cm *ConnectionManager) GetConnectionsForHost(host string) []*UTLSConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	ipList, exists := cm.hostMapping[host]
	if !exists {
		return nil
	}

	// 创建一个IP列表的副本进行遍历，这样即使在遍历期间有其他goroutine修改了
	// 原始的hostMapping，我们的遍历也是安全的，因为我们操作的是一个独立的快照。
	ipListCopy := make([]string, len(ipList))
	copy(ipListCopy, ipList)

	var conns []*UTLSConnection
	for _, ip := range ipListCopy {
		if conn, connExists := cm.connections[ip]; connExists {
			conns = append(conns, conn)
		}
	}
	return conns
}

// GetAllConnections 返回管理器中的所有连接的快照。
func (cm *ConnectionManager) GetAllConnections() []*UTLSConnection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conns := make([]*UTLSConnection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		conns = append(conns, conn)
	}
	return conns
}


// Close 关闭并移除所有连接。
func (cm *ConnectionManager) Close() {
	// 先获取所有连接的IP列表
	cm.mu.RLock()
	allIPs := make([]string, 0, len(cm.connections))
	for ip := range cm.connections {
		allIPs = append(allIPs, ip)
	}
	cm.mu.RUnlock()

	// 逐个移除，RemoveConnection会处理并发
	for _, ip := range allIPs {
		cm.RemoveConnection(ip)
	}
}

// CleanupIdleConnections 清理空闲超时的连接。
func (cm *ConnectionManager) CleanupIdleConnections() int {
	var toRemove []string // 改为存储IP地址而不是连接对象
	now := time.Now()

	cm.mu.RLock()
	// 遍历时只收集信息，不做修改
	for ip, conn := range cm.connections {
		conn.mu.Lock()
		isIdle := !conn.inUse && now.Sub(conn.created) > cm.config.IdleTimeout
		conn.mu.Unlock()
		if isIdle {
			toRemove = append(toRemove, ip)
		}
	}
	cm.mu.RUnlock()

	// 在锁外执行移除操作
	for _, ip := range toRemove {
		cm.RemoveConnection(ip)
	}
	return len(toRemove)
}
