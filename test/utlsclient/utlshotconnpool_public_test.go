package utlsclient_test

import (
	"testing"

	"crawler-platform/utlsclient"
)

func TestPoolBasics_PublicAPI(t *testing.T) {
	cfg := utlsclient.DefaultPoolConfig()
	pool := utlsclient.NewUTLSHotConnPool(cfg)
	defer pool.Close()

	// Stats 初始应为空
	stats := pool.GetStats()
	if stats.TotalConnections != 0 {
		t.Errorf("expected TotalConnections 0, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 0 {
		t.Errorf("expected ActiveConnections 0, got %d", stats.ActiveConnections)
	}
	if stats.IdleConnections != 0 {
		t.Errorf("expected IdleConnections 0, got %d", stats.IdleConnections)
	}

	// 空池不健康
	if pool.IsHealthy() {
		t.Error("empty pool should be unhealthy")
	}

	// 公开方法不应返回 nil 切片
	if pool.GetWhitelist() == nil {
		t.Error("GetWhitelist should not return nil")
	}
	if pool.GetBlacklist() == nil {
		t.Error("GetBlacklist should not return nil")
	}

	// 连接数查询
	if c := pool.GetConnectionCount("example.com"); c != 0 {
		t.Errorf("expected 0 connections for example.com, got %d", c)
	}

	// 强制清理不应 panic，且仍为空
	pool.ForceCleanup()
	stats = pool.GetStats()
	if stats.TotalConnections != 0 {
		t.Errorf("expected TotalConnections 0 after cleanup, got %d", stats.TotalConnections)
	}
}

func TestPoolUpdateAndInfo_PublicAPI(t *testing.T) {
	cfg := utlsclient.DefaultPoolConfig()
	pool := utlsclient.NewUTLSHotConnPool(cfg)
	defer pool.Close()

	// UpdateConfig 应可更新配置
	newCfg := utlsclient.DefaultPoolConfig()
	newCfg.MaxConnections = cfg.MaxConnections + 1
	pool.UpdateConfig(newCfg)

	// 不存在的连接 info 应为 nil
	if info := pool.GetConnectionInfo("1.2.3.4"); info != nil {
		t.Error("GetConnectionInfo should be nil for unknown ip")
	}
}

func TestConfigLoad_PublicAPI(t *testing.T) {
	// 不存在文件应返回错误
	if _, err := utlsclient.LoadPoolConfigFromFile("nonexistent.toml"); err == nil {
		t.Error("expected error for non-existent file")
	}
	if _, _, _, err := utlsclient.LoadConfigFromTOML("nonexistent.toml"); err == nil {
		t.Error("expected error for non-existent file")
	}
}


