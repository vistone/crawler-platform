package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	projlogger "crawler-platform/logger"

	"github.com/quic-go/quic-go"
)

// #region agent log helper
func writeDebugLog(id, hypothesisId, location, message string, data map[string]interface{}) {
	logPath := "/home/stone/crawler-platform/.cursor/debug.log"
	logEntry := map[string]interface{}{
		"id":          id,
		"timestamp":   time.Now().UnixMilli(),
		"location":    location,
		"message":     message,
		"data":        data,
		"sessionId":   "debug-session",
		"runId":       "run1",
		"hypothesisId": hypothesisId,
	}
	jsonData, _ := json.Marshal(logEntry)
	f, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if f != nil {
		fmt.Fprintf(f, "%s\n", jsonData)
		f.Close()
	}
}
// #endregion agent log

// TUICClient TUIC协议客户端
type TUICClient struct {
	serverAddr string
	token      string
	conn       *quic.Conn
	ctx        context.Context
	mu         sync.Mutex // 保护连接的并发访问
}

var (
	// 全局连接池：serverAddr -> *quic.Conn
	connectionPool = make(map[string]*quic.Conn)
	poolMu         sync.RWMutex
)

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

// Connect 连接到TUIC服务器（支持连接复用）
// 输出: error - 错误信息
func (c *TUICClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 检查连接池中是否已有可用连接
	poolMu.RLock()
	if existingConn, exists := connectionPool[c.serverAddr]; exists {
		// 检查连接是否仍然有效
		if existingConn != nil {
			// 尝试打开一个测试流来验证连接
			testStream, err := existingConn.OpenStreamSync(context.Background())
			if err == nil {
				testStream.Close()
				c.conn = existingConn
				poolMu.RUnlock()
				projlogger.Debug("复用现有连接: %s", c.serverAddr)
				return nil
			}
			// 连接无效，从池中移除
			delete(connectionPool, c.serverAddr)
		}
	}
	poolMu.RUnlock()

	// 创建新连接
	addr, err := net.ResolveUDPAddr("udp", c.serverAddr)
	if err != nil {
		return fmt.Errorf("解析服务器地址失败: %w", err)
	}

	// 创建UDP连接（不预先连接，让quic.Dial自己管理）
	// 优化UDP缓冲区大小，提高传输效率
	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return fmt.Errorf("创建UDP连接失败: %w", err)
	}
	
	// 设置UDP接收缓冲区大小（提高传输效率）
	// 尝试设置为8MB，如果失败则使用系统默认值
	if err := udpConn.SetReadBuffer(8 * 1024 * 1024); err != nil {
		projlogger.Debug("设置UDP接收缓冲区失败，使用默认值: %v", err)
	}
	// 设置UDP发送缓冲区大小
	if err := udpConn.SetWriteBuffer(8 * 1024 * 1024); err != nil {
		projlogger.Debug("设置UDP发送缓冲区失败，使用默认值: %v", err)
	}

	// 创建TLS配置（跳过证书验证，因为可能使用自签名证书）
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"tuic"},
	}

	// 创建QUIC配置 - 优化高速传输
	quicConfig := &quic.Config{
		// 保持连接活跃，减少重连开销
		KeepAlivePeriod: 5 * time.Second, // 更频繁的keepalive，保持连接活跃
		MaxIdleTimeout:  60 * time.Second, // 增加空闲超时，支持连接复用
		
		// 传输优化
		MaxIncomingStreams:    1000, // 增加最大流数，支持高并发
		MaxIncomingUniStreams: 1000, // 增加单向流数
		
		// 初始数据大小优化
		InitialStreamReceiveWindow:     8 * 1024 * 1024,  // 8MB 接收窗口
		InitialConnectionReceiveWindow: 16 * 1024 * 1024, // 16MB 连接接收窗口
		
		// 允许0-RTT连接，减少握手延迟
		Allow0RTT: true,
	}

	// 建立QUIC连接（使用本地UDP连接和目标地址）
	conn, err := quic.Dial(context.Background(), udpConn, addr, tlsConfig, quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("建立QUIC连接失败: %w", err)
	}

	c.conn = conn
	
	// 将连接加入连接池
	poolMu.Lock()
	connectionPool[c.serverAddr] = conn
	poolMu.Unlock()
	
	projlogger.Debug("已连接到TUIC服务器: %s (已加入连接池)", c.serverAddr)

	return nil
}

