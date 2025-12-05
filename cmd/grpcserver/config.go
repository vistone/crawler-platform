package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/BurntSushi/toml"

	"crawler-platform/utlsclient"
)

// LoggerConfig 日志配置
// 对应配置文件中的 [logger] 表。
type LoggerConfig struct {
	EnableDebug bool `toml:"enable_debug"`
	EnableInfo  bool `toml:"enable_info"`
	EnableWarn  bool `toml:"enable_warn"`
	EnableError bool `toml:"enable_error"`
}

// TLSConfig TLS 相关配置
// 对应配置文件中的 [tls] 表。
type TLSConfig struct {
	Enable   bool   `toml:"enable"`
	CertsDir string `toml:"certs_dir"`
}

// ServerConfig gRPC 服务器配置
// 对应配置文件中的 [server] 表。
type ServerConfig struct {
	Address   string   `toml:"address"`
	Port      string   `toml:"port"`
	Bootstrap []string `toml:"bootstrap"`
}

// LocalIPPoolConfig 本地 IP 池配置
// 对应配置文件中的 [LocalIPPool] 表。
type LocalIPPoolConfig struct {
	Enable         bool     `toml:"enable"`
	StaticIPv4s    []string `toml:"static_ipv4s"`
	IPv6SubnetCIDR string   `toml:"ipv6_subnet_cidr"`
	TargetIPCount  int      `toml:"target_ip_count"`
}

// DomainMonitorConfig 域名 IP 监控器配置
// 对应配置文件中的 [DomainMonitor] 表。
type DomainMonitorConfig struct {
	Enable                bool   `toml:"enable"`
	DNSServersFile        string `toml:"dns_servers_file"`
	UpdateIntervalMinutes int    `toml:"update_interval_minutes"`
	StorageDir            string `toml:"storage_dir"`
	StorageFormat         string `toml:"storage_format"`
}

// DNSDomainConfig 要监控的域名配置
// 对应配置文件中的 [DNSDomain] 表。
type DNSDomainConfig struct {
	// 注意: 配置文件中键名为 "HostName"，需要与之严格对齐，否则将始终解析为空。
	HostName []string `toml:"HostName"`
}

// IPInfoConfig ipinfo.io 配置
// 对应配置文件中的 [IPInfo] 表。
type IPInfoConfig struct {
	Token string `toml:"Token"`
}

// RockTreeDataConfig RockTree 数据相关配置
// 对应配置文件中的 [RockTreeDataConfig] 表。
type RockTreeDataConfig struct {
	Enable           bool   `toml:"enable"`
	HostName         string `toml:"HostName"`
	HealthCheckPath  string `toml:"healthCheckPath"` // 健康检查路径（GET方法）
	SessionIdPath    string `toml:"SessionIdPath"`   // 获取SessionID的路径（POST方法）
	BulkMetadataPath string `toml:"BulkMetadataPath"`
	NodeDataPath     string `toml:"NodeDataPath"`
	ImageryDataPath  string `toml:"ImageryDataPath"`
}

// GoogleEarthDesktopDataConfig Google Earth Desktop 数据配置
// 对应配置文件中的 [GoogleEarthDesktopDataConfig] 表。
type GoogleEarthDesktopDataConfig struct {
	Enable          bool   `toml:"enable"`
	HostName        string `toml:"HostName"`
	HealthCheckPath string `toml:"healthCheckPath"` // 健康检查路径（GET方法）
	SessionIdPath   string `toml:"SessionIdPath"`   // 获取SessionID的路径（POST方法）
	Q2Path          string `toml:"q2Path"`
	ImageryPath     string `toml:"imageryPath"`
	TerrainPath     string `toml:"terrainPath"`
}

