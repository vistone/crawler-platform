package utlsclient

import (
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

// NewTestConnection 创建测试用的连接（仅用于测试）
// 该函数放在 utlsclient 包内，便于单测构造受控的 UTLSConnection 实例
func NewTestConnection(ip, host string) *UTLSConnection {
	conn := &UTLSConnection{
		targetIP:    ip,
		targetHost:  host,
		created:     time.Now(),
		lastUsed:    time.Now(),
		lastChecked: time.Now(),
		inUse:       false,
		healthy:     true,
		fingerprint: Profile{
			HelloID:   utls.HelloChrome_Auto,
			UserAgent: "test-agent",
		},
	}

	// 初始化并发控制
	conn.mu = sync.Mutex{}
	conn.cond = sync.NewCond(&conn.mu)

	return conn
}

// SetTestConnectionLastUsed 设置测试连接的 lastUsed 时间（仅用于测试）
func SetTestConnectionLastUsed(conn *UTLSConnection, t time.Time) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.lastUsed = t
}

// SetTestConnectionCreated 设置测试连接的 created 时间（仅用于测试）
func SetTestConnectionCreated(conn *UTLSConnection, t time.Time) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.created = t
}

// SetTestConnectionInUse 设置测试连接的 inUse 状态（仅用于测试）
func SetTestConnectionInUse(conn *UTLSConnection, inUse bool) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.inUse = inUse
}

// GetTestConnectionTargetIP 获取测试连接的目标IP（仅用于测试）
func GetTestConnectionTargetIP(conn *UTLSConnection) string {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.targetIP
}