// DoRequest 通过TUIC代理发送HTTP请求
// 输入: targetURL - 目标URL, httpReq - HTTP请求
// 输出: *http.Response - HTTP响应, error - 错误信息
func (c *TUICClient) DoRequest(targetURL string, httpReq *http.Request) (*http.Response, error) {
	// #region agent log
	writeDebugLog("log_client_do_request_entry", "H5", "tuic_client.go:DoRequestEntry", "Starting DoRequest", map[string]interface{}{"targetURL": targetURL, "method": httpReq.Method})
	// #endregion agent log
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
	// #region agent log
	headerLen := 4
	if len(tuicReq) < headerLen {
		headerLen = len(tuicReq)
	}
	writeDebugLog("log_client_build_tuic_request", "H5", "tuic_client.go:buildTUICRequest", "Built TUIC request", map[string]interface{}{"targetAddr": targetAddr, "httpReqDataLen": len(httpReqData), "tuicReqLen": len(tuicReq), "tuicReqHeader": fmt.Sprintf("%x", tuicReq[:headerLen])})
	// #endregion agent log

	// 打开QUIC流
	stream, err := c.conn.OpenStreamSync(c.ctx)
	if err != nil {
		return nil, fmt.Errorf("打开QUIC流失败: %w", err)
	}
	defer stream.Close()

	// 发送TUIC请求
	if _, err := stream.Write(tuicReq); err != nil {
		// #region agent log
		writeDebugLog("log_client_write_tuic_fail", "H5", "tuic_client.go:writeTuicReqFail", "Failed to send TUIC request", map[string]interface{}{"error": err.Error()})
		// #endregion agent log
		return nil, fmt.Errorf("发送TUIC请求失败: %w", err)
	}
	// #region agent log
	writeDebugLog("log_client_write_tuic_success", "H5", "tuic_client.go:writeTuicReqSuccess", "Successfully sent TUIC request", map[string]interface{}{})
	// #endregion agent log

	// 先读取CONNECT响应（4字节）
	connectResp := make([]byte, 4)
	if _, err := io.ReadFull(stream, connectResp); err != nil {
		// #region agent log
		writeDebugLog("log_client_read_connect_resp_fail", "H2", "tuic_client.go:readConnectRespFail", "Failed to read CONNECT response", map[string]interface{}{"error": err.Error()})
		// #endregion agent log
		return nil, fmt.Errorf("读取CONNECT响应失败: %w", err)
	}
	
	version := connectResp[0]
	command := connectResp[1]
	// #region agent log
	writeDebugLog("log_client_connect_resp", "H2", "tuic_client.go:connectRespReceived", "Received CONNECT response", map[string]interface{}{"version": version, "command": command})
	// #endregion agent log
	
	if version != 5 {
		return nil, fmt.Errorf("不支持的TUIC版本: %d", version)
	}
	
	if command == 2 {
		// 错误响应
		return nil, fmt.Errorf("CONNECT失败")
	}
	
	if command != 0 {
		return nil, fmt.Errorf("意外的CONNECT响应命令: %d", command)
	}
	
	// CONNECT成功，现在读取HTTP响应数据（PACKET格式）
	var responseData []byte
	buffer := make([]byte, 32*1024)
	
	for {
		// 读取TUIC协议头（必须完整读取4字节）
		header := make([]byte, 4)
		if _, err := io.ReadFull(stream, header); err != nil {
			if err == io.EOF {
				// 流已关闭，停止读取
				break
			}
			// #region agent log
			writeDebugLog("log_client_read_packet_header_fail", "H2", "tuic_client.go:readPacketHeaderFail", "Failed to read packet header", map[string]interface{}{"error": err.Error()})
			// #endregion agent log
			return nil, fmt.Errorf("读取响应头失败: %w", err)
		}
		
		respVersion := header[0]
		respCommand := header[1]
		dataLen := binary.BigEndian.Uint16(header[2:4])
		// #region agent log
		writeDebugLog("log_client_read_packet_header", "H2", "tuic_client.go:readPacketHeader", "Read response packet header", map[string]interface{}{"respVersion": respVersion, "respCommand": respCommand, "dataLen": dataLen})
		// #endregion agent log
		
		if respVersion != 5 {
			return nil, fmt.Errorf("不支持的TUIC版本: %d", respVersion)
		}
		
		if respCommand == 1 { // PACKET命令，包含HTTP响应数据
			if dataLen == 0 {
				// 空数据包，继续读取下一个
				continue
			}
			if dataLen > uint16(len(buffer)) {
				return nil, fmt.Errorf("响应数据包太大: %d", dataLen)
			}
			
			data := buffer[:dataLen]
			if _, err := io.ReadFull(stream, data); err != nil {
				if err == io.EOF {
					// 流已关闭，但可能已经读取了部分数据
					if len(data) > 0 {
						responseData = append(responseData, data...)
					}
					break
				}
				// #region agent log
				writeDebugLog("log_client_read_packet_data_fail", "H2", "tuic_client.go:readPacketDataFail", "Failed to read packet data", map[string]interface{}{"error": err.Error(), "expectedLen": dataLen})
				// #endregion agent log
				return nil, fmt.Errorf("读取响应数据失败: %w", err)
			}
			// #region agent log
			writeDebugLog("log_client_read_packet_data", "H2", "tuic_client.go:readPacketData", "Read response packet data", map[string]interface{}{"dataLen": len(data), "totalResponseLen": len(responseData) + len(data)})
			// #endregion agent log
			
			// 累积响应数据
			responseData = append(responseData, data...)
		} else {
			// 其他命令，停止读取
			projlogger.Debug("收到非PACKET命令: %d，停止读取", respCommand)
			break
		}
	}
	
	// 注意：不需要关闭写入端，读取完响应后会自动处理

	// 解析HTTP响应
	if len(responseData) == 0 {
		return nil, fmt.Errorf("收到空响应")
	}
	
	httpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(responseData)), httpReq)
	if err != nil {
		// #region agent log
		writeDebugLog("log_client_parse_http_resp_fail", "H2", "tuic_client.go:parseHttpRespFail", "Failed to parse HTTP response", map[string]interface{}{"error": err.Error(), "responseDataLen": len(responseData)})
		// #endregion agent log
		return nil, fmt.Errorf("解析HTTP响应失败: %w", err)
	}
	// #region agent log
	writeDebugLog("log_client_parse_http_resp_success", "H2", "tuic_client.go:parseHttpRespSuccess", "Successfully parsed HTTP response", map[string]interface{}{"statusCode": httpResp.StatusCode, "contentLength": httpResp.ContentLength})
	// #endregion agent log

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

// Close 关闭连接（从连接池中移除，但不立即关闭，支持复用）
func (c *TUICClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if c.conn != nil {
		// 从连接池中移除，但保留连接以便其他请求复用
		// 注意：这里不关闭连接，让连接在空闲超时后自动关闭
		poolMu.Lock()
		if connectionPool[c.serverAddr] == c.conn {
			// 不删除，保留在池中以便复用
			// 连接会在MaxIdleTimeout后自动关闭
		}
		poolMu.Unlock()
		
		c.conn = nil
		projlogger.Debug("客户端连接已释放（连接保留在池中以便复用）")
	}
	return nil
}

// CloseAllConnections 关闭所有连接池中的连接（用于程序退出时）
func CloseAllConnections() {
	poolMu.Lock()
	defer poolMu.Unlock()
	
	for addr, conn := range connectionPool {
		if conn != nil {
			conn.CloseWithError(0, "程序退出")
			projlogger.Debug("关闭连接池中的连接: %s", addr)
		}
	}
	connectionPool = make(map[string]*quic.Conn)
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
