package utlsclient

import (
	"sync"
	"time"
)

// Blacklist 负责管理被临时屏蔽的IP地址。
// 当一个IP被添加到黑名单后，它会在指定的超时时间内被屏蔽。
// 超时后，该IP会自动从黑名单中移除，可以被重新使用。
type Blacklist struct {
	mu      sync.RWMutex
	ips     map[string]time.Time // 存储IP和它被拉黑的时间
	timeout time.Duration      // 黑名单超时时间
}

// NewBlacklist 创建一个新的黑名单管理器。
// timeout 定义了IP被屏蔽的持续时间。
func NewBlacklist(timeout time.Duration) *Blacklist {
	// 如果超时时间未设置或无效，则提供一个合理的默认值
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &Blacklist{
		ips:     make(map[string]time.Time),
		timeout: timeout,
	}
}

// Add 将一个IP添加到黑名单中，并记录当前时间。
func (b *Blacklist) Add(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ips[ip] = time.Now()
}

// Remove 从黑名单中移除一个IP（手动移除，不等待超时）。
func (b *Blacklist) Remove(ip string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.ips, ip)
}

// IsBlocked 检查一个IP当前是否处于被屏蔽状态。
// 如果IP在黑名单中且未超过超时时间，则返回true。
// 如果IP已过期，此方法会将其从黑名单中懒删除，并返回false。
func (b *Blacklist) IsBlocked(ip string) bool {
	b.mu.Lock() // 需要写锁，因为可能会删除条目
	defer b.mu.Unlock()

	blockedAt, exists := b.ips[ip]
	if !exists {
		return false // 不在黑名单中
	}

	// 检查是否已过超时时间
	if time.Since(blockedAt) > b.timeout {
		// IP已过期，从黑名单中移除
		delete(b.ips, ip)
		return false // 不再被屏蔽
	}

	return true // 仍在屏蔽时间内
}

// Cleanup 遍历整个黑名单，移除所有已过期的IP。
// 这个方法应该被定期调用，以防止黑名单无限增长。
func (b *Blacklist) Cleanup() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cleanedCount := 0
	for ip, blockedAt := range b.ips {
		if now.Sub(blockedAt) > b.timeout {
			delete(b.ips, ip)
			cleanedCount++
		}
	}
	return cleanedCount
}

// GetBlockedIPs 返回当前所有未过期的黑名单IP列表。
func (b *Blacklist) GetBlockedIPs() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var blockedIPs []string
	now := time.Now()
	for ip, blockedAt := range b.ips {
		if now.Sub(blockedAt) <= b.timeout {
			blockedIPs = append(blockedIPs, ip)
		}
	}
	return blockedIPs
}
