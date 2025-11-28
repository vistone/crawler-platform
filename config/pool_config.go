package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// PoolConfigFile 完整配置文件结构（用于TOML解析）
type PoolConfigFile struct {
	Pool      PoolConfigSection `toml:"pool"`
	Whitelist WhitelistSection  `toml:"whitelist"`
	Blacklist BlacklistSection  `toml:"blacklist"`
}

// PoolConfigSection 连接池配置段
type PoolConfigSection struct {
	MaxConnections         int `toml:"max_connections"`
	MaxConnsPerHost        int `toml:"max_conns_per_host"`
	MaxIdleConns           int `toml:"max_idle_conns"`
	ConnTimeout            int `toml:"conn_timeout"`
	IdleTimeout            int `toml:"idle_timeout"`
	MaxLifetime            int `toml:"max_lifetime"`
	TestTimeout            int `toml:"test_timeout"`
	HealthCheckInterval    int `toml:"health_check_interval"`
	CleanupInterval        int `toml:"cleanup_interval"`
	BlacklistCheckInterval int `toml:"blacklist_check_interval"`
	DNSUpdateInterval      int `toml:"dns_update_interval"`
	MaxRetries             int `toml:"max_retries"`
}

// WhitelistSection 白名单配置段
type WhitelistSection struct {
	IPs []string `toml:"ips"`
}

// BlacklistSection 黑名单配置段
type BlacklistSection struct {
	IPs []string `toml:"ips"`
}

// PoolConfigData 连接池配置数据（用于在config包中处理，避免循环导入）
type PoolConfigData struct {
	MaxConnections         int
	MaxConnsPerHost        int
	MaxIdleConns           int
	ConnTimeout            int
	IdleTimeout            int
	MaxLifetime            int
	TestTimeout            int
	HealthCheckInterval    int
	CleanupInterval        int
	BlacklistCheckInterval int
	DNSUpdateInterval      int
	MaxRetries             int
}

// PoolConfigResult 配置加载结果
type PoolConfigResult struct {
	Config    *PoolConfigData
	Whitelist []string
	Blacklist []string
}

// LoadPoolConfigFromTOML 从TOML文件加载连接池配置
// 输入: configPath - 配置文件路径
// 输出: *PoolConfigResult - 配置结果, error - 错误信息
func LoadPoolConfigFromTOML(configPath string) (*PoolConfigResult, error) {
	var configFile PoolConfigFile

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	// 解析TOML
	if _, err := toml.Decode(string(data), &configFile); err != nil {
		return nil, fmt.Errorf("解析TOML配置失败: %w", err)
	}

	// 转换为PoolConfigData
	poolConfig := &PoolConfigData{
		MaxConnections:         configFile.Pool.MaxConnections,
		MaxConnsPerHost:        configFile.Pool.MaxConnsPerHost,
		MaxIdleConns:           configFile.Pool.MaxIdleConns,
		ConnTimeout:            configFile.Pool.ConnTimeout,
		IdleTimeout:            configFile.Pool.IdleTimeout,
		MaxLifetime:            configFile.Pool.MaxLifetime,
		TestTimeout:            configFile.Pool.TestTimeout,
		HealthCheckInterval:    configFile.Pool.HealthCheckInterval,
		CleanupInterval:        configFile.Pool.CleanupInterval,
		BlacklistCheckInterval: configFile.Pool.BlacklistCheckInterval,
		DNSUpdateInterval:      configFile.Pool.DNSUpdateInterval,
		MaxRetries:             configFile.Pool.MaxRetries,
	}

	// 验证配置
	if err := validatePoolConfigData(poolConfig); err != nil {
		return nil, err
	}

	return &PoolConfigResult{
		Config:    poolConfig,
		Whitelist: configFile.Whitelist.IPs,
		Blacklist: configFile.Blacklist.IPs,
	}, nil
}

// LoadMergedPoolConfig 合并读取根目录 config.toml 与 config/config.toml，并转换为配置结果
// 输出: *PoolConfigResult - 配置结果, error - 错误信息
func LoadMergedPoolConfig() (*PoolConfigResult, error) {
	var configFile PoolConfigFile

	// 使用LoadMergedInto合并配置
	if err := LoadMergedInto(&configFile); err != nil {
		return nil, err
	}

	// 转换为PoolConfigData
	poolConfig := &PoolConfigData{
		MaxConnections:         configFile.Pool.MaxConnections,
		MaxConnsPerHost:        configFile.Pool.MaxConnsPerHost,
		MaxIdleConns:           configFile.Pool.MaxIdleConns,
		ConnTimeout:            configFile.Pool.ConnTimeout,
		IdleTimeout:            configFile.Pool.IdleTimeout,
		MaxLifetime:            configFile.Pool.MaxLifetime,
		TestTimeout:            configFile.Pool.TestTimeout,
		HealthCheckInterval:    configFile.Pool.HealthCheckInterval,
		CleanupInterval:        configFile.Pool.CleanupInterval,
		BlacklistCheckInterval: configFile.Pool.BlacklistCheckInterval,
		DNSUpdateInterval:      configFile.Pool.DNSUpdateInterval,
		MaxRetries:             configFile.Pool.MaxRetries,
	}

	// 验证配置
	if err := validatePoolConfigData(poolConfig); err != nil {
		return nil, err
	}

	return &PoolConfigResult{
		Config:    poolConfig,
		Whitelist: configFile.Whitelist.IPs,
		Blacklist: configFile.Blacklist.IPs,
	}, nil
}

// validatePoolConfigData 验证连接池配置数据
// 输入: config - 连接池配置数据
// 输出: error - 错误信息
func validatePoolConfigData(config *PoolConfigData) error {
	if config.MaxConnections <= 0 {
		return fmt.Errorf("max_connections 必须大于0")
	}
	if config.MaxConnsPerHost <= 0 {
		return fmt.Errorf("max_conns_per_host 必须大于0")
	}
	if config.ConnTimeout <= 0 {
		return fmt.Errorf("conn_timeout 必须大于0")
	}
	return nil
}
