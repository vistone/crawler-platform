package utlsclient

import (
	"sync"
)

// IPAccessController IP访问控制器，负责IP白名单和黑名单管理
type IPAccessController struct {
	whitelist map[string]bool // 白名单 (IP -> true)
	blacklist map[string]bool // 黑名单 (IP -> true)
	mu        sync.RWMutex    // 读写锁
}

// NewIPAccessController 创建新的IP访问控制器
func NewIPAccessController() *IPAccessController {
	return &IPAccessController{
		whitelist: make(map[string]bool),
		blacklist: make(map[string]bool),
	}
}

// IsIPAllowed 检查IP是否被允许访问
func (ac *IPAccessController) IsIPAllowed(ip string) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	// 如果在黑名单中，拒绝访问
	if ac.blacklist[ip] {
		Debug("IP在黑名单中，拒绝访问: %s", ip)
		return false
	}

	// 如果在白名单中，允许访问
	if ac.whitelist[ip] {
		Debug("IP在白名单中，允许访问: %s", ip)
		return true
	}

	// 如果不在任何列表中，默认允许访问（可根据需求修改）
	Debug("IP不在任何列表中，默认允许访问: %s", ip)
	return true
}

// AddIP 添加IP到白名单或黑名单
func (ac *IPAccessController) AddIP(ip string, isWhite bool) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if isWhite {
		// 添加到白名单，从黑名单中移除
		ac.whitelist[ip] = true
		delete(ac.blacklist, ip)
		Info("IP已添加到白名单: %s", ip)
	} else {
		// 添加到黑名单，从白名单中移除
		ac.blacklist[ip] = true
		delete(ac.whitelist, ip)
		Info("IP已添加到黑名单: %s", ip)
	}
}

// GetAllowedIPs 获取白名单IP列表
func (ac *IPAccessController) GetAllowedIPs() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ips := make([]string, 0, len(ac.whitelist))
	for ip := range ac.whitelist {
		ips = append(ips, ip)
	}

	return ips
}

// GetBlockedIPs 获取黑名单IP列表
func (ac *IPAccessController) GetBlockedIPs() []string {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	ips := make([]string, 0, len(ac.blacklist))
	for ip := range ac.blacklist {
		ips = append(ips, ip)
	}

	return ips
}

// RemoveFromBlacklist 从黑名单中移除IP
func (ac *IPAccessController) RemoveFromBlacklist(ip string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.blacklist[ip] {
		delete(ac.blacklist, ip)
		Info("IP已从黑名单移除: %s", ip)
	}
}

// AddToWhitelist 添加IP到白名单
func (ac *IPAccessController) AddToWhitelist(ip string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if !ac.whitelist[ip] {
		ac.whitelist[ip] = true
		// 如果在黑名单中，从黑名单移除
		delete(ac.blacklist, ip)
		Info("IP已添加到白名单: %s", ip)
	}
}

// RemoveFromWhitelist 从白名单中移除IP
func (ac *IPAccessController) RemoveFromWhitelist(ip string) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.whitelist[ip] {
		delete(ac.whitelist, ip)
		Info("IP已从白名单移除: %s", ip)
	}
}

// ClearWhitelist 清空白名单
func (ac *IPAccessController) ClearWhitelist() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.whitelist = make(map[string]bool)
	Info("白名单已清空")
}

// ClearBlacklist 清空黑名单
func (ac *IPAccessController) ClearBlacklist() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.blacklist = make(map[string]bool)
	Info("黑名单已清空")
}

// GetStats 获取访问控制统计信息
func (ac *IPAccessController) GetStats() (whitelistCount, blacklistCount int) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	whitelistCount = len(ac.whitelist)
	blacklistCount = len(ac.blacklist)

	return
}

// Contains 检查IP是否在指定的列表中
func (ac *IPAccessController) Contains(ip string, isWhite bool) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if isWhite {
		return ac.whitelist[ip]
	}
	return ac.blacklist[ip]
}

// Size 获取指定列表的大小
func (ac *IPAccessController) Size(isWhite bool) int {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if isWhite {
		return len(ac.whitelist)
	}
	return len(ac.blacklist)
}

// IsEmpty 检查指定列表是否为空
func (ac *IPAccessController) IsEmpty(isWhite bool) bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	if isWhite {
		return len(ac.whitelist) == 0
	}
	return len(ac.blacklist) == 0
}
