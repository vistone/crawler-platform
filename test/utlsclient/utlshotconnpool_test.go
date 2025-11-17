package utlsclient_test

import (
	"testing"
	"time"

	"crawler-platform/utlsclient"
)

func TestDefaultPoolConfig(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	
	if config.MaxConnections != 100 {
		t.Errorf("Expected MaxConnections to be 100, got %d", config.MaxConnections)
	}
	
	if config.BlacklistCheckInterval != 300*time.Second {
		t.Errorf("Expected BlacklistCheckInterval to be 300s, got %v", config.BlacklistCheckInterval)
	}
	
	if config.DNSUpdateInterval != 1800*time.Second {
		t.Errorf("Expected DNSUpdateInterval to be 1800s, got %v", config.DNSUpdateInterval)
	}
}

func TestNewUTLSHotConnPool(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	pool := utlsclient.NewUTLSHotConnPool(config)
	
	if pool == nil {
		t.Fatal("Failed to create pool")
	}
	
	stats := pool.GetStats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected TotalConnections to be 0, got %d", stats.TotalConnections)
	}
	
	// Test closing the pool
	if err := pool.Close(); err != nil {
		t.Errorf("Failed to close pool: %v", err)
	}
}

func TestPoolGetRandomFingerprint(t *testing.T) {
	config := utlsclient.DefaultPoolConfig()
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()
	
	// This should not panic
	// Note: getRandomFingerprint is not exported, so we test through public API
	fingerprint := utlsclient.GetRandomFingerprint()
	
	if fingerprint.Name == "" {
		t.Error("Fingerprint name should not be empty")
	}
	
	if fingerprint.UserAgent == "" {
		t.Error("Fingerprint UserAgent should not be empty")
	}
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://www.example.com/path", "www.example.com"},
		{"http://example.com", "example.com"},
		{"www.example.com/path/to/page", "www.example.com"},
		{"example.com", "example.com"},
		{"", ""},
	}
	
	for _, test := range tests {
		result := utlsclient.ExtractHostname(test.input)
		if result != test.expected {
			t.Errorf("extractHostname(%s) = %s, expected %s", test.input, result, test.expected)
		}
	}
}
