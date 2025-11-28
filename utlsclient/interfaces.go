package utlsclient

import (
	"context"
	"io"
	"net/http"
)

// IPPoolProvider 定义IP池提供者的接口
// 用于从IP池获取IP地址或域名对应的IP列表
type IPPoolProvider interface {
	// GetIP 从池中获取一个可用的IP地址
	// 返回: IP地址字符串, 错误信息
	GetIP() (string, error)

	// GetIPsForDomain 获取指定域名的最新IP列表
	// 参数: domain - 域名
	// 返回: IP地址列表
	GetIPsForDomain(domain string) []string
}

// AccessController 定义访问控制器的接口
// 用于管理IP黑白名单和访问控制
type AccessController interface {
	// IsIPAllowed 检查IP是否被允许访问
	// 参数: ip - IP地址
	// 返回: 是否允许
	IsIPAllowed(ip string) bool

	// AddIP 添加IP到指定名单
	// 参数: ip - IP地址, isWhite - 是否添加到白名单
	AddIP(ip string, isWhite bool)

	// GetAllowedIPs 获取白名单IP列表
	// 返回: 白名单IP列表
	GetAllowedIPs() []string

	// GetBlockedIPs 获取黑名单IP列表
	// 返回: 黑名单IP列表
	GetBlockedIPs() []string

	// RemoveFromBlacklist 从黑名单移除IP
	// 参数: ip - IP地址
	RemoveFromBlacklist(ip string)

	// AddToWhitelist 添加IP到白名单
	// 参数: ip - IP地址
	AddToWhitelist(ip string)
}

// HTTPClient 定义HTTP客户端的接口
// 用于执行HTTP请求
type HTTPClient interface {
	// Do 执行HTTP请求
	// 参数: req - HTTP请求
	// 返回: HTTP响应, 错误信息
	Do(req *http.Request) (*http.Response, error)

	// DoWithContext 带上下文的HTTP请求
	// 参数: ctx - 上下文, req - HTTP请求
	// 返回: HTTP响应, 错误信息
	DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error)

	// Get 执行GET请求
	// 参数: url - 请求URL
	// 返回: HTTP响应, 错误信息
	Get(url string) (*http.Response, error)

	// Post 执行POST请求
	// 参数: url - 请求URL, contentType - 内容类型, body - 请求体
	// 返回: HTTP响应, 错误信息
	Post(url string, contentType string, body io.Reader) (*http.Response, error)

	// Head 执行HEAD请求
	// 参数: url - 请求URL
	// 返回: HTTP响应, 错误信息
	Head(url string) (*http.Response, error)
}

// DNSUpdater 和 BlacklistManager 接口定义在各自的实现文件中
// - DNSUpdater: 定义在 dns_updater.go
// - BlacklistManager: 定义在 blacklist_manager.go

// 日志接口与实现移至项目级 logger 包
