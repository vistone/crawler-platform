package utlsclient

import (
	"sync"
	"sync/atomic"
	"time"
)

// ConnectionMetrics 连接池指标收集器
// 用于监控连接的生命周期、健康状态和性能指标
type ConnectionMetrics struct {
	// 连接计数
	totalConnections     int64 // 总连接数（累计）
	activeConnections    int64 // 当前活跃连接数
	healthyConnections   int64 // 当前健康连接数
	unhealthyConnections int64 // 当前不健康连接数

	// 请求统计
	totalRequests   int64 // 总请求数
	failedRequests  int64 // 失败请求数
	forbiddenErrors int64 // 403错误数

	// 性能指标
	requestDurationMs int64 // 请求耗时（毫秒，用于计算平均耗时）

	// 连接生命周期
	connectionsCreated   int64 // 创建的连接数
	connectionsClosed    int64 // 关闭的连接数
	connectionsRecovered int64 // 恢复的连接数

	// IP池指标
	ipv6AddressesUsed int64 // 使用的IPv6地址数
	ipv4AddressesUsed int64 // 使用的IPv4地址数

	// 黑名单指标
	blacklistedIPs   int64 // 被拉黑的IP数
	blacklistCleaned int64 // 从黑名单移除的IP数

	// 时间戳
	lastUpdated time.Time

	mu sync.RWMutex
}

// NewConnectionMetrics 创建新的指标收集器
func NewConnectionMetrics() *ConnectionMetrics {
	return &ConnectionMetrics{
		lastUpdated: time.Now(),
	}
}

// RecordConnectionCreated 记录连接创建
func (m *ConnectionMetrics) RecordConnectionCreated() {
	atomic.AddInt64(&m.totalConnections, 1)
	atomic.AddInt64(&m.connectionsCreated, 1)
	atomic.AddInt64(&m.activeConnections, 1)
	m.updateTimestamp()
}

// RecordConnectionClosed 记录连接关闭
func (m *ConnectionMetrics) RecordConnectionClosed() {
	atomic.AddInt64(&m.connectionsClosed, 1)
	atomic.AddInt64(&m.activeConnections, -1)
	m.updateTimestamp()
}

// RecordConnectionHealthy 记录连接变为健康
func (m *ConnectionMetrics) RecordConnectionHealthy(wasUnhealthy bool) {
	if wasUnhealthy {
		atomic.AddInt64(&m.unhealthyConnections, -1)
		atomic.AddInt64(&m.healthyConnections, 1)
		atomic.AddInt64(&m.connectionsRecovered, 1)
	} else {
		atomic.AddInt64(&m.healthyConnections, 1)
	}
	m.updateTimestamp()
}

// RecordConnectionUnhealthy 记录连接变为不健康
func (m *ConnectionMetrics) RecordConnectionUnhealthy() {
	atomic.AddInt64(&m.healthyConnections, -1)
	atomic.AddInt64(&m.unhealthyConnections, 1)
	m.updateTimestamp()
}

// RecordRequest 记录请求
func (m *ConnectionMetrics) RecordRequest(duration time.Duration, success bool) {
	atomic.AddInt64(&m.totalRequests, 1)
	atomic.AddInt64(&m.requestDurationMs, duration.Milliseconds())
	if !success {
		atomic.AddInt64(&m.failedRequests, 1)
	}
	m.updateTimestamp()
}

// RecordForbiddenError 记录403错误
func (m *ConnectionMetrics) RecordForbiddenError() {
	atomic.AddInt64(&m.forbiddenErrors, 1)
	m.updateTimestamp()
}

// RecordIPUsed 记录IP使用
func (m *ConnectionMetrics) RecordIPUsed(isIPv6 bool) {
	if isIPv6 {
		atomic.AddInt64(&m.ipv6AddressesUsed, 1)
	} else {
		atomic.AddInt64(&m.ipv4AddressesUsed, 1)
	}
	m.updateTimestamp()
}

// RecordBlacklisted 记录IP被拉黑
func (m *ConnectionMetrics) RecordBlacklisted() {
	atomic.AddInt64(&m.blacklistedIPs, 1)
	m.updateTimestamp()
}

