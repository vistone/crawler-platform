package main

import (
	"time"
)

// UTLSProxyServerConfig represents the utlsProxyServer section
type UTLSProxyServerConfig struct {
	Port int `toml:"port"`
}

// HotConnPoolConfig represents the HotConnPool section
type HotConnPoolConfig struct {
	// 最大连接数
	MaxConnections int `toml:"max_connections"`
	// 每个主机最大连接数
	MaxConnsPerHost int `toml:"max_conns_per_host"`
	// 最大空闲连接数
	MaxIdleConns int `toml:"max_idle_conns"`
	// 连接超时时间 (秒)
	ConnTimeout int `toml:"conn_timeout"`
	// 空闲超时时间 (秒)
	IdleTimeout int `toml:"idle_timeout"`
	// 连接最大生命周期 (秒)
	MaxLifetime int `toml:"max_lifetime"`
	// 测试请求超时时间 (秒)
	TestTimeout int `toml:"test_timeout"`
	// 健康检查间隔 (秒)
	HealthCheckInterval int `toml:"health_check_interval"`
	// 清理间隔 (秒)
	CleanupInterval int `toml:"cleanup_interval"`
	// 黑名单检查间隔 (秒)
	BlacklistCheckInterval int `toml:"blacklist_check_interval"`
	// DNS更新间隔 (秒)
	DNSUpdateInterval int `toml:"dns_update_interval"`
	// 最大重试次数
	MaxRetries int `toml:"max_retries"`
}

// TUICNodeConfig represents a single TUIC node configuration
type TUICNodeConfig struct {
	Name              string `toml:"name"`
	Listen            string `toml:"listen"`
	Server            string `toml:"server"`
	Port              int    `toml:"port"`
	Certificate       string `toml:"certificate"`
	PrivateKey        string `toml:"private_key"`
	UUID              string `toml:"uuid"`
	Password          string `toml:"password"`
	CongestionControl string `toml:"congestion_control"`
	UDPRelayMode      string `toml:"udp_relay_mode"`
	ZeroRTTHandshake  bool   `toml:"zero_rtt_handshake"`
	Disabled          bool   `toml:"disabled"`
}

// TUICConfig represents the TUIC configuration section
type TUICConfig struct {
	Enable bool             `toml:"enable"`
	Nodes  []TUICNodeConfig `toml:"nodes"`
}

// Config represents the entire configuration structure
type Config struct {
	UTLSProxyServer UTLSProxyServerConfig `toml:"utlsProxyServer"`
	HotConnPool     HotConnPoolConfig     `toml:"HotConnPool"`
	TUIC            TUICConfig            `toml:"tuic"`
}

// GetConnTimeout returns connection timeout as time.Duration
func (h *HotConnPoolConfig) GetConnTimeout() time.Duration {
	return time.Duration(h.ConnTimeout) * time.Second
}

// GetIdleTimeout returns idle timeout as time.Duration
func (h *HotConnPoolConfig) GetIdleTimeout() time.Duration {
	return time.Duration(h.IdleTimeout) * time.Second
}

// GetMaxLifetime returns max lifetime as time.Duration
func (h *HotConnPoolConfig) GetMaxLifetime() time.Duration {
	return time.Duration(h.MaxLifetime) * time.Second
}

// GetTestTimeout returns test timeout as time.Duration
func (h *HotConnPoolConfig) GetTestTimeout() time.Duration {
	return time.Duration(h.TestTimeout) * time.Second
}

// GetHealthCheckInterval returns health check interval as time.Duration
func (h *HotConnPoolConfig) GetHealthCheckInterval() time.Duration {
	return time.Duration(h.HealthCheckInterval) * time.Second
}

// GetCleanupInterval returns cleanup interval as time.Duration
func (h *HotConnPoolConfig) GetCleanupInterval() time.Duration {
	return time.Duration(h.CleanupInterval) * time.Second
}

// GetBlacklistCheckInterval returns blacklist check interval as time.Duration
func (h *HotConnPoolConfig) GetBlacklistCheckInterval() time.Duration {
	return time.Duration(h.BlacklistCheckInterval) * time.Second
}

// GetDNSUpdateInterval returns DNS update interval as time.Duration
func (h *HotConnPoolConfig) GetDNSUpdateInterval() time.Duration {
	return time.Duration(h.DNSUpdateInterval) * time.Second
}