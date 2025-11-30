package test

import (
	"time"

	"crawler-platform/utlsclient"
)

// ClosePoolWithTimeout 带超时的连接池关闭辅助函数
// 输入:
//   - pool: 要关闭的连接池
//   - timeout: 超时时间
// 输出: 无
func ClosePoolWithTimeout(pool *utlsclient.UTLSHotConnPool, timeout time.Duration) {
	if pool == nil {
		return
	}
	
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		pool.Close()
	}()
	
	select {
	case <-closeDone:
		// Close完成
	case <-time.After(timeout):
		// 超时，但继续执行（避免测试挂起）
	}
}
