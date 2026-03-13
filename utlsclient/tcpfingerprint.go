package utlsclient

import (
	"fmt"
	"net"
	"syscall"

	projlogger "crawler-platform/logger"
)

// TCPFingerprint 定义 TCP/IP 层指纹配置
// 不同操作系统有不同的 TCP 参数特征，用于伪装成特定平台的连接
type TCPFingerprint struct {
	Platform      string // 平台名称 (Windows/macOS/Linux/iOS)
	InitialTTL    int    // IP 初始 TTL 值
	WindowSize    int    // TCP 窗口大小
	WindowScaling int    // 窗口缩放因子
	MSS           int    // 最大段大小
	SACK          bool   // 是否支持 SACK
	Timestamps    bool   // 是否启用时间戳
	NOP           bool   // 是否启用 NOP 选项
	TFO           bool   // 是否支持 TCP Fast Open
}

// 预定义的 TCP 指纹库
// 基于各操作系统默认的 TCP 参数特征
var tcpFingerprints = map[string]TCPFingerprint{
	"Windows": {
		Platform:      "Windows",
		InitialTTL:    128,
		WindowSize:    64240,
		WindowScaling: 8,
		MSS:           1460,
		SACK:          true,
		Timestamps:    true,
		NOP:           true,
		TFO:           false,
	},
	"macOS": {
		Platform:      "macOS",
		InitialTTL:    64,
		WindowSize:    65535,
		WindowScaling: 8,
		MSS:           1460,
		SACK:          true,
		Timestamps:    true,
		NOP:           true,
		TFO:           true,
	},
	"Linux": {
		Platform:      "Linux",
		WindowSize:    29200,
		WindowScaling: 7,
		MSS:           1460,
		SACK:          true,
		Timestamps:    true,
		NOP:           true,
		TFO:           true,
	},
	"iOS": {
		Platform:      "iOS",
		InitialTTL:    64,
		WindowSize:    65535,
		WindowScaling: 8,
		MSS:           1460,
		SACK:          true,
		Timestamps:    true,
		NOP:           true,
		TFO:           true,
	},
}

// GetTCPFingerprint 根据平台名称获取对应的 TCP 指纹配置
// 如果找不到对应平台，返回 Linux 作为默认值
func GetTCPFingerprint(platform string) TCPFingerprint {
	if fp, ok := tcpFingerprints[platform]; ok {
		return fp
	}
	// 默认返回 Linux 配置
	return tcpFingerprints["Linux"]
}

// ApplyTCPFingerprint 将 TCP 指纹应用到 TCP 连接
// 根据平台设置对应的 TCP 参数（TTL、窗口大小、Window Scaling、MSS、SACK、Timestamps等）
func ApplyTCPFingerprint(tcpConn *net.TCPConn, platform string) error {
	if tcpConn == nil {
		return fmt.Errorf("TCP 连接为空")
	}

	fp := GetTCPFingerprint(platform)

	// 获取底层文件描述符
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return fmt.Errorf("获取 SyscallConn 失败: %w", err)
	}

	// 设置 IP TTL (Time To Live)
	var ttlErr error
	ttlErr = rawConn.Control(func(fd uintptr) {
		if fp.InitialTTL > 0 {
			// 设置 IPv4 TTL
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_TTL, fp.InitialTTL)
			// 设置 IPv6 Hop Limit (IPv6 的 TTL 等价物)
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_UNICAST_HOPS, fp.InitialTTL)
		}
	})
	if ttlErr != nil {
		projlogger.Debug("设置 TCP TTL 失败 (平台: %s): %v", platform, ttlErr)
	}

	// 设置 TCP 窗口大小
	if fp.WindowSize > 0 {
		if err := tcpConn.SetWriteBuffer(fp.WindowSize); err != nil {
			projlogger.Debug("设置 TCP 写缓冲区失败 (平台: %s): %v", platform, err)
		}
		if err := tcpConn.SetReadBuffer(fp.WindowSize); err != nil {
			projlogger.Debug("设置 TCP 读缓冲区失败 (平台: %s): %v", platform, err)
		}
	}

	// 设置高级 TCP 选项（Window Scaling、SACK、Timestamps）
	rawConn.Control(func(fd uintptr) {
		// TCP Window Scaling (RFC 1323) - 启用窗口缩放以支持大窗口
		if fp.WindowScaling > 0 {
			// 在 Linux 上，窗口缩放是通过 tcp_adv_win_scale 和动态调整的
			// 这里我们尝试设置，但可能会失败（需要 root 权限）
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_WINDOW_CLAMP, fp.WindowSize)
		}

		// TCP SACK (选择性确认) - 大多数现代系统默认启用
		if fp.SACK {
			// 尝试启用 SACK (TCP_SACK 选项在 Linux 上通常是只读的，但尝试设置)
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 1, 1) // TCP_SACK = 1
		}

		// TCP Timestamps (RFC 1323) - 用于 RTT 计算和保护序列号回绕
		// 注意：TCP_TIMESTAMP 常量值在不同架构上可能不同，使用原始值 0x18 (24)
		if fp.Timestamps {
			// 尝试启用时间戳选项 (TCP_TIMESTAMP = 0x18)
			syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, 0x18, 1)
		}

		// TCP_NODELAY - 禁用 Nagle 算法，减少延迟（爬虫场景通常需要）
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1)
	})

	// 设置 KeepAlive 参数（各平台通用）
	if err := tcpConn.SetKeepAlive(true); err != nil {
		projlogger.Debug("设置 TCP KeepAlive 失败: %v", err)
	}

	projlogger.Debug("已应用 TCP 指纹 (平台: %s, TTL: %d, Window: %d, Scaling: %d, SACK: %v, Timestamps: %v)",
		fp.Platform, fp.InitialTTL, fp.WindowSize, fp.WindowScaling, fp.SACK, fp.Timestamps)

	return nil
}