// Config gRPC 服务器整体配置
// 注意: 各字段的 toml 标签需要与 config.toml 中表名精确对应。
type Config struct {
	Logger                 LoggerConfig                 `toml:"logger"`
	TLS                    TLSConfig                    `toml:"tls"`
	Server                 ServerConfig                 `toml:"server"`
	LocalIPPool            LocalIPPoolConfig            `toml:"LocalIPPool"`
	DomainMonitor          DomainMonitorConfig          `toml:"DomainMonitor"`
	DNSDomain              DNSDomainConfig              `toml:"DNSDomain"`
	IPInfo                 IPInfoConfig                 `toml:"IPInfo"`
	RockTreeData           RockTreeDataConfig           `toml:"RockTreeDataConfig"`
	GoogleEarthDesktopData GoogleEarthDesktopDataConfig `toml:"GoogleEarthDesktopDataConfig"`
	UtlsClient             UtlsClientConfig             `toml:"UtlsClient"`
}

// UtlsClientConfig UTLS 客户端连接池配置
// 对应配置文件中的 [UtlsClient] 表。
type UtlsClientConfig struct {
	// 每个主机最大连接数
	MaxConnsPerHost int `toml:"max_conns_per_host"`

	// 连接池预热间隔（字符串格式，如 "5m"）
	PreWarmInterval string `toml:"pre_warm_interval"`

	// 最大并发预热数
	MaxConcurrentPreWarms int `toml:"max_concurrent_pre_warms"`

	// 连接超时时间（字符串格式，如 "10s"）
	ConnTimeout string `toml:"conn_timeout"`

	// 空闲连接超时时间（字符串格式，如 "30m"）
	IdleTimeout string `toml:"idle_timeout"`

	// 连接最大生存时间（字符串格式，如 "1h"）
	MaxConnLifetime string `toml:"max_conn_lifetime"`

	// 健康检查间隔（字符串格式，如 "5m"）
	HealthCheckInterval string `toml:"health_check_interval"`

	// IP黑名单超时时间（字符串格式，如 "15m"）
	IPBlacklistTimeout string `toml:"ip_blacklist_timeout"`

	// 健康检查路径（GET方法）
	HealthCheckPath string `toml:"health_check_path"`

	// 获取SessionID的路径（POST方法）
	SessionIdPath string `toml:"session_id_path"`

	// 获取SessionID的请求体（POST方法使用）
	SessionIdBody []byte `toml:"session_id_body"`
}

// defaultConfig 返回一份合理的默认配置（在没有配置文件时使用）
func defaultConfig() *Config {
	return &Config{
		Logger: LoggerConfig{
			EnableDebug: true,
			EnableInfo:  true,
			EnableWarn:  true,
			EnableError: true,
		},
		TLS: TLSConfig{
			Enable:   false,
			CertsDir: "./certs",
		},
		Server: ServerConfig{
			Address: "0.0.0.0",
			Port:    "50051",
		},
		LocalIPPool: LocalIPPoolConfig{
			Enable:        false,
			TargetIPCount: 0,
		},
		DomainMonitor: DomainMonitorConfig{
			Enable:                false,
			UpdateIntervalMinutes: 10,
			StorageDir:            "./data/domain_ips",
			StorageFormat:         "json",
		},
		IPInfo: IPInfoConfig{
			Token: "",
		},
		RockTreeData: RockTreeDataConfig{
			Enable: false,
		},
		GoogleEarthDesktopData: GoogleEarthDesktopDataConfig{
			Enable: false,
		},
		UtlsClient: UtlsClientConfig{
			MaxConnsPerHost:       10,
			PreWarmInterval:       "5m",
			MaxConcurrentPreWarms: 20,
			ConnTimeout:           "10s",
			IdleTimeout:           "30m",
			MaxConnLifetime:       "1h",
			HealthCheckInterval:   "5m",
			IPBlacklistTimeout:    "15m",
			HealthCheckPath:       "/rt/earth/PlanetoidMetadata",
			SessionIdPath:         "",
			SessionIdBody:         nil,
		},
	}
}

// LoadConfig 加载 gRPC 服务器配置。
// 输入:
//   - path: 配置文件路径；如果为空，则使用项目根目录下的 config.toml / config/config.toml 合并结果；
//     如果两者都不存在，则返回内置默认配置。
//
// 输出:
//   - *Config: 配置实例
//   - error:  加载或解析失败时返回错误
func LoadConfig(path string) (*Config, error) {
	cfg := defaultConfig()

	// 如果显式指定了路径，则只解析该文件
	if path != "" {
		if _, err := tomlDecodeFile(path, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件 %s 失败: %w", path, err)
		}
		return cfg, nil
	}

	return cfg, nil
}

