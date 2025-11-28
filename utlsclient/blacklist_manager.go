package utlsclient

import (
	"sync"
	"time"

	projlogger "crawler-platform/logger"
)

// BlacklistManager 黑名单管理器接口
// 负责定期检查黑名单并清理相关连接
type BlacklistManager interface {
	// Start 启动黑名单管理器
	Start()

	// Stop 停止黑名单管理器
	Stop()

	// CheckAndRecover 检查并恢复黑名单中的连接
	// 输出: error - 错误信息
	CheckAndRecover() error
}

// DefaultBlacklistManager 默认黑名单管理器实现
type DefaultBlacklistManager struct {
	ipAccessCtrl      *IPAccessController
	connManager       *ConnectionManager
	checkInterval     time.Duration
	done              chan struct{}
	mu                sync.Mutex
	running           bool
	wg                sync.WaitGroup
}

// NewDefaultBlacklistManager 创建默认黑名单管理器
// 输入: ipAccessCtrl - IP访问控制器, connManager - 连接管理器, checkInterval - 检查间隔
// 输出: *DefaultBlacklistManager - 黑名单管理器实例
func NewDefaultBlacklistManager(
	ipAccessCtrl *IPAccessController,
	connManager *ConnectionManager,
	checkInterval time.Duration,
) *DefaultBlacklistManager {
	return &DefaultBlacklistManager{
		ipAccessCtrl:  ipAccessCtrl,
		connManager:    connManager,
		checkInterval: checkInterval,
		done:           make(chan struct{}),
	}
}

// Start 启动黑名单管理器
func (b *DefaultBlacklistManager) Start() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		return
	}

	b.running = true
	b.done = make(chan struct{})
	b.wg.Add(1)
	go b.blacklistCheckLoop()
}

// Stop 停止黑名单管理器
func (b *DefaultBlacklistManager) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	close(b.done)
	b.mu.Unlock()

	b.wg.Wait()
}

// CheckAndRecover 检查并恢复黑名单中的连接
func (b *DefaultBlacklistManager) CheckAndRecover() error {
	if b.ipAccessCtrl == nil {
		return nil
	}

	// 获取黑名单IP列表
	blacklistIPs := b.ipAccessCtrl.GetBlockedIPs()

	if len(blacklistIPs) == 0 {
		return nil
	}

	// 清理黑名单中的连接
	for _, ip := range blacklistIPs {
		b.connManager.RemoveConnection(ip)
		projlogger.Debug("清理黑名单连接: %s", ip)
	}

	return nil
}

// blacklistCheckLoop 黑名单检查循环
func (b *DefaultBlacklistManager) blacklistCheckLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 立即检查done通道，确保快速响应停止信号
			select {
			case <-b.done:
				return
			default:
			}
			b.CheckAndRecover()
		case <-b.done:
			return
		}
	}
}
