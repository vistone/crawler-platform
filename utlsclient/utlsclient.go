package utlsclient

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	projlogger "crawler-platform/logger"

	utls "github.com/refraction-networking/utls"

	"golang.org/x/net/http2"
)

// IsConnectionError 检查错误是否是连接错误（导出以供测试使用）
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	for _, keyword := range ConnectionErrorKeywords {
		if strings.Contains(errStr, keyword) {
			return true
		}
	}
	// 检查是否是预定义的连接错误
	return errors.Is(err, ErrConnectionBroken) || errors.Is(err, ErrConnectionClosed)
}

// UTLSClient 是一个基于 uTLS 的 HTTP 客户端，用于模拟真实浏览器
type UTLSClient struct {
	conn       *UTLSConnection
	timeout    time.Duration
	userAgent  string
	maxRetries int
}

// NewUTLSClient 创建新的 UTLS 客户端
func NewUTLSClient(conn *UTLSConnection) *UTLSClient {
	return &UTLSClient{
		conn:       conn,
		timeout:    30 * time.Second,
		userAgent:  conn.fingerprint.UserAgent,
		maxRetries: 3,
	}
}

// SetTimeout 设置请求超时时间
func (c *UTLSClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
}

// SetUserAgent 设置 User-Agent
func (c *UTLSClient) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

// SetMaxRetries 设置最大重试次数
func (c *UTLSClient) SetMaxRetries(maxRetries int) {
	c.maxRetries = maxRetries
}

// SetDebug 设置调试模式（兼容旧接口）
// 注意：此方法已废弃，请使用全局日志系统 SetGlobalLogger 来控制日志输出
func (c *UTLSClient) SetDebug(debug bool) {
	// 为了兼容性，如果启用调试模式，设置全局日志为默认日志记录器
	if debug {
		projlogger.SetGlobalLogger(&projlogger.DefaultLogger{})
	}
	// 如果禁用调试模式，不做任何操作，使用当前的全局日志设置
}

// Do 执行 HTTP 请求
func (c *UTLSClient) Do(req *http.Request) (*http.Response, error) {
	return c.DoWithContext(context.Background(), req)
}

// DoWithContext 带上下文的 HTTP 请求
func (c *UTLSClient) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 设置请求头
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	// 设置Accept-Language（如果没有设置）
	if req.Header.Get("Accept-Language") == "" && c.conn.acceptLanguage != "" {
		req.Header.Set("Accept-Language", c.conn.acceptLanguage)
	}
	// Host 优先由调用方设定；若为空则从热连接参数传递（仅参数注入，不做连接管理）
	if req.Host == "" {
		if h := c.conn.TargetHost(); h != "" {
			req.Host = h
		} else if req.URL != nil && req.URL.Host != "" {
			req.Host = req.URL.Host
		}
	}

	var lastErr error
	for i := 0; i <= c.maxRetries; i++ {
		if i > 0 {
			projlogger.Debug("请求重试 %d/%d", i, c.maxRetries)
			time.Sleep(time.Duration(i) * DefaultRetryDelay)
		}

		resp, err := c.doRequest(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("%w: %v", ErrMaxRetriesExceeded, lastErr)
}

// doRequest 实际执行请求
func (c *UTLSClient) doRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 创建带超时的上下文
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	// 检测协商的协议
	negotiatedProto := c.conn.tlsConn.ConnectionState().NegotiatedProtocol
	projlogger.Debug("使用协议: %s", negotiatedProto)

	// 如果协商了 HTTP/2，使用 http2.Transport
	if negotiatedProto == "h2" {
		return c.doHTTP2Request(ctx, req)
	}

	// 否则使用 HTTP/1.1
	return c.doHTTP1Request(ctx, req)
}

