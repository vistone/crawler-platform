package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/json/badoption"
	M "github.com/sagernet/sing/common/metadata"
)

// SingBoxTUICClient 使用 sing-box 的真正的 TUIC 客户端实现
type SingBoxTUICClient struct {
	serverAddr string
	uuid       string
	password   string
	box        *box.Box
	httpClient *http.Client
}

// NewSingBoxTUICClient 创建新的 sing-box TUIC 客户端
func NewSingBoxTUICClient(serverAddr, uuid, password string) (*SingBoxTUICClient, error) {
	// 解析服务器地址
	host, port, err := net.SplitHostPort(serverAddr)
	if err != nil {
		return nil, fmt.Errorf("解析服务器地址失败: %w", err)
	}

	// 构建 TUIC outbound 配置
	tuicOutboundOptions := option.TUICOutboundOptions{
		ServerOptions: option.ServerOptions{
			Server:     host,
			ServerPort: parsePortFromString(port),
		},
		UUID:              uuid,
		Password:          password,
		CongestionControl: "bbr",
		ZeroRTTHandshake:  true,                                 // 启用 ZeroRTT 以提升性能（减少首次连接延迟）
		Heartbeat:         badoption.Duration(30 * time.Second), // 增加心跳间隔（减少开销，QUIC连接更稳定）
	}
	// 设置 TLS 配置（通过 OutboundTLSOptionsContainer）
	// 注意：TUIC 协议需要 ALPN 配置，必须与服务器端匹配（通常是 "h3"）
	// 如果连接到 IP 地址，证书可能不匹配，需要设置 Insecure 或使用正确的 ServerName
	tlsOptions := &option.OutboundTLSOptions{
		Enabled:    true,
		ServerName: host, // 使用服务器地址作为 ServerName
		Insecure:   true, // 如果证书是为 localhost/127.0.0.1 签发的，连接到 IP 时需要跳过验证
		// 生产环境建议：使用域名并配置正确的证书，然后设置 Insecure: false
		ALPN: []string{"h3"}, // TUIC 协议使用 HTTP/3 ALPN
	}
	tuicOutboundOptions.ReplaceOutboundTLSOptions(tlsOptions)

	// 构建 sing-box 配置
	options := option.Options{
		Log: &option.LogOptions{
			Level: "error", // 设置为 error 级别，禁用 INFO 级别的连接日志输出
		},
		Outbounds: []option.Outbound{
			{
				Type:    "tuic",
				Tag:     "tuic-out",
				Options: &tuicOutboundOptions, // 传递指针
			},
			{
				Type: "direct",
				Tag:  "direct-out",
			},
		},
		// 注意：路由配置是可选的，如果不配置，默认会使用第一个 outbound (tuic-out)
	}

	// 创建 sing-box 实例
	// 使用 include 包来创建所有必要的 registry（会自动注册所有协议，包括 TUIC）
	ctx := context.Background()
	ctx = include.Context(ctx)

	instance, err := box.New(box.Options{
		Context: ctx,
		Options: options,
	})
	if err != nil {
		// 检查是否是 QUIC 相关的错误
		errStr := err.Error()
		if strings.Contains(errStr, "QUIC is not included") || strings.Contains(errStr, "with_quic") {
			return nil, fmt.Errorf("创建 sing-box 实例失败: %w\n\n提示: 必须使用 -tags with_quic 构建标签编译客户端:\n  go build -tags with_quic -o grpcclient ./cmd/grpcclient", err)
		}
		return nil, E.Cause(err, "创建 sing-box 实例失败")
	}

	err = instance.Start()
	if err != nil {
		return nil, E.Cause(err, "启动 sing-box 失败")
	}

	// 获取 sing-box 的 outbound manager，用于通过 TUIC 协议发送请求
	outboundManager := instance.Outbound()

	// 获取 TUIC outbound（通过 tag 查找）
	tuicOutbound, found := outboundManager.Outbound("tuic-out")
	if !found {
		return nil, fmt.Errorf("无法找到 TUIC outbound (tag: tuic-out)")
	}

	// 创建自定义 DialContext，使用 sing-box 的 TUIC outbound 进行连接
	// 关键：sing-box的TUIC outbound内部会自动复用QUIC连接，我们只需要确保HTTP Transport的连接池配置正确
	// sing-box的DialContext会自动复用相同目标的QUIC连接，这是QUIC协议的特性
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 只处理 TCP 网络
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil, fmt.Errorf("不支持的网络类型: %s (仅支持 tcp)", network)
		}

		// 解析地址为 Socksaddr
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("解析地址失败: %w", err)
		}

		// 解析端口
		var port uint16
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			return nil, fmt.Errorf("解析端口失败: %w", err)
		}

		// 创建 Socksaddr（使用 ParseSocksaddrHostPort 方法）
		destination := M.ParseSocksaddrHostPort(host, port)

		// 验证地址是否匹配 TUIC 服务器地址
		currentAddr := net.JoinHostPort(host, portStr)
		if currentAddr != serverAddr {
			// 这不应该发生，因为 HTTP 请求的目标就是 TUIC 服务器
			fmt.Printf("⚠️  警告: DialContext 地址 %s 与 TUIC 服务器地址 %s 不匹配\n", currentAddr, serverAddr)
		}

		// 使用 TUIC outbound 进行连接
		// sing-box的TUIC outbound内部会自动复用QUIC连接：
		// - 相同目标地址的多个DialContext调用会复用同一个QUIC连接
		// - QUIC协议支持多路复用，多个TCP流可以共享同一个QUIC连接
		// - 这是QUIC协议的核心特性，sing-box已经实现了这个机制
		conn, err := tuicOutbound.DialContext(ctx, "tcp", destination)
		if err != nil {
			return nil, fmt.Errorf("通过 TUIC 协议连接到 %s 失败: %w", addr, err)
		}

		// 返回连接，sing-box会自动管理QUIC连接的复用
		return conn, nil
	}

	// 配置优化的 HTTP Transport，使用 sing-box 的 dialer
	// 关键优化策略：
	// 1. sing-box的TUIC outbound内部会自动复用QUIC连接（相同目标地址复用同一个QUIC连接）
	// 2. HTTP Transport的连接池管理TCP流层面的复用（多个HTTP请求复用同一个TCP连接）
	// 3. 两层复用机制协同工作：QUIC连接复用（sing-box管理）+ TCP流复用（HTTP Transport管理）
	transport := &http.Transport{
		DialContext:           dialContext,       // 使用 TUIC 协议的 dialer（sing-box会自动复用QUIC连接）
		MaxIdleConns:          1000,              // 大幅增加最大空闲连接数（充分利用QUIC连接复用）
		MaxIdleConnsPerHost:   500,               // 大幅增加每个主机最大空闲连接数（高并发场景）
		MaxConnsPerHost:       0,                 // 0 表示不限制（QUIC支持大量并发流，不限制TCP流数量）
		IdleConnTimeout:       600 * time.Second, // 大幅增加空闲连接超时（QUIC连接可以保持更久，减少重建开销）
		DisableKeepAlives:     false,             // 启用 Keep-Alive（TCP流复用）
		DisableCompression:    true,              // 禁用压缩（减少CPU开销，QUIC层已优化）
		ForceAttemptHTTP2:     false,             // HTTP/1.1 即可（通过 TUIC/QUIC 时不需要 HTTP/2）
		WriteBufferSize:       128 * 1024,        // 增加写缓冲区大小（128KB，提升吞吐量）
		ReadBufferSize:        128 * 1024,        // 增加读缓冲区大小（128KB，提升吞吐量）
		ResponseHeaderTimeout: 10 * time.Second,  // 响应头超时（快速失败）
		ExpectContinueTimeout: 1 * time.Second,   // Expect: 100-continue 超时
		TLSHandshakeTimeout:   5 * time.Second,   // TLS握手超时（虽然通过QUIC，但设置以防万一）
	}

	return &SingBoxTUICClient{
		serverAddr: serverAddr,
		uuid:       uuid,
		password:   password,
		box:        instance,
		httpClient: &http.Client{
			Timeout:   15 * time.Second, // 减少超时时间（从30秒减少到15秒，快速失败）
			Transport: transport,
		},
	}, nil
}

