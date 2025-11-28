package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	projlogger "crawler-platform/logger"

	"github.com/quic-go/quic-go"
)

// TUICClient TUIC协议客户端
type TUICClient struct {
	serverAddr string
	token      string
	conn       *quic.Conn
	ctx        context.Context
}

// NewTUICClient 创建新的TUIC客户端
// 输入: serverAddr - 服务器地址 (host:port), token - 认证令牌
// 输出: *TUICClient - 客户端实例, error - 错误信息
func NewTUICClient(serverAddr, token string) (*TUICClient, error) {
	if serverAddr == "" {
		return nil, fmt.Errorf("服务器地址不能为空")
	}
	if token == "" {
		return nil, fmt.Errorf("认证令牌不能为空")
	}

	return &TUICClient{
		serverAddr: serverAddr,
		token:      token,
		ctx:        context.Background(),
	}, nil
}

// Connect 连接到TUIC服务器
// 输出: error - 错误信息
func (c *TUICClient) Connect() error {
	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp", c.serverAddr)
	if err != nil {
		return fmt.Errorf("解析服务器地址失败: %w", err)
	}

	// 创建UDP连接
	udpConn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("创建UDP连接失败: %w", err)
	}

	// 创建TLS配置（跳过证书验证，因为可能使用自签名证书）
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"tuic"},
	}

	// 创建QUIC配置
	quicConfig := &quic.Config{
		KeepAlivePeriod: 10 * time.Second,
		MaxIdleTimeout:  30 * time.Second,
	}

	// 建立QUIC连接
	conn, err := quic.Dial(context.Background(), udpConn, addr, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("建立QUIC连接失败: %w", err)
	}

	c.conn = conn
	projlogger.Debug("已连接到TUIC服务器: %s", c.serverAddr)

	return nil
}

// DoRequest 通过TUIC代理发送HTTP请求
// 输入: targetURL - 目标URL, httpReq - HTTP请求
// 输出: *http.Response - HTTP响应, error - 错误信息
func (c *TUICClient) DoRequest(targetURL string, httpReq *http.Request) (*http.Response, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("未连接到服务器，请先调用Connect()")
	}

	// 解析目标URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("解析目标URL失败: %w", err)
	}

	// 获取目标主机和端口
	targetHost := parsedURL.Hostname()
	targetPort := parsedURL.Port()
	if targetPort == "" {
		if parsedURL.Scheme == "https" {
			targetPort = "443"
		} else {
			targetPort = "80"
		}
	}
	targetAddr := fmt.Sprintf("%s:%s", targetHost, targetPort)

	// 序列化HTTP请求
	var httpReqBuf bytes.Buffer
	if err := httpReq.Write(&httpReqBuf); err != nil {
		return nil, fmt.Errorf("序列化HTTP请求失败: %w", err)
	}
	httpReqData := httpReqBuf.Bytes()

	// 构建TUIC协议请求
	tuicReq, err := c.buildTUICRequest(targetAddr, httpReqData)
	if err != nil {
		return nil, fmt.Errorf("构建TUIC请求失败: %w", err)
	}

	// 打开QUIC流
	stream, err := c.conn.OpenStreamSync(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("打开QUIC流失败: %w", err)
	}
	defer stream.Close()

	// 发送TUIC请求
	if _, err := stream.Write(tuicReq); err != nil {
		return nil, fmt.Errorf("发送TUIC请求失败: %w", err)
	}

	// 关闭写入端
	if err := stream.Close(); err != nil {
		return nil, fmt.Errorf("关闭流写入端失败: %w", err)
	}

	// 读取响应
	responseData, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析HTTP响应
	httpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(responseData)), httpReq)
	if err != nil {
		return nil, fmt.Errorf("解析HTTP响应失败: %w", err)
	}

	return httpResp, nil
}

// buildTUICRequest 构建TUIC v5协议请求
// 输入: targetAddr - 目标地址 (host:port), httpReqData - HTTP请求数据
// 输出: []byte - TUIC协议数据, error - 错误信息
func (c *TUICClient) buildTUICRequest(targetAddr string, httpReqData []byte) ([]byte, error) {
	// 解析目标地址
	host, portStr, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("解析目标地址失败: %w", err)
	}

	// 解析端口
	var port uint16
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return nil, fmt.Errorf("解析端口失败: %w", err)
	}

	// 构建payload: [地址类型(1字节)][地址长度(1字节)][地址][端口(2字节)][HTTP请求数据]
	hostBytes := []byte(host)
	addrType := byte(0) // 0 = 域名, 1 = IPv4, 2 = IPv6
	addrLen := byte(len(hostBytes))

	payload := make([]byte, 0, 1+1+len(hostBytes)+2+len(httpReqData))
	payload = append(payload, addrType)
	payload = append(payload, addrLen)
	payload = append(payload, hostBytes...)

	// 添加端口（大端序）
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, port)
	payload = append(payload, portBytes...)

	// 添加HTTP请求数据
	payload = append(payload, httpReqData...)

	// 构建TUIC v5协议头: [版本(1字节)][命令(1字节)][长度(2字节)][payload]
	version := byte(5)
	command := byte(0) // 0 = CONNECT命令
	payloadLen := uint16(len(payload))

	request := make([]byte, 0, 4+len(payload))
	request = append(request, version)
	request = append(request, command)

	// 添加长度（大端序）
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, payloadLen)
	request = append(request, lenBytes...)

	// 添加payload
	request = append(request, payload...)

	return request, nil
}

// Close 关闭连接
func (c *TUICClient) Close() error {
	if c.conn != nil {
		if err := c.conn.CloseWithError(0, "客户端关闭"); err != nil {
			return err
		}
		c.conn = nil
	}
	return nil
}

// tuicMain TUIC客户端主函数
// 输入: proxyAddr - 代理服务器地址, token - 认证令牌, targetURL - 目标URL, method - HTTP方法, timeout - 超时时间
func tuicMain(proxyAddr, token, targetURL, method string, timeout time.Duration) {

	// 验证必需参数
	if proxyAddr == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须提供代理服务器地址\n")
		os.Exit(1)
	}
	if token == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须提供TUIC认证令牌\n")
		os.Exit(1)
	}
	if targetURL == "" {
		fmt.Fprintf(os.Stderr, "错误: 必须提供目标URL\n")
		os.Exit(1)
	}

	// 初始化日志
	projlogger.SetGlobalLogger(&projlogger.DefaultLogger{})

	// 创建TUIC客户端
	client, err := NewTUICClient(proxyAddr, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建TUIC客户端失败: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// 连接到服务器
	if err := client.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "连接服务器失败: %v\n", err)
		os.Exit(1)
	}

	// 创建HTTP请求
	httpReq, err := http.NewRequest(method, targetURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建HTTP请求失败: %v\n", err)
		os.Exit(1)
	}

	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	httpReq = httpReq.WithContext(ctx)

	// 发送请求
	projlogger.Info("通过TUIC代理发送请求: %s %s", method, targetURL)
	httpResp, err := client.DoRequest(targetURL, httpReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
		os.Exit(1)
	}
	defer httpResp.Body.Close()

	// 显示响应
	fmt.Printf("状态: %s\n", httpResp.Status)
	fmt.Printf("状态码: %d\n", httpResp.StatusCode)

	// 读取响应体
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取响应体失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("响应体长度: %d 字节\n", len(body))
	if len(body) > 0 && len(body) <= 200 {
		fmt.Printf("响应体内容: %q\n", string(body))
	} else if len(body) > 200 {
		fmt.Printf("响应体前200字节: %q\n", string(body[:200]))
	}
}