// RecordBlacklistCleaned 记录黑名单清理
func (m *ConnectionMetrics) RecordBlacklistCleaned(count int) {
	atomic.AddInt64(&m.blacklistCleaned, int64(count))
	atomic.AddInt64(&m.blacklistedIPs, -int64(count))
	m.updateTimestamp()
}

// GetSnapshot 获取当前指标的只读快照
func (m *ConnectionMetrics) GetSnapshot() MetricsSnapshot {
	return MetricsSnapshot{
		TotalConnections:     atomic.LoadInt64(&m.totalConnections),
		ActiveConnections:    atomic.LoadInt64(&m.activeConnections),
		HealthyConnections:   atomic.LoadInt64(&m.healthyConnections),
		UnhealthyConnections: atomic.LoadInt64(&m.unhealthyConnections),
		TotalRequests:        atomic.LoadInt64(&m.totalRequests),
		FailedRequests:       atomic.LoadInt64(&m.failedRequests),
		ForbiddenErrors:      atomic.LoadInt64(&m.forbiddenErrors),
		ConnectionsCreated:   atomic.LoadInt64(&m.connectionsCreated),
		ConnectionsClosed:    atomic.LoadInt64(&m.connectionsClosed),
		ConnectionsRecovered: atomic.LoadInt64(&m.connectionsRecovered),
		IPv6AddressesUsed:    atomic.LoadInt64(&m.ipv6AddressesUsed),
		IPv4AddressesUsed:    atomic.LoadInt64(&m.ipv4AddressesUsed),
		BlacklistedIPs:       atomic.LoadInt64(&m.blacklistedIPs),
		BlacklistCleaned:     atomic.LoadInt64(&m.blacklistCleaned),
		AvgRequestDurationMs: m.getAvgRequestDurationMs(),
		LastUpdated:          m.getLastUpdated(),
	}
}

// getAvgRequestDurationMs 计算平均请求耗时
func (m *ConnectionMetrics) getAvgRequestDurationMs() int64 {
	total := atomic.LoadInt64(&m.totalRequests)
	if total == 0 {
		return 0
	}
	return atomic.LoadInt64(&m.requestDurationMs) / total
}

// updateTimestamp 更新时间戳
func (m *ConnectionMetrics) updateTimestamp() {
	m.mu.Lock()
	m.lastUpdated = time.Now()
	m.mu.Unlock()
}

// getLastUpdated 获取最后更新时间
func (m *ConnectionMetrics) getLastUpdated() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastUpdated
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	TotalConnections     int64     `json:"total_connections"`
	ActiveConnections    int64     `json:"active_connections"`
	HealthyConnections   int64     `json:"healthy_connections"`
	UnhealthyConnections int64     `json:"unhealthy_connections"`
	TotalRequests        int64     `json:"total_requests"`
	FailedRequests       int64     `json:"failed_requests"`
	ForbiddenErrors      int64     `json:"forbidden_errors"`
	ConnectionsCreated   int64     `json:"connections_created"`
	ConnectionsClosed    int64     `json:"connections_closed"`
	ConnectionsRecovered int64     `json:"connections_recovered"`
	IPv6AddressesUsed    int64     `json:"ipv6_addresses_used"`
	IPv4AddressesUsed    int64     `json:"ipv4_addresses_used"`
	BlacklistedIPs       int64     `json:"blacklisted_ips"`
	BlacklistCleaned     int64     `json:"blacklist_cleaned"`
	AvgRequestDurationMs int64     `json:"avg_request_duration_ms"`
	LastUpdated          time.Time `json:"last_updated"`
}

// SuccessRate 计算请求成功率
func (s MetricsSnapshot) SuccessRate() float64 {
	if s.TotalRequests == 0 {
		return 100.0
	}
	return float64(s.TotalRequests-s.FailedRequests) / float64(s.TotalRequests) * 100
}

// HealthRate 计算连接健康率
func (s MetricsSnapshot) HealthRate() float64 {
	total := s.HealthyConnections + s.UnhealthyConnections
	if total == 0 {
		return 100.0
	}
	return float64(s.HealthyConnections) / float64(total) * 100
}