// parsePortFromString 从字符串解析端口号
func parsePortFromString(portStr string) uint16 {
	var port uint16
	fmt.Sscanf(portStr, "%d", &port)
	if port == 0 {
		port = 8443
	}
	return port
}

// SubmitTask 提交任务请求
// 注意：现在 HTTP 请求会通过 TUIC 协议传输（通过自定义 DialContext）
// HTTP 请求发送到 TUIC 服务器，然后 TUIC 服务器转发到目标服务器
func (c *SingBoxTUICClient) SubmitTask(ctx context.Context, req *tasksmanager.TaskRequest) (*tasksmanager.TaskResponse, error) {
	// HTTP 请求会通过 TUIC 协议传输（通过自定义 DialContext）
	// 请求路径：客户端 → TUIC 协议 → TUIC 服务器 → 目标服务器
	url := "http://" + c.serverAddr + "/task/submit"

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回错误状态码: %d, 响应: %s", resp.StatusCode, string(respBody))
	}

	var taskResp tasksmanager.TaskResponse
	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &taskResp, nil
}

// RegisterClient 客户端注册
func (c *SingBoxTUICClient) RegisterClient(ctx context.Context, clientInfo *tasksmanager.TaskClientInfo) (*tasksmanager.RegisterClientResponse, error) {
	return &tasksmanager.RegisterClientResponse{
		Success: true,
		Message: "TUIC 模式：客户端注册已跳过（sing-box TUIC 模式）",
	}, nil
}

// ClientHeartbeat 客户端心跳
func (c *SingBoxTUICClient) ClientHeartbeat(ctx context.Context, clientInfo *tasksmanager.TaskClientInfo) (*tasksmanager.ClientHeartbeatResponse, error) {
	return &tasksmanager.ClientHeartbeatResponse{
		Success: true,
	}, nil
}

// GetTaskClientInfoList 获取任务客户端列表
func (c *SingBoxTUICClient) GetTaskClientInfoList(ctx context.Context, req *tasksmanager.TaskClientInfoListRequest) (*tasksmanager.TaskClientInfoListResponse, error) {
	return &tasksmanager.TaskClientInfoListResponse{
		Items: []*tasksmanager.TaskClientInfo{},
	}, nil
}

// GetGrpcServerNodeInfoList 获取 gRPC 服务器节点列表
func (c *SingBoxTUICClient) GetGrpcServerNodeInfoList(ctx context.Context, req *tasksmanager.GrpcServerNodeInfoListRequest) (*tasksmanager.GrpcServerNodeInfoListResponse, error) {
	return &tasksmanager.GrpcServerNodeInfoListResponse{
		Items: []*tasksmanager.GrpcServerNodeInfo{},
	}, nil
}

// Close 关闭客户端
func (c *SingBoxTUICClient) Close() error {
	if c.box != nil {
		c.box.Close()
	}
	return nil
}
