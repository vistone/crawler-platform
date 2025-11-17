package utlsclient

import (
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
)

// acquireIP 从IP池或DNS解析获取IP地址
// 参数: pool - 连接池实例, targetHost - 目标主机
// 返回: IP地址, 错误信息
func (p *UTLSHotConnPool) acquireIP(targetHost string) (string, error) {
	if p.ipPool != nil {
		ip, err := p.ipPool.GetIP()
		if err != nil {
			return "", fmt.Errorf("获取IP失败: %v", err)
		}
		return ip, nil
	}

	// 如果没有IP池，解析域名获取IP
	ips, err := net.LookupIP(targetHost)
	if err != nil {
		return "", fmt.Errorf("解析域名失败: %v", err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("没有找到域名的IP地址")
	}

	// 选择第一个IPv4地址
	for _, addr := range ips {
		if addr.To4() != nil {
			return addr.String(), nil
		}
	}

	return "", fmt.Errorf("没有找到IPv4地址")
}

// validateIPAccess 验证IP是否允许访问
// 参数: ip - IP地址
// 返回: 是否允许, 错误信息
func (p *UTLSHotConnPool) validateIPAccess(ip string) bool {
	if p.ipAccessCtrl == nil {
		return true // 如果没有访问控制器，默认允许
	}
	return p.ipAccessCtrl.IsIPAllowed(ip)
}

// selectFingerprint 选择TLS指纹
// 返回: 指纹配置
func (p *UTLSHotConnPool) selectFingerprint() Profile {
	if p.fingerprintLib != nil {
		return p.fingerprintLib.RandomRecommendedProfile()
	}

	// 默认指纹
	return Profile{
		HelloID:   utls.HelloChrome_Auto,
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
	}
}

// createAndValidateConnection 创建并验证连接
// 参数: ip - IP地址, targetHost - 目标主机, fingerprint - TLS指纹, validatePath - 验证路径（可选）
// 返回: 连接对象, 错误信息
func (p *UTLSHotConnPool) createAndValidateConnection(ip, targetHost string, fingerprint Profile, validatePath string) (*UTLSConnection, error) {
	// 建立连接
	conn, err := p.establishConnection(ip, targetHost, fingerprint)
	if err != nil {
		// 连接失败，加入黑名单
		if p.ipAccessCtrl != nil {
			p.ipAccessCtrl.AddIP(ip, false)
		}
		return nil, err
	}

	// 验证连接
	var validateErr error
	if validatePath != "" {
		validateErr = p.validateConnectionWithPath(conn, validatePath)
	} else {
		validateErr = p.validateConnection(conn)
	}

	if validateErr != nil {
		conn.Close()
		return nil, validateErr
	}

	// 添加到连接池
	p.addToPool(conn)

	return conn, nil
}

