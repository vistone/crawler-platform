package utlsclient

import (
	"errors"
	"time"
)

// 网络相关常量
const (
	// DefaultHTTPSPort HTTPS默认端口
	DefaultHTTPSPort = 443

	// DefaultHTTPPort HTTP默认端口
	DefaultHTTPPort = 80
)

// HTTP状态码常量
const (
	// StatusOK HTTP 200 成功
	StatusOK = 200

	// StatusNoContent HTTP 204 无内容
	StatusNoContent = 204

	// StatusForbidden HTTP 403 禁止访问
	StatusForbidden = 403
)

// 协议相关常量
const (
	// HTTPSProtocol HTTPS协议
	HTTPSProtocol = "https"

	// HTTPProtocol HTTP协议
	HTTPProtocol = "http"
)

// 错误相关常量
const (
	// DefaultRetryDelay 默认重试延迟
	DefaultRetryDelay = 1 * time.Second

	// MinSuccessRate 最小成功率阈值
	MinSuccessRate = 0.5
)

// 连接错误关键词
var (
	// ConnectionErrorKeywords 连接错误关键词列表
	ConnectionErrorKeywords = []string{
		"connection",
		"broken pipe",
		"connection reset",
		"connection refused",
		"connection closed",
	}
)

// 错误定义
var (
	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("connection closed")

	// ErrConnectionBroken 连接已断开
	ErrConnectionBroken = errors.New("connection broken")

	// ErrIPBlocked IP被封禁
	ErrIPBlocked = errors.New("IP blocked")

	// ErrConnectionUnhealthy 连接不健康
	ErrConnectionUnhealthy = errors.New("connection unhealthy")

	// ErrConnectionTimeout 连接超时
	ErrConnectionTimeout = errors.New("connection timeout")

	// ErrInvalidURL 无效的URL
	ErrInvalidURL = errors.New("invalid URL")

	// ErrInvalidHost 无效的主机名
	ErrInvalidHost = errors.New("invalid hostname")

	// ErrMaxRetriesExceeded 超过最大重试次数
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

