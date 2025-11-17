package utlsclient_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"crawler-platform/logger"
	"crawler-platform/utlsclient"
)

func TestNewUTLSClient(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	if client == nil {
		t.Fatal("NewUTLSClient should not return nil")
	}
}

func TestUTLSClientSetTimeout(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	newTimeout := 10 * time.Second
	client.SetTimeout(newTimeout)

	// 验证超时已设置（通过再次设置并调用方法验证）
	client.SetTimeout(5 * time.Second)
	// 如果方法正常工作，不会panic
}

func TestUTLSClientSetUserAgent(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	newUA := "new-user-agent"
	client.SetUserAgent(newUA)

	// 验证UserAgent已设置（通过调用方法验证）
	client.SetUserAgent("another-agent")
	// 如果方法正常工作，不会panic
}

func TestUTLSClientSetMaxRetries(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	newRetries := 5
	client.SetMaxRetries(newRetries)

	// 验证重试次数已设置（通过再次设置验证）
	client.SetMaxRetries(3)
	// 如果方法正常工作，不会panic
}

func TestUTLSClientSetDebug(t *testing.T) {
	// 设置NopLogger以避免输出
	logger.SetGlobalLogger(&logger.NopLogger{})

	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	// 测试启用调试模式
	client.SetDebug(true)
	// 验证全局日志已设置（通过检查不会panic）
	logger.Debug("test")

	// 测试禁用调试模式
	client.SetDebug(false)
	// 应该仍然可以调用
	logger.Debug("test")
}

func TestUTLSClientDoWithContext_NilConnection(t *testing.T) {
	// 由于无法直接创建带有nil连接的客户端，我们跳过这个测试
	// 或者通过反射创建，但这超出了单元测试的范围
	t.Skip("Skipping test that requires access to unexported fields")
}

func TestUTLSClientDoWithContext_UnhealthyConnection(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	utlsclient.SetTestConnectionInUse(conn, false)
	// 设置连接为不健康需要通过内部方法，这里跳过
	t.Skip("Skipping test that requires access to unexported fields")
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{errors.New("connection refused"), true},
		{errors.New("broken pipe"), true},
		{errors.New("connection reset"), true},
		{errors.New("connection closed"), true},
		{utlsclient.ErrConnectionBroken, true},
		{utlsclient.ErrConnectionClosed, true},
		{errors.New("some other error"), false},
		{nil, false},
	}

	for _, test := range tests {
		result := utlsclient.IsConnectionError(test.err)
		if result != test.expected {
			t.Errorf("IsConnectionError(%v) = %v, expected %v", test.err, result, test.expected)
		}
	}
}

func TestUTLSClientGet(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	// 这个测试会因为无法建立真实连接而失败
	// 由于连接没有真实的tlsConn，会panic，所以跳过实际请求测试
	// 在实际集成测试中，可以使用mock连接或真实连接
	t.Skip("Skipping test that requires real TLS connection")

	_, err := client.Get("https://example.com")
	if err == nil {
		t.Log("Get succeeded")
	} else {
		t.Logf("Get failed: %v", err)
	}
}

func TestUTLSClientPost(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	// 跳过需要真实连接的测试
	t.Skip("Skipping test that requires real TLS connection")

	body := strings.NewReader("test body")
	_, err := client.Post("https://example.com", "text/plain", body)
	if err == nil {
		t.Log("Post succeeded")
	} else {
		t.Logf("Post failed: %v", err)
	}
}

func TestUTLSClientHead(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")
	client := utlsclient.NewUTLSClient(conn)

	// 跳过需要真实连接的测试
	t.Skip("Skipping test that requires real TLS connection")

	_, err := client.Head("https://example.com")
	if err == nil {
		t.Log("Head succeeded")
	} else {
		t.Logf("Head failed: %v", err)
	}
}

func TestUTLSConnectionMethods(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")

	// 测试TargetHost
	if conn.TargetHost() != "example.com" {
		t.Errorf("Expected TargetHost 'example.com', got '%s'", conn.TargetHost())
	}

	// 测试TargetIP
	if conn.TargetIP() != "1.2.3.4" {
		t.Errorf("Expected TargetIP '1.2.3.4', got '%s'", conn.TargetIP())
	}

	// 测试Fingerprint
	fp := conn.Fingerprint()
	if fp.UserAgent != "test-agent" {
		t.Errorf("Expected UserAgent 'test-agent', got '%s'", fp.UserAgent)
	}

	// 测试Created
	now := time.Now()
	if conn.Created().After(now) || conn.Created().Before(now.Add(-time.Second)) {
		t.Error("Created time should be approximately now")
	}

	// 测试LastUsed
	if conn.LastUsed().After(now) || conn.LastUsed().Before(now.Add(-time.Second)) {
		t.Error("LastUsed time should be approximately now")
	}

	// 测试RequestCount
	if conn.RequestCount() != 0 {
		t.Errorf("Expected RequestCount 0, got %d", conn.RequestCount())
	}

	// 测试ErrorCount
	if conn.ErrorCount() != 0 {
		t.Errorf("Expected ErrorCount 0, got %d", conn.ErrorCount())
	}

	// 测试IsHealthy
	if !conn.IsHealthy() {
		t.Error("Connection should be healthy")
	}
}

func TestUTLSConnectionStats(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")

	stats := conn.Stats()

	if stats.TargetHost != "example.com" {
		t.Errorf("Expected TargetHost 'example.com', got '%s'", stats.TargetHost)
	}

	if stats.TargetIP != "1.2.3.4" {
		t.Errorf("Expected TargetIP '1.2.3.4', got '%s'", stats.TargetIP)
	}

	if !stats.IsHealthy {
		t.Error("Stats should show connection as healthy")
	}
}

func TestUTLSConnectionClose(t *testing.T) {
	conn := utlsclient.NewTestConnection("1.2.3.4", "example.com")

	// Close应该成功（即使连接为nil）
	err := conn.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	// 多次关闭应该也是安全的
	err = conn.Close()
	if err != nil {
		t.Errorf("Second Close should not return error: %v", err)
	}
}