// ApplyTCPFingerprintByProfile 根据浏览器指纹配置应用 TCP 指纹
// 确保 TLS 指纹和 TCP 指纹来自同一平台
func ApplyTCPFingerprintByProfile(tcpConn *net.TCPConn, profile Profile) error {
	if tcpConn == nil {
		return fmt.Errorf("TCP 连接为空")
	}

	// 将浏览器指纹的平台映射到 TCP 指纹平台
	platform := mapPlatform(profile.Platform)

	return ApplyTCPFingerprint(tcpConn, platform)
}

// mapPlatform 将浏览器指纹的平台名称映射到 TCP 指纹平台
// 处理不同命名方式的一致性
func mapPlatform(platform string) string {
	switch platform {
	case "Windows":
		return "Windows"
	case "macOS":
		return "macOS"
	case "Linux":
		return "Linux"
	case "iOS":
		return "iOS"
	default:
		return "Linux"
	}
}

// GetFingerprintConsistency 检查指纹配置的一致性
// 返回 TLS 指纹和 TCP 指纹是否匹配
func GetFingerprintConsistency(profile Profile) (bool, string) {
	tcpFP := GetTCPFingerprint(mapPlatform(profile.Platform))

	if tcpFP.Platform != mapPlatform(profile.Platform) {
		return false, fmt.Sprintf("平台不匹配: TLS=%s, TCP=%s", profile.Platform, tcpFP.Platform)
	}

	return true, fmt.Sprintf("指纹一致 (平台: %s, TTL: %d, Window: %d)",
		tcpFP.Platform, tcpFP.InitialTTL, tcpFP.WindowSize)
}

// TCPFingerprintWithIPPool 包含 IP 池的 TCP 指纹配置
// 用于确保使用动态 IPv6 地址，防止固定 IP 外泄
type TCPFingerprintWithIPPool struct {
	Fingerprint TCPFingerprint
	LocalIP     net.IP
	LocalIPStr  string
}

// ValidateIPPoolUsage 验证是否成功使用了 IP 地址池
// 防止固定 IP 地址外泄
func ValidateIPPoolUsage(localIPStr string, hasIPPool bool) (bool, string) {
	if !hasIPPool {
		return true, "未配置 IP 池，使用系统默认地址"
	}

	if localIPStr == "" {
		return false, "警告：配置了 IP 池但未能获取地址，可能使用系统默认地址，存在固定 IP 外泄风险"
	}

	// 检查是否为 IPv6 地址
	ip := net.ParseIP(localIPStr)
	if ip != nil && ip.To4() == nil {
		return true, fmt.Sprintf("成功使用 IPv6 地址池地址: %s", localIPStr)
	}

	return true, fmt.Sprintf("使用 IP 池地址: %s", localIPStr)
}

// LogFingerprintAndIP 记录完整的指纹和 IP 信息
// 用于调试和监控反检测效果
func LogFingerprintAndIP(profile Profile, localIPStr string, targetIP string) {
	tcpFP := GetTCPFingerprint(mapPlatform(profile.Platform))

	// 检查 IP 类型
	ipType := "未知"
	if localIPStr != "" {
		ip := net.ParseIP(localIPStr)
		if ip != nil {
			if ip.To4() == nil {
				ipType = "IPv6"
			} else {
				ipType = "IPv4"
			}
		}
	}

	projlogger.Debug("反检测配置 - 平台: %s, TLS: %s, TCP_TTL: %d, TCP_Window: %d, 源IP: %s (%s), 目标IP: %s",
		profile.Platform,
		profile.Name,
		tcpFP.InitialTTL,
		tcpFP.WindowSize,
		localIPStr,
		ipType,
		targetIP,
	)
}
