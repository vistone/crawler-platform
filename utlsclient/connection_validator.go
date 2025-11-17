package utlsclient

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ConnectionValidator 连接验证器，负责连接的有效性验证
type ConnectionValidator struct {
	config *PoolConfig
}

// NewConnectionValidator 创建新的连接验证器
func NewConnectionValidator(config *PoolConfig) *ConnectionValidator {
	return &ConnectionValidator{
		config: config,
	}
}

// ValidateConnection 验证连接的基本健康状态
func (cv *ConnectionValidator) ValidateConnection(conn *UTLSConnection) error {
	if conn == nil {
		return fmt.Errorf("连接为空")
	}

	// 检查连接是否健康
	conn.mu.Lock()
	isHealthy := conn.healthy
	conn.mu.Unlock()

	if !isHealthy {
		return fmt.Errorf("连接不健康")
	}

	// 执行默认的根路径验证
	return cv.ValidateConnectionWithPath(conn, "/")
}

// ValidateConnectionWithPath 验证连接的指定路径
func (cv *ConnectionValidator) ValidateConnectionWithPath(conn *UTLSConnection, path string) error {
	if conn == nil {
		return fmt.Errorf("连接为空")
	}

	if path == "" {
		path = "/"
	}

	Info("开始验证连接路径: %s%s", conn.targetHost, path)

	// 创建HEAD请求进行验证
	req := &http.Request{
		Method: "HEAD",
		URL: &url.URL{
			Scheme: "https",
			Host:   conn.targetHost,
			Path:   path,
		},
		Header: make(http.Header),
		Host:   conn.targetHost,
	}

	// 设置请求头
	req.Header.Set("User-Agent", "UTLSHotConnPool/1.0 HealthCheck")
	req.Header.Set("Connection", "keep-alive")

	// 创建UTLSClient进行验证
	client := NewUTLSClient(conn)
	client.SetTimeout(cv.config.ConnTimeout)

	// 执行请求
	startTime := time.Now()
	resp, err := client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		Error("连接验证失败: %s -> %v", conn.targetIP, err)
		return fmt.Errorf("连接验证失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		Error("连接验证失败，状态码异常: %s -> %d", conn.targetIP, resp.StatusCode)
		return fmt.Errorf("连接验证失败，状态码异常: %d", resp.StatusCode)
	}

	// 更新连接统计信息
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.mu.Unlock()

	Info("连接验证成功: %s (响应时间: %v, 状态码: %d)", conn.targetIP, responseTime, resp.StatusCode)
	return nil
}

// ValidateConnectionWithFullURL 使用完整URL验证连接
func (cv *ConnectionValidator) ValidateConnectionWithFullURL(conn *UTLSConnection, fullURL string) error {
	if conn == nil {
		return fmt.Errorf("连接为空")
	}

	// 解析URL
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return fmt.Errorf("URL解析失败: %v", err)
	}

	// 检查主机名是否匹配
	if parsedURL.Hostname() != conn.targetHost {
		Error("URL主机名不匹配: %s != %s", parsedURL.Hostname(), conn.targetHost)
		return fmt.Errorf("URL主机名不匹配")
	}

	Info("使用完整URL验证连接: %s", fullURL)

	// 创建HEAD请求进行验证
	req := &http.Request{
		Method: "HEAD",
		URL:    parsedURL,
		Header: make(http.Header),
		Host:   conn.targetHost,
	}

	// 设置请求头
	req.Header.Set("User-Agent", "UTLSHotConnPool/1.0 URLValidation")
	req.Header.Set("Connection", "keep-alive")

	// 创建UTLSClient进行验证
	client := NewUTLSClient(conn)
	client.SetTimeout(cv.config.ConnTimeout)

	// 执行请求
	startTime := time.Now()
	resp, err := client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		Error("URL验证失败: %s -> %v", conn.targetIP, err)
		return fmt.Errorf("URL验证失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		Error("URL验证失败，状态码异常: %s -> %d", conn.targetIP, resp.StatusCode)
		return fmt.Errorf("URL验证失败，状态码异常: %d", resp.StatusCode)
	}

	// 更新连接统计信息
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.mu.Unlock()

	Info("URL验证成功: %s (响应时间: %v, 状态码: %d)", conn.targetIP, responseTime, resp.StatusCode)
	return nil
}

// ValidateConnectionWithGET 使用GET请求验证连接（更严格的验证）
func (cv *ConnectionValidator) ValidateConnectionWithGET(conn *UTLSConnection, path string, maxBodySize int64) error {
	if conn == nil {
		return fmt.Errorf("连接为空")
	}

	if path == "" {
		path = "/"
	}

	Info("使用GET请求验证连接: %s%s", conn.targetHost, path)

	// 创建GET请求进行验证
	req := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "https",
			Host:   conn.targetHost,
			Path:   path,
		},
		Header: make(http.Header),
		Host:   conn.targetHost,
	}

	// 设置请求头
	req.Header.Set("User-Agent", "UTLSHotConnPool/1.0 GETValidation")
	req.Header.Set("Connection", "keep-alive")

	// 创建UTLSClient进行验证
	client := NewUTLSClient(conn)
	client.SetTimeout(cv.config.ConnTimeout)

	// 执行请求
	startTime := time.Now()
	resp, err := client.Do(req)
	responseTime := time.Since(startTime)

	if err != nil {
		Error("GET验证失败: %s -> %v", conn.targetIP, err)
		return fmt.Errorf("GET验证失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		Error("GET验证失败，状态码异常: %s -> %d", conn.targetIP, resp.StatusCode)
		return fmt.Errorf("GET验证失败，状态码异常: %d", resp.StatusCode)
	}

	// 限制读取的响应体大小
	if maxBodySize > 0 && resp.ContentLength > maxBodySize {
		Error("响应体过大，验证失败: %s -> %d bytes", conn.targetIP, resp.ContentLength)
		return fmt.Errorf("响应体过大: %d bytes", resp.ContentLength)
	}

	// 更新连接统计信息
	conn.mu.Lock()
	conn.lastUsed = time.Now()
	conn.mu.Unlock()

	Info("GET验证成功: %s (响应时间: %v, 状态码: %d, 内容长度: %d)", conn.targetIP, responseTime, resp.StatusCode, resp.ContentLength)
	return nil
}

// BatchValidateConnections 批量验证连接
func (cv *ConnectionValidator) BatchValidateConnections(connections []*UTLSConnection, path string) (valid, invalid int, errors []error) {
	for _, conn := range connections {
		if err := cv.ValidateConnectionWithPath(conn, path); err != nil {
			invalid++
			errors = append(errors, fmt.Errorf("连接验证失败 %s: %v", conn.targetIP, err))
			Error("批量验证失败: %s -> %v", conn.targetIP, err)
		} else {
			valid++
			Info("批量验证成功: %s", conn.targetIP)
		}
	}

	return valid, invalid, errors
}

// QuickHealthCheck 快速健康检查（只检查连接状态，不发送请求）
func (cv *ConnectionValidator) QuickHealthCheck(conn *UTLSConnection) error {
	if conn == nil {
		return fmt.Errorf("连接为空")
	}

	conn.mu.Lock()
	isHealthy := conn.healthy
	lastUsed := conn.lastUsed
	conn.mu.Unlock()

	if !isHealthy {
		return fmt.Errorf("连接不健康")
	}

	// 检查连接是否超时
	if time.Since(lastUsed) > cv.config.IdleTimeout {
		return fmt.Errorf("连接空闲超时")
	}

	return nil
}
