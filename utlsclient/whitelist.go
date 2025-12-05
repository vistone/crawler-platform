package utlsclient

import "sync"

// Whitelist 负责管理允许使用的IP地址列表。
// 只有存在于白名单中的IP才能被用于建立新连接。
type Whitelist struct {
	mu    sync.RWMutex
	ips   map[string]struct{} // 使用空结构体作为值，节省内存
	allowAll bool              // 如果为true，则允许所有IP
}

// NewWhitelist 创建一个新的白名单管理器。
// initialIPs 是初始的IP列表。
// allowAll 设置为true时，IsAllowed将始终返回true，相当于禁用白名单检查。
func NewWhitelist(initialIPs []string, allowAll bool) *Whitelist {
	wl := &Whitelist{
		ips:      make(map[string]struct{}),
		allowAll: allowAll,
	}
	if !allowAll {
		for _, ip := range initialIPs {
			wl.ips[ip] = struct{}{}
		}
	}
	return wl
}

// IsAllowed 检查给定的IP是否在白名单中。
func (wl *Whitelist) IsAllowed(ip string) bool {
	if wl.allowAll {
		return true
	}
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	_, exists := wl.ips[ip]
	return exists
}

// Add 向白名单中添加一个IP。
func (wl *Whitelist) Add(ip string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	wl.ips[ip] = struct{}{}
}

// Remove 从白名单中移除一个IP。
func (wl *Whitelist) Remove(ip string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	delete(wl.ips, ip)
}

// SetIPs 重新设置整个白名单列表。
func (wl *Whitelist) SetIPs(ips []string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	wl.ips = make(map[string]struct{})
	for _, ip := range ips {
		wl.ips[ip] = struct{}{}
	}
}