// doHTTP2Request 使用 HTTP/2 执行请求
func (c *UTLSClient) doHTTP2Request(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 获取或创建 HTTP/2 客户端连接
	c.conn.h2Mu.Lock()
	var cc *http2.ClientConn
	if c.conn.h2ClientConn != nil {
		cc = c.conn.h2ClientConn.(*http2.ClientConn)
		// 检查连接是否可用
		if !cc.CanTakeNewRequest() {
			cc.Close()
			c.conn.h2ClientConn = nil
			cc = nil
		}
	}

	if cc == nil {
		t := &http2.Transport{}
		var err error
		cc, err = t.NewClientConn(c.conn.tlsConn)
		if err != nil {
			c.conn.h2Mu.Unlock()
			return nil, fmt.Errorf("创建HTTP/2连接失败: %w", err)
		}
		c.conn.h2ClientConn = cc
		projlogger.Debug("创建新的HTTP/2客户端连接")
	} else {
		projlogger.Debug("复用现有HTTP/2客户端连接")
	}
	c.conn.h2Mu.Unlock()

	projlogger.Debug("发送HTTP/2请求: %s %s", req.Method, req.URL.String())

	// 执行请求
	resp, err := cc.RoundTrip(req)
	if err != nil {
		// 如果请求失败，标记连接为需要重建
		c.conn.h2Mu.Lock()
		if c.conn.h2ClientConn == cc {
			cc.Close()
			c.conn.h2ClientConn = nil
		}
		c.conn.h2Mu.Unlock()
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	return resp, nil
}

// doHTTP1Request 使用 HTTP/1.1 执行请求
func (c *UTLSClient) doHTTP1Request(ctx context.Context, req *http.Request) (*http.Response, error) {
	// 构建原始 HTTP 请求
	rawReq, err := c.buildRawRequest(req)
	if err != nil {
		return nil, fmt.Errorf("构建原始请求失败: %w", err)
	}

	projlogger.Debug("发送请求:\n%s", rawReq)

	// 通过连接的 RoundTripRaw 执行实际传输（不在此处理连接细节）
	reader, err := c.conn.RoundTripRaw(ctx, []byte(rawReq))
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 读取响应
	resp, err := c.readResponse(ctx, req, reader)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	return resp, nil
}

// buildRawRequest 构建原始 HTTP 请求
func (c *UTLSClient) buildRawRequest(req *http.Request) (string, error) {
	var buf bytes.Buffer

	// 请求行
	buf.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, req.URL.RequestURI()))

	// Host 头
	buf.WriteString(fmt.Sprintf("Host: %s\r\n", req.Host))

	// 其他头部
	for key, values := range req.Header {
		for _, value := range values {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	// Content-Length 如果有 body
	if req.Body != nil {
		buf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", req.ContentLength))
	}

	// Connection 头
	if req.Header.Get("Connection") == "" {
		buf.WriteString("Connection: keep-alive\r\n")
	}

	// 空行
	buf.WriteString("\r\n")

	// Body
	if req.Body != nil {
		defer req.Body.Close()
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return "", fmt.Errorf("读取请求体失败: %w", err)
		}
		buf.Write(body)
	}

	return buf.String(), nil
}

// readResponse 读取 HTTP 响应
func (c *UTLSClient) readResponse(ctx context.Context, req *http.Request, r io.Reader) (*http.Response, error) {
	// 创建 reader（由传输层提供的底层连接读端）
	reader := bufio.NewReader(r)

	// 读取状态行
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("读取状态行失败: %w", err)
	}

	// 解析状态行
	parts := strings.Split(strings.TrimSpace(statusLine), " ")
	if len(parts) < 3 {
		return nil, fmt.Errorf("无效的状态行: %s", statusLine)
	}

	statusCode := parts[1]
	statusText := strings.Join(parts[2:], " ")

	// 创建响应对象
	resp := &http.Response{
		Status:     fmt.Sprintf("%s %s", statusCode, statusText),
		StatusCode: 0, // 稍后设置
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Request:    req,
	}

	// 解析状态码
	var n int
	n, err = fmt.Sscanf(statusCode, "%d", &resp.StatusCode)
	if err != nil || n != 1 {
		return nil, fmt.Errorf("解析状态码失败: %s", statusCode)
	}

	// 读取头部
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("读取头部失败: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break // 头部结束
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue // 忽略无效头部
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		resp.Header.Add(key, value)
	}

	// 处理响应体
	resp.Body = &responseBody{
		reader: reader,
		conn:   c.conn.tlsConn,
	}

	projlogger.Debug("收到响应: %s", resp.Status)
	for key, values := range resp.Header {
		projlogger.Debug("  %s: %s", key, strings.Join(values, ", "))
	}

	return resp, nil
}

// responseBody 响应体包装器
type responseBody struct {
	reader *bufio.Reader
	conn   *utls.UConn
	closed bool
	mu     sync.Mutex
}

func (rb *responseBody) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return 0, io.EOF
	}

	return rb.reader.Read(p)
}

func (rb *responseBody) Close() error {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return nil
	}

	rb.closed = true
	// 不关闭连接，让它保持 keep-alive
	return nil
}

// Get 快捷方法：执行 GET 请求
func (c *UTLSClient) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post 快捷方法：执行 POST 请求
func (c *UTLSClient) Post(url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// Head 快捷方法：执行 HEAD 请求
func (c *UTLSClient) Head(url string) (*http.Response, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}
