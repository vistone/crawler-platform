package utlsclient_test

import (
	"testing"
	"time"

	"crawler-platform/utlsclient"
)

func TestConstants(t *testing.T) {
	// 测试端口常量
	if utlsclient.DefaultHTTPSPort != 443 {
		t.Errorf("Expected DefaultHTTPSPort to be 443, got %d", utlsclient.DefaultHTTPSPort)
	}

	if utlsclient.DefaultHTTPPort != 80 {
		t.Errorf("Expected DefaultHTTPPort to be 80, got %d", utlsclient.DefaultHTTPPort)
	}

	// 测试HTTP状态码常量
	if utlsclient.StatusOK != 200 {
		t.Errorf("Expected StatusOK to be 200, got %d", utlsclient.StatusOK)
	}

	if utlsclient.StatusNoContent != 204 {
		t.Errorf("Expected StatusNoContent to be 204, got %d", utlsclient.StatusNoContent)
	}

	if utlsclient.StatusForbidden != 403 {
		t.Errorf("Expected StatusForbidden to be 403, got %d", utlsclient.StatusForbidden)
	}

	// 测试协议常量
	if utlsclient.HTTPSProtocol != "https" {
		t.Errorf("Expected HTTPSProtocol to be 'https', got %s", utlsclient.HTTPSProtocol)
	}

	if utlsclient.HTTPProtocol != "http" {
		t.Errorf("Expected HTTPProtocol to be 'http', got %s", utlsclient.HTTPProtocol)
	}

	// 测试重试延迟常量
	if utlsclient.DefaultRetryDelay != 1*time.Second {
		t.Errorf("Expected DefaultRetryDelay to be 1s, got %v", utlsclient.DefaultRetryDelay)
	}

	// 测试成功率阈值
	if utlsclient.MinSuccessRate != 0.5 {
		t.Errorf("Expected MinSuccessRate to be 0.5, got %f", utlsclient.MinSuccessRate)
	}
}

func TestConnectionErrorKeywords(t *testing.T) {
	if len(utlsclient.ConnectionErrorKeywords) == 0 {
		t.Error("ConnectionErrorKeywords should not be empty")
	}

	// 验证包含预期的关键词
	expectedKeywords := []string{
		"connection",
		"broken pipe",
		"connection reset",
		"connection refused",
		"connection closed",
	}

	for _, keyword := range expectedKeywords {
		found := false
		for _, k := range utlsclient.ConnectionErrorKeywords {
			if k == keyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ConnectionErrorKeywords should contain '%s'", keyword)
		}
	}
}

func TestErrorDefinitions(t *testing.T) {
	// 测试错误定义不为空
	if utlsclient.ErrConnectionClosed == nil {
		t.Error("ErrConnectionClosed should not be nil")
	}

	if utlsclient.ErrConnectionBroken == nil {
		t.Error("ErrConnectionBroken should not be nil")
	}

	if utlsclient.ErrIPBlocked == nil {
		t.Error("ErrIPBlocked should not be nil")
	}

	if utlsclient.ErrConnectionUnhealthy == nil {
		t.Error("ErrConnectionUnhealthy should not be nil")
	}

	if utlsclient.ErrConnectionTimeout == nil {
		t.Error("ErrConnectionTimeout should not be nil")
	}

	if utlsclient.ErrInvalidURL == nil {
		t.Error("ErrInvalidURL should not be nil")
	}

	if utlsclient.ErrInvalidHost == nil {
		t.Error("ErrInvalidHost should not be nil")
	}

	if utlsclient.ErrMaxRetriesExceeded == nil {
		t.Error("ErrMaxRetriesExceeded should not be nil")
	}

	// 测试错误消息
	if utlsclient.ErrConnectionClosed.Error() == "" {
		t.Error("ErrConnectionClosed should have error message")
	}
}