// GetCertsDir 返回 TLS 证书目录。
// 优先使用配置中的 TLS.CertsDir；如果为空则返回默认值 "./certs"。
func (c *Config) GetCertsDir() string {
	if c == nil {
		return "./certs"
	}
	if c.TLS.CertsDir != "" {
		return c.TLS.CertsDir
	}
	return "./certs"
}

// LoadDNSServersFromJSON 从 JSON 文件加载 DNS 服务器列表。
// JSON 格式示例: ["8.8.8.8", "1.1.1.1"]
// 输入:
//   - path: JSON 文件路径
//
// 输出:
//   - []string: DNS 服务器地址列表
//   - error: 加载或解析失败时返回错误
func LoadDNSServersFromJSON(path string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("DNS 服务器列表文件路径不能为空")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 DNS 服务器列表文件失败: %w", err)
	}

	// 1) 首先尝试按简单形式解析: ["8.8.8.8", "1.1.1.1"]
	var servers []string
	if err := json.Unmarshal(data, &servers); err == nil {
		return servers, nil
	}

	// 2) 如果简单形式失败，再尝试兼容当前 dnsservernames.json 的结构:
	// {
	//   "servers": {
	//     "Google-Public-主": "8.8.8.8",
	//     ...
	//   }
	// }
	type namedServers struct {
		Servers map[string]string `json:"servers"`
	}
	var ns namedServers
	if err := json.Unmarshal(data, &ns); err != nil {
		return nil, fmt.Errorf("解析 DNS 服务器列表 JSON 失败: %w", err)
	}

	// 将 map 中的值（IP 地址）展开成去重后的切片
	ipSet := make(map[string]struct{})
	for _, ip := range ns.Servers {
		if ip == "" {
			continue
		}
		ipSet[ip] = struct{}{}
	}

	result := make([]string, 0, len(ipSet))
	for ip := range ipSet {
		result = append(result, ip)
	}

	// 为了日志和调试的可读性，对结果进行排序
	sort.Strings(result)

	return result, nil
}

// ToPoolConfig 将 UtlsClientConfig 转换为 utlsclient.PoolConfig。
func (c *UtlsClientConfig) ToPoolConfig() *utlsclient.PoolConfig {
	parseDuration := func(s string, defaultVal time.Duration) time.Duration {
		if s == "" {
			return defaultVal
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return defaultVal
		}
		return d
	}

	return &utlsclient.PoolConfig{
		MaxConnsPerHost:       c.MaxConnsPerHost,
		PreWarmInterval:       parseDuration(c.PreWarmInterval, 5*time.Minute),
		MaxConcurrentPreWarms: c.MaxConcurrentPreWarms,
		ConnTimeout:           parseDuration(c.ConnTimeout, 10*time.Second),
		IdleTimeout:           parseDuration(c.IdleTimeout, 30*time.Minute),
		MaxConnLifetime:       parseDuration(c.MaxConnLifetime, 1*time.Hour),
		HealthCheckInterval:   parseDuration(c.HealthCheckInterval, 5*time.Minute),
		IPBlacklistTimeout:    parseDuration(c.IPBlacklistTimeout, 15*time.Minute),
		HealthCheckPath:       c.HealthCheckPath,
		SessionIdPath:         c.SessionIdPath,
		SessionIdBody:         c.SessionIdBody,
	}
}

// tomlDecodeFile 是对 toml.DecodeFile 的简单封装，便于单元测试替换。
// 单独抽出是为了保持 Config 模块职责单一：解析逻辑集中在本文件。
func tomlDecodeFile(path string, cfg *Config) (interface{}, error) {
	// 这里直接使用 toml.DecodeFile，但通过局部封装避免在多个地方散落依赖。
	type tomlDecoder interface {
		DecodeFile(fpath string, v interface{}) (interface{}, error)
	}
	// 直接使用全局函数
	return toml.DecodeFile(path, cfg)
}
