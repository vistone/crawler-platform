package utlsclient_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"crawler-platform/utlsclient"
)

func TestNewConnectionManager(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	if cm == nil {
		t.Fatal("NewConnectionManager should not return nil")
	}
}

func TestConnectionManagerAddConnection(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	cm.AddConnection(conn)

	// 验证连接已添加
	retrieved := cm.GetConnection("1.2.3.4")
	if retrieved == nil {
		t.Error("Connection should be retrievable after adding")
	}
	if utlsclient.GetTestConnectionTargetIP(retrieved) != "1.2.3.4" {
		t.Errorf("Expected IP 1.2.3.4, got %s", utlsclient.GetTestConnectionTargetIP(retrieved))
	}

	// 验证域名映射
	connections := cm.GetConnectionsForHost("example.com")
	if len(connections) == 0 {
		t.Error("Should have connections for the host")
	}
}

func TestConnectionManagerGetConnection(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	// 测试不存在的连接
	conn := cm.GetConnection("1.2.3.4")
	if conn != nil {
		t.Error("GetConnection should return nil for non-existent connection")
	}

	// 添加连接后应该能找到
	testConn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	cm.AddConnection(testConn)

	conn = cm.GetConnection("1.2.3.4")
	if conn == nil {
		t.Error("GetConnection should return connection after adding")
	}
}

func TestConnectionManagerRemoveConnection(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	cm.AddConnection(conn)

	// 移除连接
	cm.RemoveConnection("1.2.3.4")

	// 验证连接已移除
	retrieved := cm.GetConnection("1.2.3.4")
	if retrieved != nil {
		t.Error("Connection should be removed")
	}

	// 验证域名映射也已更新
	connections := cm.GetConnectionsForHost("example.com")
	for _, c := range connections {
		if utlsclient.GetTestConnectionTargetIP(c) == "1.2.3.4" {
			t.Error("Connection should be removed from host mapping")
		}
	}
}

func TestConnectionManagerGetConnectionsForHost(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	// 测试不存在的host
	connections := cm.GetConnectionsForHost("nonexistent.com")
	if len(connections) != 0 {
		t.Error("Should return empty slice for non-existent host")
	}

	// 添加多个连接到同一个host
	conn1 := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	conn2 := utlsclient.NewTestConnection("1.2.3.5", "example.com")
	cm.AddConnection(conn1)
	cm.AddConnection(conn2)

	connections = cm.GetConnectionsForHost("example.com")
	if len(connections) != 2 {
		t.Errorf("Expected 2 connections, got %d", len(connections))
	}
}

func TestConnectionManagerGetHostMapping(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	conn1 := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	conn2 := utlsclient.NewTestConnection("1.2.3.5", "example.com")
	conn3 := utlsclient.NewTestConnection("2.3.4.5", "test.com")

	cm.AddConnection(conn1)
	cm.AddConnection(conn2)
	cm.AddConnection(conn3)

	mapping := cm.GetHostMapping()

	if len(mapping) != 2 {
		t.Errorf("Expected 2 hosts in mapping, got %d", len(mapping))
	}

	if len(mapping["example.com"]) != 2 {
		t.Errorf("Expected 2 IPs for example.com, got %d", len(mapping["example.com"]))
	}

	if len(mapping["test.com"]) != 1 {
		t.Errorf("Expected 1 IP for test.com, got %d", len(mapping["test.com"]))
	}
}

func TestConnectionManagerCleanupIdleConnections(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	config.IdleTimeout = 100 * time.Millisecond
	cm := utlsclient.NewConnectionManager(config)

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	utlsclient.SetTestConnectionLastUsed(conn, time.Now().Add(-200*time.Millisecond)) // 超过空闲时间
	cm.AddConnection(conn)

	cleaned := cm.CleanupIdleConnections()
	if cleaned != 1 {
		t.Errorf("Expected 1 connection cleaned, got %d", cleaned)
	}

	// 验证连接已移除
	retrieved := cm.GetConnection("1.2.3.4")
	if retrieved != nil {
		t.Error("Idle connection should be cleaned up")
	}
}

func TestConnectionManagerCleanupExpiredConnections(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	config.MaxLifetime = 100 * time.Millisecond
	cm := utlsclient.NewConnectionManager(config)

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	utlsclient.SetTestConnectionCreated(conn, time.Now().Add(-200*time.Millisecond)) // 超过生命周期
	utlsclient.SetTestConnectionInUse(conn, false)
	cm.AddConnection(conn)

	cleaned := cm.CleanupExpiredConnections(config.MaxLifetime)
	if cleaned != 1 {
		t.Errorf("Expected 1 connection cleaned, got %d", cleaned)
	}

	// 验证连接已移除
	retrieved := cm.GetConnection("1.2.3.4")
	if retrieved != nil {
		t.Error("Expired connection should be cleaned up")
	}
}

func TestConnectionManagerConcurrentAccess(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	var wg sync.WaitGroup
	numGoroutines := 10
	numConnections := 10

	// 并发添加连接
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numConnections; j++ {
				ip := fmt.Sprintf("1.2.3.%d", id*numConnections+j)
				conn := utlsclient.NewTestConnection(ip, "example.com")
				cm.AddConnection(conn)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有连接都已添加
	connections := cm.GetConnectionsForHost("example.com")
	if len(connections) != numGoroutines*numConnections {
		t.Errorf("Expected %d connections, got %d", numGoroutines*numConnections, len(connections))
	}
}

func TestConnectionManagerClose(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	cm := utlsclient.NewConnectionManager(config)

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	cm.AddConnection(conn)

	// 关闭应该成功
	err := cm.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	// 验证连接已清理
	connections := cm.GetConnectionsForHost("example.com")
	if len(connections) != 0 {
		t.Error("Connections should be cleaned up after Close")
	}
}
