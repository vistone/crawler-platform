package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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

// ProtocolType 协议类型枚举
type ProtocolType string

const (
	ProtocolTypeGRPC ProtocolType = "grpc" // gRPC 协议
	ProtocolTypeTUIC ProtocolType = "tuic" // TUIC 协议
	ProtocolTypeBoth ProtocolType = "both" // 两种协议都启用
)

// ProtocolConfig 协议选择配置
// 对应配置文件中的 [protocol] 表。
type ProtocolConfig struct {
	Type ProtocolType `toml:"type"` // 协议类型: "grpc", "tuic", "both"
}

// TUICConfig TUIC 服务器配置
// 对应配置文件中的 [tuic] 表。
type TUICConfig struct {
	Enable      bool   `toml:"enable"`        // 是否启用 TUIC 协议
	Address     string `toml:"address"`       // 监听地址
	Port        string `toml:"port"`          // 监听端口
	Token       string `toml:"token"`         // TUIC 认证令牌
	UUID        string `toml:"uuid"`          // TUIC UUID（必需，用于 sing-box TUIC 服务器）
	Password    string `toml:"password"`      // TUIC 密码（可选）
	Congestion  string `toml:"congestion"`    // 拥塞控制算法（bbr/cubic/reno）
	TLSCertPath string `toml:"tls_cert_path"` // TLS 证书路径（用于 sing-box）
	TLSKeyPath  string `toml:"tls_key_path"`  // TLS 密钥路径（用于 sing-box）
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
	Protocol               ProtocolConfig               `toml:"protocol"` // 协议选择配置
	Server                 ServerConfig                 `toml:"server"`
	TUIC                   TUICConfig                   `toml:"tuic"` // TUIC 配置
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
		Protocol: ProtocolConfig{
			Type: ProtocolTypeGRPC, // 默认使用 gRPC
		},
		Server: ServerConfig{
			Address: "0.0.0.0",
			Port:    "50051",
		},
		TUIC: TUICConfig{
			Enable:      false,
			Address:     "0.0.0.0",
			Port:        "8443",
			Token:       "",
			UUID:        "",
			Password:    "",
			Congestion:  "bbr",
			TLSCertPath: "",
			TLSKeyPath:  "",
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

// SaveTUICConfig 保存 TUIC UUID 和密码到配置文件
// 注意：只更新 uuid 和 password 字段，其他配置保持不变
func SaveTUICConfig(path string, uuid, password string) error {
	if path == "" {
		return fmt.Errorf("配置文件路径不能为空")
	}

	// 读取现有配置文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 将内容转换为字符串
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	var uuidUpdated, passwordUpdated bool
	var inTUICSection bool

	// 遍历所有行，更新 uuid 和 password
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检测是否进入 [tuic] 部分
		if trimmed == "[tuic]" {
			inTUICSection = true
			continue
		}

		// 检测是否离开 [tuic] 部分（遇到下一个 [section]）
		if inTUICSection && strings.HasPrefix(trimmed, "[") && trimmed != "[tuic]" {
			inTUICSection = false
		}

		// 在 [tuic] 部分更新 uuid 和 password
		if inTUICSection {
			// 更新 uuid（匹配 "uuid = " 或 "uuid=" 开头的行）
			if strings.HasPrefix(trimmed, "uuid") && strings.Contains(trimmed, "=") {
				lines[i] = fmt.Sprintf("uuid = \"%s\"", uuid)
				uuidUpdated = true
			}
			// 更新 password（匹配 "password = " 或 "password=" 开头的行）
			if strings.HasPrefix(trimmed, "password") && strings.Contains(trimmed, "=") {
				lines[i] = fmt.Sprintf("password = \"%s\"", password)
				passwordUpdated = true
			}
		}
	}

	// 如果 uuid 或 password 没有找到对应的行，需要在 [tuic] 部分添加
	if inTUICSection && (!uuidUpdated || !passwordUpdated) {
		// 找到 [tuic] 部分的结束位置（下一个 [section] 或文件末尾）
		tuicEndIndex := len(lines)
		for i := 0; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "[tuic]" {
				// 找到 [tuic] 部分，查找结束位置
				for j := i + 1; j < len(lines); j++ {
					trimmedNext := strings.TrimSpace(lines[j])
					if strings.HasPrefix(trimmedNext, "[") && trimmedNext != "[tuic]" {
						tuicEndIndex = j
						break
					}
				}
				// 在 [tuic] 部分末尾添加缺失的字段
				newLines := make([]string, 0, len(lines)+2)
				newLines = append(newLines, lines[:tuicEndIndex]...)
				if !uuidUpdated {
					newLines = append(newLines, fmt.Sprintf("uuid = \"%s\"", uuid))
				}
				if !passwordUpdated {
					newLines = append(newLines, fmt.Sprintf("password = \"%s\"", password))
				}
				newLines = append(newLines, lines[tuicEndIndex:]...)
				lines = newLines
				break
			}
		}
	}

	// 写入更新后的内容
	newContent := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}
