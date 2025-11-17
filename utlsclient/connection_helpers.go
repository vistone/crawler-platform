package utlsclient

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/net/http2"
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
		return p.fingerprintLib.RandomProfile()
	}
	return GetRandomFingerprint()
}

// createAndValidateConnection 创建并验证连接
// 参数: ip - IP地址, targetHost - 目标主机, fingerprint - TLS指纹, validatePath - 验证路径（可选）
// 返回: 连接对象, 错误信息
func (p *UTLSHotConnPool) createAndValidateConnection(ip, targetHost string, fingerprint Profile, validatePath string) (*UTLSConnection, error) {
	// 若指定了验证路径，先使用临时TLS连接执行 GET 验证（非侵入，使用 Connection: close）
	if validatePath != "" {
		// 确保路径以 /
		if !strings.HasPrefix(validatePath, "/") {
			validatePath = "/" + validatePath
		}
		// 建立临时连接
		tmp, err := p.establishConnection(ip, targetHost, fingerprint)
		if err != nil {
			if p.ipAccessCtrl != nil {
				p.ipAccessCtrl.AddIP(ip, false)
			}
			return nil, err
		}
		// 优先使用 HTTP/2 验证
		req := &http.Request{
			Method: "GET",
			URL:    &url.URL{Scheme: "https", Host: targetHost, Path: validatePath},
			Header: make(http.Header),
			Host:   targetHost,
		}
		req.Header.Set("User-Agent", fingerprint.UserAgent)
		req.Header.Set("Accept", "*/*")
		var code int
		if cc, h2err := (&http2.Transport{}).NewClientConn(tmp.tlsConn); h2err == nil {
			resp, err := cc.RoundTrip(req)
			if err != nil {
				// 回退到 HTTP/1.1 原始请求
			} else {
				code = resp.StatusCode
				// 可选校验响应体长度为13（若服务端返回Content-Length）
				if resp.ContentLength != -1 && resp.ContentLength != 13 {
					code = 0 // 标记失败
				}
				if resp.Body != nil {
					resp.Body.Close()
				}
			}
			cc.Close()
		}
		if code == 0 {
			// 回退到 HTTP/1.1 原始请求（某些环境不支持h2或协商失败）
			var b strings.Builder
			b.WriteString("GET ")
			b.WriteString(validatePath)
			b.WriteString(" HTTP/1.1\r\n")
			b.WriteString("Host: ")
			b.WriteString(targetHost)
			b.WriteString("\r\n")
			b.WriteString("User-Agent: ")
			b.WriteString(fingerprint.UserAgent)
			b.WriteString("\r\n")
			b.WriteString("Connection: close\r\n")
			b.WriteString("Accept: */*\r\n\r\n")
			if _, err := tmp.tlsConn.Write([]byte(b.String())); err != nil {
				tmp.Close()
				if p.ipAccessCtrl != nil {
					p.ipAccessCtrl.AddIP(ip, false)
				}
				return nil, err
			}
			reader := bufio.NewReader(tmp.tlsConn)
			statusLine, err := reader.ReadString('\n')
			if err != nil {
				tmp.Close()
				if p.ipAccessCtrl != nil {
					p.ipAccessCtrl.AddIP(ip, false)
				}
				return nil, err
			}
			parts := strings.Split(strings.TrimSpace(statusLine), " ")
			if len(parts) < 3 {
				tmp.Close()
				if p.ipAccessCtrl != nil {
					p.ipAccessCtrl.AddIP(ip, false)
				}
				return nil, fmt.Errorf("无效状态行: %s", statusLine)
			}
			if _, err := fmt.Sscanf(parts[1], "%d", &code); err != nil {
				tmp.Close()
				if p.ipAccessCtrl != nil {
					p.ipAccessCtrl.AddIP(ip, false)
				}
				return nil, fmt.Errorf("解析状态码失败: %v", err)
			}
		}
		// 关闭临时连接
		tmp.Close()
		if code != StatusOK {
			return nil, fmt.Errorf("连接验证失败，状态码: %d", code)
		}
	}

	// 验证通过或未指定验证路径后，建立实际热连接
	conn, err := p.establishConnection(ip, targetHost, fingerprint)
	if err != nil {
		if p.ipAccessCtrl != nil {
			p.ipAccessCtrl.AddIP(ip, false)
		}
		return nil, err
	}

	// 若未指定验证路径，则进行默认验证（HEAD /）
	if validatePath == "" {
		if err := p.validateConnection(conn); err != nil {
			conn.Close()
			return nil, err
		}
	}

	// 添加到连接池（通过验证才会走到这里，从而将IP加入白名单）
	p.addToPool(conn)
	return conn, nil
}
