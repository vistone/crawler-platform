package utlsclient

import "errors"

// 错误定义
// 统一管理所有包级别的错误类型和变量，便于错误处理和使用

var (
	// ErrIPBlockedBy403 表示IP因为403 Forbidden而被拒绝
	ErrIPBlockedBy403 = errors.New("validation failed with 403 Forbidden")

	// ErrConnectionUnhealthy 表示连接已标记为不健康
	ErrConnectionUnhealthy = errors.New("connection marked as unhealthy")

	// ErrNoAvailableConnection 表示没有可用的连接
	ErrNoAvailableConnection = errors.New("no available connection")

	// ErrConnectionInUse 表示连接正在使用中
	ErrConnectionInUse = errors.New("connection is in use")

	// ErrInvalidConfig 表示配置无效
	ErrInvalidConfig = errors.New("invalid configuration")
)
