package grpcserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"
	"crawler-platform/utlsclient"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Server gRPC 服务器结构
type Server struct {
	tasksmanager.UnimplementedTasksManagerServer

	// 服务器配置
	address string
	port    string
	nodeID  string

	// 数据存储
	clients   map[string]*tasksmanager.TaskClientInfo // 客户端信息映射
	clientsMu sync.RWMutex

	nodes   map[string]*tasksmanager.GrpcServerNodeInfo // 节点信息映射
	nodesMu sync.RWMutex

	tasks   map[string]*tasksmanager.TaskRequest // 任务映射
	tasksMu sync.RWMutex

	messages   map[string]*tasksmanager.NodeMessage // 消息队列
	messagesMu sync.RWMutex

	// 任务长度统计
	totalTaskLength int64 // 累计接收到的任务总长度（字节）
	taskLengthMu    sync.Mutex

	// 记录节点注册时间，用于判断新节点
	nodeRegisterTimes  map[string]time.Time       // 节点UUID -> 注册时间
	lastHeartbeatNodes map[string]map[string]bool // 客户端UUID -> 已知节点UUID集合（用于返回新节点）

	// gRPC 服务器实例
	grpcServer *grpc.Server

	// 节点连接管理器（用于自动发现和连接其他节点）
	nodeConnector *NodeConnector

	// TLS 配置
	tlsConfig *tls.Config

	// 日志记录器
	logger logger.Logger

	// 本地 IP 池（可选，用于 IP 地址管理）
	ipPool IPPoolInterface

	// UTLS 客户端（基于 utlsclient.Client，用于执行下载任务）
	utlsClient *utlsclient.Client

	// 任务执行配置（用于构建 URL 和选择热连接池）
	// 这些配置字段从 config.go 中的 Config 结构体传递过来
	rockTreeDataEnable           bool
	rockTreeDataHostName         string
	rockTreeDataBulkMetadataPath string
	rockTreeDataNodeDataPath     string
	rockTreeDataImageryDataPath  string

	googleEarthDesktopDataEnable      bool
	googleEarthDesktopDataHostName    string
	googleEarthDesktopDataQ2Path      string
	googleEarthDesktopDataImageryPath string
	googleEarthDesktopDataTerrainPath string

	// TUIC 服务器配置（用于 GetTUICConfig RPC）
	tuicEnabled    bool
	tuicAddress    string
	tuicPort       string
	tuicUUID       string
	tuicPassword   string
	tuicCongestion string
	tuicConfigMu   sync.RWMutex
}

// IPPoolInterface 定义 IP 池接口（与 localippool.IPPool 接口兼容，避免循环导入）
type IPPoolInterface interface {
	GetIP() net.IP
	ReleaseIP(ip net.IP)
	MarkIPUnused(ip net.IP)
	SetTargetIPCount(count int)
	Close() error
}

// NewServer 创建新的 gRPC 服务器实例
func NewServer(address, port string) *Server {
	return NewServerWithTLS(address, port, nil)
}

// NewServerWithTLS 创建新的 gRPC 服务器实例（带 TLS 配置）
func NewServerWithTLS(address, port string, tlsConfig *tls.Config) *Server {
	nodeID := generateNodeID()

	s := &Server{
		address:            address,
		port:               port,
		nodeID:             nodeID,
		clients:            make(map[string]*tasksmanager.TaskClientInfo),
		nodes:              make(map[string]*tasksmanager.GrpcServerNodeInfo),
		tasks:              make(map[string]*tasksmanager.TaskRequest),
		messages:           make(map[string]*tasksmanager.NodeMessage),
		nodeRegisterTimes:  make(map[string]time.Time),
		lastHeartbeatNodes: make(map[string]map[string]bool),
		tlsConfig:          tlsConfig,
		logger:             logger.GetGlobalLogger(),
	}

	// 注册自己为节点
	s.registerSelfAsNode()

	// 创建节点连接管理器
	s.nodeConnector = NewNodeConnector(s.nodeID, s.address, s.port)
	s.nodeConnector.Start()

	return s
}

// SetBootstrapNodes 设置引导节点地址
// 服务器启动时连接到这些节点以发现网络中的其他节点
func (s *Server) SetBootstrapNodes(addresses []string) {
	if s.nodeConnector != nil && len(addresses) > 0 {
		go func() {
			// 等待一小段时间确保服务器已启动
			time.Sleep(1 * time.Second)
			if err := s.nodeConnector.Bootstrap(addresses); err != nil {
				s.logger.Warn("引导连接失败: %v", err)
			}
		}()
	}
}

// SetTUICConfig 设置 TUIC 服务器配置（用于 GetTUICConfig RPC）
func (s *Server) SetTUICConfig(enabled bool, address, port, uuid, password, congestion string) {
	s.tuicConfigMu.Lock()
	defer s.tuicConfigMu.Unlock()
	s.tuicEnabled = enabled
	s.tuicAddress = address
	s.tuicPort = port
	s.tuicUUID = uuid
	s.tuicPassword = password
	s.tuicCongestion = congestion
}

// GetTUICConfig 获取 TUIC 服务器配置
func (s *Server) GetTUICConfig(ctx context.Context, req *tasksmanager.TUICConfigRequest) (*tasksmanager.TUICConfigResponse, error) {
	s.tuicConfigMu.RLock()
	defer s.tuicConfigMu.RUnlock()

	if !s.tuicEnabled {
		return &tasksmanager.TUICConfigResponse{
			Success: true,
			Message: "TUIC 服务器未启用",
			Enabled: false,
		}, nil
	}

	return &tasksmanager.TUICConfigResponse{
		Success:    true,
		Message:    "TUIC 配置获取成功",
		Enabled:    true,
		Address:    s.tuicAddress,
		Port:       s.tuicPort,
		Uuid:       s.tuicUUID,
		Password:   s.tuicPassword,
		Congestion: s.tuicCongestion,
	}, nil
}

// SetIPPool 设置本地 IP 池
// 输入: ipPool - IP 池接口实例
func (s *Server) SetIPPool(ipPool IPPoolInterface) {
	s.ipPool = ipPool
	if ipPool != nil {
		s.logger.Info("本地 IP 池已设置到服务器")
	}
}

// GetIPPool 获取本地 IP 池
// 输出: IPPoolInterface - IP 池接口实例（可能为 nil）
func (s *Server) GetIPPool() IPPoolInterface {
	return s.ipPool
}

// SetUTLSClient 设置 UTLS 客户端（用于通过 utlsclient.Client 执行任务）
func (s *Server) SetUTLSClient(client *utlsclient.Client) {
	s.utlsClient = client
}

// SetRockTreeDataConfig 设置 RockTree 数据配置（从 config.go 的 Config 结构体传递）
func (s *Server) SetRockTreeDataConfig(enable bool, hostName, bulkMetadataPath, nodeDataPath, imageryDataPath string) {
	s.rockTreeDataEnable = enable
	s.rockTreeDataHostName = hostName
	s.rockTreeDataBulkMetadataPath = bulkMetadataPath
	s.rockTreeDataNodeDataPath = nodeDataPath
	s.rockTreeDataImageryDataPath = imageryDataPath
}

// SetGoogleEarthDesktopDataConfig 设置 Google Earth Desktop 数据配置（从 config.go 的 Config 结构体传递）
func (s *Server) SetGoogleEarthDesktopDataConfig(enable bool, hostName, q2Path, imageryPath, terrainPath string) {
	s.googleEarthDesktopDataEnable = enable
	s.googleEarthDesktopDataHostName = hostName
	s.googleEarthDesktopDataQ2Path = q2Path
	s.googleEarthDesktopDataImageryPath = imageryPath
	s.googleEarthDesktopDataTerrainPath = terrainPath
}

// generateNodeID 生成节点 ID
func generateNodeID() string {
	return uuid.New().String()
}

// registerSelfAsNode 将自己注册为节点
func (s *Server) registerSelfAsNode() {
	hostname, _ := GetRealHostname()
	sysInfo, err := GetAllSystemInfo()
	if err != nil {
		s.logger.Warn("获取系统信息失败: %v，使用默认值", err)
		sysInfo = &SystemInfo{
			Hostname:   hostname,
			SystemInfo: GetRealSystemInfo(),
			CPUInfo:    GetRealCPUInfo(),
			MemoryInfo: GetRealMemoryInfo(),
		}
	}

	nodeInfo := &tasksmanager.GrpcServerNodeInfo{
		NodeUuid:           s.nodeID,
		NodeName:           sysInfo.Hostname,
		NodeIp:             s.address,
		NodePort:           s.port,
		NodeSystem:         sysInfo.SystemInfo,
		NodeVersion:        "1.0.0",
		NodeCpu:            sysInfo.CPUInfo,
		NodeMemory:         sysInfo.MemoryInfo,
		NodeCreateTime:     time.Now().Format(time.RFC3339),
		NodeLastActiveTime: time.Now().Format(time.RFC3339),
	}

	s.nodesMu.Lock()
	s.nodes[s.nodeID] = nodeInfo
	s.nodesMu.Unlock()

	s.logger.Info("节点已注册: %s (%s:%s)", s.nodeID, s.address, s.port)
}

// Start 启动 gRPC 服务器
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", s.address, s.port))
	if err != nil {
		return fmt.Errorf("监听端口失败: %w", err)
	}

	// 配置 gRPC 服务器选项
	var opts []grpc.ServerOption

	// 如果配置了 TLS，使用 TLS 凭证
	if s.tlsConfig != nil {
		creds := credentials.NewTLS(s.tlsConfig)
		opts = append(opts, grpc.Creds(creds))
		s.logger.Info("gRPC 服务器已启用 TLS 加密")
	} else {
		s.logger.Info("gRPC 服务器未启用 TLS（使用明文连接）")
	}

	s.grpcServer = grpc.NewServer(opts...)
	tasksmanager.RegisterTasksManagerServer(s.grpcServer, s)

	s.logger.Info("gRPC 服务器启动在 %s:%s", s.address, s.port)

	// 启动心跳检查
	go s.startHeartbeatChecker()

	return s.grpcServer.Serve(lis)
}

// Stop 停止 gRPC 服务器
func (s *Server) Stop() {
	s.logger.Info("开始停止服务器...")

	// 停止节点连接管理器
	if s.nodeConnector != nil {
		s.logger.Info("正在停止节点连接管理器...")
		s.nodeConnector.Stop()
		s.logger.Info("节点连接管理器已停止")
	}

	// 关闭 IP 池（如果存在）
	if s.ipPool != nil {
		s.logger.Info("正在关闭 IP 池...")
		if err := s.ipPool.Close(); err != nil {
			s.logger.Warn("关闭 IP 池时出错: %v", err)
		} else {
			s.logger.Info("IP 池已关闭")
		}
	}

	if s.grpcServer != nil {
		s.logger.Info("正在停止 gRPC 服务器（优雅关闭，最多等待 10 秒）...")
		// 使用带超时的优雅关闭
		stopped := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(stopped)
		}()

		// 等待优雅关闭完成，最多等待 10 秒
		select {
		case <-stopped:
			s.logger.Info("gRPC 服务器已优雅停止")
		case <-time.After(10 * time.Second):
			s.logger.Warn("优雅关闭超时，强制停止 gRPC 服务器...")
			s.grpcServer.Stop()
			s.logger.Info("gRPC 服务器已强制停止")
		}
	}

	s.logger.Info("服务器停止完成")
}

// GetTaskClientInfoList 获取任务客户端列表
// 从所有服务器节点收集客户端信息（同步）
func (s *Server) GetTaskClientInfoList(ctx context.Context, req *tasksmanager.TaskClientInfoListRequest) (*tasksmanager.TaskClientInfoListResponse, error) {
	// 先获取本地客户端列表
	s.clientsMu.RLock()
	localClients := make([]*tasksmanager.TaskClientInfo, 0, len(s.clients))
	for _, client := range s.clients {
		localClients = append(localClients, client)
	}
	s.clientsMu.RUnlock()

	// 从其他服务器节点收集客户端信息
	var allClients []*tasksmanager.TaskClientInfo
	clientMap := make(map[string]*tasksmanager.TaskClientInfo) // 用UUID去重

	// 添加本地客户端
	for _, client := range localClients {
		clientMap[client.ClientUuid] = client
	}

	// 从其他节点获取客户端列表
	if s.nodeConnector != nil {
		connectedNodes := s.nodeConnector.GetConnectedNodes()
		for nodeUUID, client := range connectedNodes {
			// 向其他服务器节点请求客户端列表
			ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
			resp, err := client.GetTaskClientInfoList(ctx2, &tasksmanager.TaskClientInfoListRequest{})
			cancel()

			if err == nil {
				for _, clientInfo := range resp.Items {
					// 合并客户端信息（如果本地没有或版本更新，则使用远程的）
					if existing, exists := clientMap[clientInfo.ClientUuid]; !exists {
						clientMap[clientInfo.ClientUuid] = clientInfo
						s.logger.Debug("从节点 %s 同步客户端: %s", nodeUUID, clientInfo.ClientUuid)
					} else {
						// 比较最后活跃时间，保留最新的
						if clientInfo.ClientLastActiveTime > existing.ClientLastActiveTime {
							clientMap[clientInfo.ClientUuid] = clientInfo
						}
					}
				}
			} else {
				s.logger.Debug("从节点 %s 获取客户端列表失败: %v", nodeUUID, err)
			}
		}
	}

	// 转换为列表
	for _, client := range clientMap {
		allClients = append(allClients, client)
	}

	s.logger.Debug("获取客户端列表，本地 %d 个，合并后共 %d 个客户端", len(localClients), len(allClients))

	return &tasksmanager.TaskClientInfoListResponse{
		Items: allClients,
	}, nil
}

// isClientNode 判断是否为客户端节点（客户端节点的 UUID 通常以 "node-" 开头）
func (s *Server) isClientNode(nodeUUID string) bool {
	return len(nodeUUID) > 5 && nodeUUID[:5] == "node-"
}

// isServerNode 判断是否为服务器节点（服务器节点是 UUID 格式，不以 "node-" 开头）
func (s *Server) isServerNode(nodeUUID string) bool {
	return !s.isClientNode(nodeUUID)
}

// GetGrpcServerNodeInfoList 获取 gRPC 服务器节点列表（只返回服务器节点，不包括客户端节点）
func (s *Server) GetGrpcServerNodeInfoList(ctx context.Context, req *tasksmanager.GrpcServerNodeInfoListRequest) (*tasksmanager.GrpcServerNodeInfoListResponse, error) {
	s.nodesMu.RLock()
	defer s.nodesMu.RUnlock()

	items := make([]*tasksmanager.GrpcServerNodeInfo, 0)
	for uuid, node := range s.nodes {
		// 只返回服务器节点，不包括客户端节点
		if s.isServerNode(uuid) {
			items = append(items, node)
		}
	}

	s.logger.Debug("获取服务器节点列表，共 %d 个服务器节点（排除客户端节点）", len(items))

	return &tasksmanager.GrpcServerNodeInfoListResponse{
		Items: items,
	}, nil
}

// buildPathForTask 根据任务类型和参数构建路径（不包含域名，只返回路径部分）
// 返回: dataType, hostName, path, error
func (s *Server) buildPathForTask(taskType tasksmanager.TaskType, tileKey string, epoch int32, imageryEpoch *int32) (string, string, string, error) {
	// 根据任务类型选择配置和路径模板
	switch taskType {
	case tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2:
		if !s.googleEarthDesktopDataEnable {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData 配置未启用")
		}
		hostName := s.googleEarthDesktopDataHostName
		pathTemplate := s.googleEarthDesktopDataQ2Path
		if hostName == "" || pathTemplate == "" {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData Q2 配置不完整")
		}
		path := fmt.Sprintf(pathTemplate, tileKey, epoch)
		return "GoogleEarthDesktopData", hostName, path, nil

	case tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_IMAGERY:
		if !s.googleEarthDesktopDataEnable {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData 配置未启用")
		}
		if imageryEpoch == nil {
			return "", "", "", fmt.Errorf("Imagery 任务需要 imageryEpoch 参数")
		}
		hostName := s.googleEarthDesktopDataHostName
		pathTemplate := s.googleEarthDesktopDataImageryPath
		if hostName == "" || pathTemplate == "" {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData Imagery 配置不完整")
		}
		path := fmt.Sprintf(pathTemplate, tileKey, *imageryEpoch)
		return "GoogleEarthDesktopData", hostName, path, nil

	case tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_TERRAIN:
		if !s.googleEarthDesktopDataEnable {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData 配置未启用")
		}
		hostName := s.googleEarthDesktopDataHostName
		pathTemplate := s.googleEarthDesktopDataTerrainPath
		if hostName == "" || pathTemplate == "" {
			return "", "", "", fmt.Errorf("GoogleEarthDesktopData Terrain 配置不完整")
		}
		path := fmt.Sprintf(pathTemplate, tileKey, epoch)
		return "GoogleEarthDesktopData", hostName, path, nil

	default:
		return "", "", "", fmt.Errorf("不支持的任务类型: %v", taskType)
	}
}

// executeTaskWithHotPool 使用热连接池执行任务
// 参数: dataType - 数据类型, hostName - 主机名（用于从池中获取连接）, path - 请求路径（包含查询参数）
func (s *Server) executeTaskWithHotPool(dataType, hostName, path string, req *tasksmanager.TaskRequest) ([]byte, int32, error) {
	if s.utlsClient == nil {
		return nil, 0, fmt.Errorf("UTLS 客户端未设置")
	}

	// 记录各个阶段的时间
	//totalStart := time.Now()

	// 使用主机名和路径构建请求 URL（底层连接已绑定到具体 IP）
	requestURL := fmt.Sprintf("https://%s%s", hostName, path)

	// 确定 HTTP 方法
	method := "GET"
	if req.TaskMethod != nil {
		switch *req.TaskMethod {
		case tasksmanager.TaskMethod_TASK_METHOD_POST:
			method = "POST"
		case tasksmanager.TaskMethod_TASK_METHOD_PUT:
			method = "PUT"
		case tasksmanager.TaskMethod_TASK_METHOD_DELETE:
			method = "DELETE"
		default:
			method = "GET"
		}
	}

	// 发送请求（带 5xx 与网络错误重试）
	// 统一重试策略：只在预热中或连接恢复时重试，其他情况立即返回
	const maxHTTPRetries = 2 // 最多重试2次（初始请求+1次重试）
	var lastErr error
	var conn *utlsclient.UTLSConnection

	for attempt := 1; attempt <= maxHTTPRetries; attempt++ {
		// 每次重试都获取新连接（如果连接已标记为不健康，会获取新连接）
		connStart := time.Now()
		newConn, err := s.utlsClient.GetConnectionForHost(hostName)
		if err != nil {
			lastErr = err
			errStr := err.Error()

			// 统一判断是否需要重试
			shouldRetry := false
			var waitTime time.Duration

			if strings.Contains(errStr, "PoolManager正在预热中") {
				// 预热中：等待后重试
				shouldRetry = true
				waitTime = 200 * time.Millisecond // 短暂等待，让预热有机会完成
			} else if strings.Contains(errStr, "所有连接都不健康，正在异步激活") {
				// 连接正在异步激活：短暂等待后重试
				shouldRetry = true
				waitTime = 100 * time.Millisecond // 短暂等待，让异步激活有机会完成
			}

			if shouldRetry && attempt < maxHTTPRetries {
				s.logger.Debug("连接获取失败，等待 %v 后重试 (第 %d 次): %v", waitTime, attempt, err)
				time.Sleep(waitTime)
				continue
			}

			// 不需要重试或已达到最大重试次数，立即返回错误
			return nil, 0, fmt.Errorf("获取连接失败(重试 %d 次后仍失败): %w", attempt-1, err)
		}

		// 释放旧连接（如果有）
		if conn != nil {
			s.utlsClient.ReleaseConnection(conn)
		}
		conn = newConn

		connTime := time.Since(connStart)
		if connTime > 500*time.Millisecond {
			s.logger.Debug("⚠️ 获取连接耗时: %v (数据类型: %s, 尝试: %d)", connTime, dataType, attempt)
		}

		// 每次重试都重新构建请求（因为 Body 是一次性的 Reader）
		var bodyReader io.Reader
		if len(req.TaskBody) > 0 {
			bodyReader = bytes.NewReader(req.TaskBody)
		}

		httpReq, err := http.NewRequest(method, requestURL, bodyReader)
		if err != nil {
			s.utlsClient.ReleaseConnection(conn)
			return nil, 0, fmt.Errorf("创建 HTTP 请求失败: %w", err)
		}

		// 设置 Host 头为域名（用于 SNI 和 Host 头）
		httpReq.Host = hostName
		httpReq.Header.Set("Host", hostName)

		// 设置请求头
		if bodyReader != nil {
			httpReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(req.TaskBody)))
		}

		// 记录请求开始时间和连接信息
		requestStart := time.Now()
		localIP := conn.LocalIP()
		remoteIP := conn.TargetIP()

		resp, err := conn.RoundTrip(httpReq)
		if err != nil {
			lastErr = err
			// 请求失败，记录日志
			requestDuration := time.Since(requestStart)
			requestCount := conn.RequestCount()
			s.logger.Warn("[任务请求失败] 本地IPv6=%s, 远程IPv6=%s, 已完成请求数=%d, 耗时=%v, 错误=%v",
				getIPDisplay(localIP), getIPDisplay(remoteIP), requestCount, requestDuration, err)

			// 释放连接（连接可能已损坏）
			s.utlsClient.ReleaseConnection(conn)
			conn = nil

			// 对于网络错误，短暂等待后重试（让连接池有机会恢复）
			if attempt < maxHTTPRetries {
				waitTime := 50 * time.Millisecond // 减少等待时间，从200ms减少到50ms
				s.logger.Debug("HTTP 请求失败，等待 %v 后重试 (第 %d 次): %v", waitTime, attempt, err)
				time.Sleep(waitTime)
				continue
			}
			// 最后一次重试失败，返回错误
			return nil, 0, fmt.Errorf("HTTP 请求失败(重试 %d 次后仍失败): %w", attempt, err)
		}

		// 请求成功，记录详细日志
		requestDuration := time.Since(requestStart)
		requestCount := conn.RequestCount()
		s.logger.Info("[任务请求成功] 本地IPv6=%s, 远程IPv6=%s, 已完成请求数=%d, 耗时=%v, 状态码=%d, 路径=%s",
			getIPDisplay(localIP), getIPDisplay(remoteIP), requestCount, requestDuration, resp.StatusCode, path)

		// 请求成功，释放连接并返回
		s.utlsClient.ReleaseConnection(conn)

		// 成功拿到响应，读取响应体
		responseBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			// 释放连接
			s.utlsClient.ReleaseConnection(conn)
			conn = nil
			if attempt < maxHTTPRetries {
				s.logger.Warn("读取响应体失败(第 %d 次，将重试): %v", attempt, readErr)
				time.Sleep(50 * time.Millisecond) // 减少等待时间
				continue
			}
			return nil, 0, fmt.Errorf("读取响应体失败(重试 %d 次后仍失败): %w", attempt, readErr)
		}

		// 对 5xx 做有限次重试
		if resp.StatusCode >= 500 && resp.StatusCode < 600 && attempt < maxHTTPRetries {
			// 释放连接（5xx可能是临时问题，释放连接让连接池有机会恢复）
			s.utlsClient.ReleaseConnection(conn)
			conn = nil
			s.logger.Warn("后端返回状态码 %d (第 %d 次)，将重试，请求URL=%s", resp.StatusCode, attempt, requestURL)
			time.Sleep(50 * time.Millisecond) // 减少等待时间
			continue
		}

		// 正常返回
		return responseBody, int32(resp.StatusCode), nil
	}

	// 理论上不会走到这里，如果走到这里，返回最后一次错误
	return nil, 0, fmt.Errorf("HTTP 请求失败: %v", lastErr)
}

// SubmitTask 提交任务请求
func (s *Server) SubmitTask(ctx context.Context, req *tasksmanager.TaskRequest) (*tasksmanager.TaskResponse, error) {
	taskID := generateTaskID()

	s.tasksMu.Lock()
	s.tasks[taskID] = req
	s.tasksMu.Unlock()

	// 使用反射或直接字段访问获取 TileKey、epoch 等字段
	// 注意：proto 文件需要重新生成后才能访问这些字段
	// 这里先尝试通过反射访问，如果失败则返回错误提示需要重新生成 proto
	tileKey, epoch, imageryEpoch, err := s.extractTaskParams(req)
	if err != nil {
		return nil, fmt.Errorf("提取任务参数失败: %w（提示：需要重新生成 proto 文件）", err)
	}

	// 计算任务长度并累加
	taskLength := int64(0)
	if req.TaskBody != nil {
		taskLength += int64(len(req.TaskBody))
	}
	taskLength += int64(len(req.TaskClientId))
	taskLength += int64(len(tileKey))
	taskLength += 4 // epoch (int32)

	s.taskLengthMu.Lock()
	s.totalTaskLength += taskLength
	//totalLength := s.totalTaskLength
	s.taskLengthMu.Unlock()

	//s.logger.Debug("收到任务请求: %s, 类型: %v, TileKey: %s, epoch: %d, 任务长度: %d 字节, 累计总长度: %d 字节 (%s)",
	//	taskID, req.TaskType, tileKey, epoch, taskLength, totalLength, formatBytes(totalLength))

	// 构建路径（不包含域名，只返回路径部分）
	dataType, hostName, path, err := s.buildPathForTask(req.TaskType, tileKey, epoch, imageryEpoch)
	if err != nil {
		return nil, fmt.Errorf("构建路径失败: %w", err)
	}

	//s.logger.Debug("任务 %s 构建的路径: %s (数据类型: %s, 主机名: %s)", taskID, path, dataType, hostName)

	// 使用热连接池执行任务（通过主机名获取连接，使用 IP 地址直接访问）
	responseBody, statusCode, err := s.executeTaskWithHotPool(dataType, hostName, path, req)
	if err != nil {
		// 检查是否是连接问题（应该继续重试，而不是返回 500）
		errStr := err.Error()
		isConnectionError := strings.Contains(errStr, "获取连接失败") ||
			strings.Contains(errStr, "没有可用连接") ||
			strings.Contains(errStr, "所有连接都不健康") ||
			strings.Contains(errStr, "HTTP 请求失败") ||
			strings.Contains(errStr, "use of closed network connection") ||
			strings.Contains(errStr, "connection closed") ||
			strings.Contains(errStr, "broken pipe") ||
			strings.Contains(errStr, "连接已标记为不健康")

		if isConnectionError {
			// 连接问题，不应该返回 500，应该继续重试
			// 但这里已经重试了 10 次，说明连接池可能有问题
			// 记录详细日志，但不返回 500（返回一个临时错误，让客户端知道需要重试）
			s.logger.Warn("任务执行失败（连接问题，已重试10次）: %s, 错误: %v", taskID, err)
			// 返回 503 Service Unavailable，表示服务暂时不可用，客户端可以重试
			errorStatusCode := int32(503)
			errorBody := []byte(fmt.Sprintf("服务暂时不可用，请稍后重试: %v", err))
			return &tasksmanager.TaskResponse{
				TaskClientId:           req.TaskClientId,
				TaskType:               req.TaskType,
				TaskResponseBody:       errorBody,
				TaskResponseStatusCode: &errorStatusCode,
			}, nil
		}

		// 其他错误（如配置错误），返回 500
		s.logger.Warn("任务执行失败: %s, 错误: %v", taskID, err)
		errorStatusCode := int32(500)
		errorBody := []byte(fmt.Sprintf("任务执行失败: %v", err))
		return &tasksmanager.TaskResponse{
			TaskClientId:           req.TaskClientId,
			TaskType:               req.TaskType,
			TaskResponseBody:       errorBody,
			TaskResponseStatusCode: &errorStatusCode,
		}, nil
	}

	//s.logger.Debug("任务 %s 执行成功，状态码: %d, 响应体长度: %d 字节", taskID, statusCode, len(responseBody))

	// 构建响应
	response := &tasksmanager.TaskResponse{
		TaskClientId:           req.TaskClientId,
		TaskType:               req.TaskType,
		TaskResponseBody:       responseBody,
		TaskResponseStatusCode: &statusCode,
	}

	// 设置 TileKey、epoch 等字段（需要 proto 重新生成后支持）
	s.setResponseParams(response, tileKey, epoch, imageryEpoch)

	return response, nil
}

// extractTaskParams 提取任务参数（使用反射访问新字段，待 proto 重新生成后改为直接字段访问）
func (s *Server) extractTaskParams(req *tasksmanager.TaskRequest) (tileKey string, epoch int32, imageryEpoch *int32, err error) {
	// 使用反射访问字段（因为 proto 可能还没重新生成）
	// 注意：这里假设字段名是 TileKey、Epoch、ImageryEpoch
	// 如果 proto 已经重新生成，可以直接使用 req.TileKey, req.Epoch 等
	reqValue := reflect.ValueOf(req).Elem()

	tileKeyField := reqValue.FieldByName("TileKey")
	if !tileKeyField.IsValid() || tileKeyField.Kind() != reflect.String {
		return "", 0, nil, fmt.Errorf("TileKey 字段不存在或类型不匹配")
	}
	tileKey = tileKeyField.String()

	epochField := reqValue.FieldByName("Epoch")
	if !epochField.IsValid() || epochField.Kind() != reflect.Int32 {
		return "", 0, nil, fmt.Errorf("Epoch 字段不存在或类型不匹配")
	}
	epoch = int32(epochField.Int())

	imageryEpochField := reqValue.FieldByName("ImageryEpoch")
	if imageryEpochField.IsValid() && imageryEpochField.Kind() == reflect.Ptr && !imageryEpochField.IsNil() {
		if imageryEpochField.Elem().Kind() == reflect.Int32 {
			val := int32(imageryEpochField.Elem().Int())
			imageryEpoch = &val
		}
	}

	return tileKey, epoch, imageryEpoch, nil
}

// setResponseParams 设置响应参数（使用反射，待 proto 重新生成后改为直接字段访问）
func (s *Server) setResponseParams(resp *tasksmanager.TaskResponse, tileKey string, epoch int32, imageryEpoch *int32) {
	respValue := reflect.ValueOf(resp).Elem()

	if tileKeyField := respValue.FieldByName("TileKey"); tileKeyField.IsValid() && tileKeyField.CanSet() {
		tileKeyField.SetString(tileKey)
	}

	if epochField := respValue.FieldByName("Epoch"); epochField.IsValid() && epochField.CanSet() {
		epochField.SetInt(int64(epoch))
	}

	if imageryEpoch != nil {
		if imageryEpochField := respValue.FieldByName("ImageryEpoch"); imageryEpochField.IsValid() && imageryEpochField.CanSet() {
			ptrValue := reflect.New(reflect.TypeOf(int32(0)))
			ptrValue.Elem().SetInt(int64(*imageryEpoch))
			imageryEpochField.Set(ptrValue)
		}
	}
}

// GetTotalTaskLength 获取累计接收到的任务总长度（字节）
// 输出: int64 - 累计任务总长度（字节）
func (s *Server) GetTotalTaskLength() int64 {
	s.taskLengthMu.Lock()
	defer s.taskLengthMu.Unlock()
	return s.totalTaskLength
}

// RegisterClient 客户端注册（客户端专用接口，与服务器节点完全分离）
func (s *Server) RegisterClient(ctx context.Context, clientInfo *tasksmanager.TaskClientInfo) (*tasksmanager.RegisterClientResponse, error) {
	// 更新客户端信息
	s.clientsMu.Lock()
	s.clients[clientInfo.ClientUuid] = clientInfo
	clientInfo.ClientTaskStatus = tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE
	clientInfo.ClientLastActiveTime = time.Now().Format(time.RFC3339)
	s.clientsMu.Unlock()

	s.logger.Info("客户端注册: %s (%s)", clientInfo.ClientUuid, clientInfo.ClientName)

	// 返回所有已知的服务器节点列表（客户端可以连接到这些服务器）
	s.nodesMu.RLock()
	serverNodes := make([]*tasksmanager.GrpcServerNodeInfo, 0)
	for uuid, node := range s.nodes {
		// 只返回服务器节点
		if s.isServerNode(uuid) {
			serverNodes = append(serverNodes, node)
		}
	}
	s.nodesMu.RUnlock()

	return &tasksmanager.RegisterClientResponse{
		Success:     true,
		Message:     "客户端注册成功",
		ServerNodes: serverNodes,
	}, nil
}

// ClientHeartbeat 客户端心跳（客户端专用接口，与服务器节点完全分离）
func (s *Server) ClientHeartbeat(ctx context.Context, clientInfo *tasksmanager.TaskClientInfo) (*tasksmanager.ClientHeartbeatResponse, error) {
	// 更新客户端信息和资源使用情况
	s.clientsMu.Lock()
	if client, exists := s.clients[clientInfo.ClientUuid]; exists {
		// 更新客户端信息
		client.ClientLastActiveTime = time.Now().Format(time.RFC3339)
		client.ClientTaskStatus = tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE
		if clientInfo.CpuUsagePercent != nil {
			client.CpuUsagePercent = clientInfo.CpuUsagePercent
		}
		if clientInfo.MemoryUsedBytes != nil {
			client.MemoryUsedBytes = clientInfo.MemoryUsedBytes
		}
		if clientInfo.MemoryTotalBytes != nil {
			client.MemoryTotalBytes = clientInfo.MemoryTotalBytes
		}
		if clientInfo.NetworkRxBytesPerSec != nil {
			client.NetworkRxBytesPerSec = clientInfo.NetworkRxBytesPerSec
		}
		if clientInfo.NetworkTxBytesPerSec != nil {
			client.NetworkTxBytesPerSec = clientInfo.NetworkTxBytesPerSec
		}
		if clientInfo.DiskUsedBytes != nil {
			client.DiskUsedBytes = clientInfo.DiskUsedBytes
		}
		if clientInfo.DiskTotalBytes != nil {
			client.DiskTotalBytes = clientInfo.DiskTotalBytes
		}
		if clientInfo.ResourceUpdateTime != nil {
			client.ResourceUpdateTime = clientInfo.ResourceUpdateTime
		}
	} else {
		// 首次心跳，注册客户端
		clientInfo.ClientTaskStatus = tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE
		clientInfo.ClientLastActiveTime = time.Now().Format(time.RFC3339)
		s.clients[clientInfo.ClientUuid] = clientInfo
	}
	s.clientsMu.Unlock()

	// 返回新上线的服务器节点（相对于该客户端上次心跳时）
	var newServerNodes []*tasksmanager.GrpcServerNodeInfo

	// 获取该客户端上次心跳时已知的服务器节点列表
	clientKnownNodesKey := "client-" + clientInfo.ClientUuid
	s.nodesMu.RLock()
	knownNodes := s.lastHeartbeatNodes[clientKnownNodesKey]
	if knownNodes == nil {
		knownNodes = make(map[string]bool)
	}

	// 找出新注册的服务器节点（客户端不知道的，且最近60秒内注册的）
	now := time.Now()
	for uuid, node := range s.nodes {
		if !s.isServerNode(uuid) {
			continue
		}

		// 如果客户端已经知道这个节点，跳过（除非是最近60秒内注册的新节点）
		if knownNodes[uuid] {
			registerTime, isNew := s.nodeRegisterTimes[uuid]
			isRecentNewNode := isNew && now.Sub(registerTime) < 60*time.Second
			// 只有最近注册的新节点才返回（即使客户端知道，因为可能是首次同步）
			if !isRecentNewNode {
				continue // 客户端已经知道且不是新节点，不返回
			}
		}

		// 客户端不知道的节点，或者是最近60秒内注册的新节点
		newServerNodes = append(newServerNodes, node)
	}

	// 更新该客户端已知的服务器节点列表
	s.lastHeartbeatNodes[clientKnownNodesKey] = make(map[string]bool)
	for uuid := range s.nodes {
		if s.isServerNode(uuid) {
			s.lastHeartbeatNodes[clientKnownNodesKey][uuid] = true
		}
	}
	s.nodesMu.RUnlock()

	return &tasksmanager.ClientHeartbeatResponse{
		Success:        true,
		NewServerNodes: newServerNodes,
	}, nil
}

// RegisterNode 服务器节点注册（只处理服务器节点）
func (s *Server) RegisterNode(ctx context.Context, req *tasksmanager.NodeRegistrationRequest) (*tasksmanager.NodeRegistrationResponse, error) {
	nodeInfo := req.NodeInfo

	// 只处理服务器节点
	s.nodesMu.Lock()
	s.nodes[nodeInfo.NodeUuid] = nodeInfo
	s.nodesMu.Unlock()

	s.logger.Info("服务器节点注册: %s (%s:%s)", nodeInfo.NodeUuid, nodeInfo.NodeIp, nodeInfo.NodePort)

	// 记录节点注册时间
	s.nodeRegisterTimes[nodeInfo.NodeUuid] = time.Now()

	// 自动连接到新注册的服务器节点
	if s.nodeConnector != nil {
		s.nodeConnector.OnNewNodeDiscovered(nodeInfo)
	}

	// 通知所有已连接的服务器节点有新节点加入
	s.notifyNodesAboutNewNode(nodeInfo)

	// 通知所有客户端有新服务器节点上线（用于客户端自动连接）
	s.notifyClientsAboutNewServerNode(nodeInfo)

	// 返回已知的服务器节点列表
	s.nodesMu.RLock()
	knownNodes := make([]*tasksmanager.GrpcServerNodeInfo, 0)
	for uuid, node := range s.nodes {
		// 只返回服务器节点
		if uuid != nodeInfo.NodeUuid && s.isServerNode(uuid) {
			knownNodes = append(knownNodes, node)
		}
	}
	s.nodesMu.RUnlock()

	return &tasksmanager.NodeRegistrationResponse{
		Success:    true,
		Message:    "服务器节点注册成功",
		KnownNodes: knownNodes,
	}, nil
}

// NodeHeartbeat 服务器节点心跳（只处理服务器节点，客户端不应该使用此接口）
func (s *Server) NodeHeartbeat(ctx context.Context, req *tasksmanager.NodeHeartbeatRequest) (*tasksmanager.NodeHeartbeatResponse, error) {
	// 只处理服务器节点的心跳
	s.nodesMu.Lock()
	if node, exists := s.nodes[req.NodeUuid]; exists {
		// 更新节点的资源使用情况
		if req.NodeInfo != nil {
			node.CpuUsagePercent = req.NodeInfo.CpuUsagePercent
			node.MemoryUsedBytes = req.NodeInfo.MemoryUsedBytes
			node.MemoryTotalBytes = req.NodeInfo.MemoryTotalBytes
			node.NetworkRxBytesPerSec = req.NodeInfo.NetworkRxBytesPerSec
			node.NetworkTxBytesPerSec = req.NodeInfo.NetworkTxBytesPerSec
			node.DiskUsedBytes = req.NodeInfo.DiskUsedBytes
			node.DiskTotalBytes = req.NodeInfo.DiskTotalBytes
			updateTime := time.Now().Format(time.RFC3339)
			node.ResourceUpdateTime = &updateTime
		}
		node.NodeLastActiveTime = time.Now().Format(time.RFC3339)
	} else {
		// 如果节点不存在，可能是首次心跳，创建节点信息（仅限服务器节点）
		if req.NodeInfo != nil && s.isServerNode(req.NodeUuid) {
			s.nodes[req.NodeUuid] = req.NodeInfo
			updateTime := time.Now().Format(time.RFC3339)
			req.NodeInfo.ResourceUpdateTime = &updateTime
			req.NodeInfo.NodeLastActiveTime = time.Now().Format(time.RFC3339)
			s.nodeRegisterTimes[req.NodeUuid] = time.Now()
		}
	}
	s.nodesMu.Unlock()

	s.logger.Debug("收到服务器节点心跳: %s", req.NodeUuid)

	// 返回新上线的服务器节点（相对于该客户端上次心跳时）
	var updatedNodes []*tasksmanager.GrpcServerNodeInfo

	// 获取该客户端上次心跳时已知的服务器节点列表
	s.nodesMu.RLock()
	knownNodes := s.lastHeartbeatNodes[req.NodeUuid]
	if knownNodes == nil {
		knownNodes = make(map[string]bool)
	}

	// 找出新注册的服务器节点（最近60秒内注册的，且客户端不知道的）
	// 只返回服务器节点，不包括客户端节点
	now := time.Now()
	for uuid, node := range s.nodes {
		// 跳过自己和请求心跳的节点，跳过客户端节点
		if uuid == s.nodeID || uuid == req.NodeUuid || !s.isServerNode(uuid) {
			continue
		}

		// 如果是客户端不知道的节点，或者是最近60秒内注册的新节点
		registerTime, isNew := s.nodeRegisterTimes[uuid]
		isRecentNewNode := isNew && now.Sub(registerTime) < 60*time.Second

		if !knownNodes[uuid] || isRecentNewNode {
			updatedNodes = append(updatedNodes, node)
		}
	}

	// 更新该客户端已知的服务器节点列表（只记录服务器节点）
	s.lastHeartbeatNodes[req.NodeUuid] = make(map[string]bool)
	for uuid := range s.nodes {
		if uuid != s.nodeID && uuid != req.NodeUuid && s.isServerNode(uuid) {
			s.lastHeartbeatNodes[req.NodeUuid][uuid] = true
		}
	}
	s.nodesMu.RUnlock()

	if len(updatedNodes) > 0 {
		s.logger.Debug("向客户端 %s 返回 %d 个新节点", req.NodeUuid, len(updatedNodes))
	}

	return &tasksmanager.NodeHeartbeatResponse{
		Success:      true,
		UpdatedNodes: updatedNodes, // 只返回新节点
	}, nil
}

// SendNodeMessage 发送节点消息
func (s *Server) SendNodeMessage(ctx context.Context, req *tasksmanager.NodeMessageRequest) (*tasksmanager.NodeMessageResponse, error) {
	msg := req.Message

	// 如果是广播消息
	if msg.ToNodeUuid == "" {
		// 存储消息，等待其他节点拉取
		s.messagesMu.Lock()
		s.messages[msg.MessageId] = msg
		s.messagesMu.Unlock()

		s.logger.Info("收到广播消息: %s, 类型: %s", msg.MessageId, msg.MessageType)
	} else {
		// 点对点消息
		s.messagesMu.Lock()
		s.messages[msg.MessageId] = msg
		s.messagesMu.Unlock()

		s.logger.Info("收到点对点消息: %s -> %s, 类型: %s", msg.FromNodeUuid, msg.ToNodeUuid, msg.MessageType)
	}

	return &tasksmanager.NodeMessageResponse{
		Success: true,
		Message: "消息已接收",
	}, nil
}

// SyncNodeList 同步节点列表
func (s *Server) SyncNodeList(ctx context.Context, req *tasksmanager.SyncNodeListRequest) (*tasksmanager.SyncNodeListResponse, error) {
	knownUUIDs := make(map[string]bool)
	for _, uuid := range req.KnownNodeUuids {
		knownUUIDs[uuid] = true
	}

	s.nodesMu.RLock()
	nodesToAdd := make([]*tasksmanager.GrpcServerNodeInfo, 0)
	nodesToUpdate := make([]*tasksmanager.GrpcServerNodeInfo, 0)

	// 只同步服务器节点，不包括客户端节点
	for uuid, node := range s.nodes {
		// 跳过客户端节点
		if !s.isServerNode(uuid) {
			continue
		}

		if !knownUUIDs[uuid] {
			// 新节点
			nodesToAdd = append(nodesToAdd, node)
		} else {
			// 检查是否需要更新
			nodesToUpdate = append(nodesToUpdate, node)
		}
	}
	s.nodesMu.RUnlock()

	s.logger.Debug("节点列表同步: 新增 %d 个，更新 %d 个", len(nodesToAdd), len(nodesToUpdate))

	return &tasksmanager.SyncNodeListResponse{
		NodesToAdd:    nodesToAdd,
		NodesToUpdate: nodesToUpdate,
	}, nil
}

// notifyNodesAboutNewNode 通知所有已连接的节点有新节点加入
func (s *Server) notifyNodesAboutNewNode(newNode *tasksmanager.GrpcServerNodeInfo) {
	if s.nodeConnector == nil {
		return
	}

	connectedNodes := s.nodeConnector.GetConnectedNodes()
	if len(connectedNodes) == 0 {
		return
	}

	// 向所有已连接的节点发送新节点信息
	// 每个 goroutine 使用独立的 context，避免互相影响
	for nodeUUID, client := range connectedNodes {
		go func(uuid string, c tasksmanager.TasksManagerClient) {
			// 为每个通知创建独立的 context
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			msg := &tasksmanager.NodeMessage{
				MessageId:    fmt.Sprintf("new-node-%s-%d", newNode.NodeUuid, time.Now().Unix()),
				FromNodeUuid: s.nodeID,
				ToNodeUuid:   uuid, // 点对点消息
				MessageType:  "NODE_DISCOVERED",
				Payload:      []byte(fmt.Sprintf(`{"node_uuid":"%s","node_ip":"%s","node_port":"%s"}`, newNode.NodeUuid, newNode.NodeIp, newNode.NodePort)),
				Timestamp:    time.Now().UnixMilli(),
			}

			req := &tasksmanager.NodeMessageRequest{
				Message: msg,
			}

			_, err := c.SendNodeMessage(ctx, req)
			if err != nil {
				// 通知失败不应该影响节点状态，只记录调试日志
				s.logger.Debug("通知节点 %s 新节点信息失败（非致命）: %v", uuid, err)
			} else {
				s.logger.Debug("已通知节点 %s 新节点加入: %s", uuid, newNode.NodeUuid)
			}
		}(nodeUUID, client)
	}
}

// notifyClientsAboutNewServerNode 通知所有客户端有新服务器节点上线
func (s *Server) notifyClientsAboutNewServerNode(nodeInfo *tasksmanager.GrpcServerNodeInfo) {
	// 通过心跳响应机制，客户端会在下次心跳时自动获取新节点
	// 这里不需要主动推送，因为心跳响应已经包含了新节点信息
	s.logger.Debug("新服务器节点 %s 已上线，客户端将在下次心跳时自动发现", nodeInfo.NodeUuid)
}

// startHeartbeatChecker 启动心跳检查器
func (s *Server) startHeartbeatChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		s.nodesMu.Lock()
		for uuid, node := range s.nodes {
			// 跳过自己
			if uuid == s.nodeID {
				continue
			}

			// 检查节点是否超时（90秒未收到心跳，放宽超时时间）
			lastActive, err := time.Parse(time.RFC3339, node.NodeLastActiveTime)
			if err == nil && now.Sub(lastActive) > 90*time.Second {
				s.logger.Warn("节点超时，可能已离线: %s (最后活跃: %s)", uuid, node.NodeLastActiveTime)
				// 注意：这里只记录警告，不自动移除节点，因为可能是网络波动
				// 节点会在下次心跳时自动恢复
			}
		}
		s.nodesMu.Unlock()
	}
}

// generateTaskID 生成任务 ID
func generateTaskID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// LoadTLSConfigFromCertsDir 从 certs 目录加载 TLS 配置
// 自动查找 certs 目录下的 .pem 证书和密钥文件
// 支持的命名格式：
//   - server.crt / server.key
//   - cert.pem / key.pem
//   - *.crt / *.key
//   - *.pem (证书和密钥都在同一个目录)
func LoadTLSConfigFromCertsDir(certsDir string) (*tls.Config, error) {
	if certsDir == "" {
		return nil, fmt.Errorf("证书目录路径不能为空")
	}

	// 检查目录是否存在
	info, err := os.Stat(certsDir)
	if err != nil {
		return nil, fmt.Errorf("证书目录不存在或无法访问: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("指定的路径不是目录: %s", certsDir)
	}

	var certFile, keyFile string

	// 查找证书和密钥文件
	err = filepath.WalkDir(certsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		baseName := filepath.Base(path)

		// 查找证书文件
		if certFile == "" {
			if ext == ".crt" || ext == ".pem" {
				// 优先匹配 server.crt, cert.pem
				if baseName == "server.crt" || baseName == "cert.pem" {
					certFile = path
				} else if certFile == "" && (ext == ".crt" || (ext == ".pem" && baseName != "key.pem" && baseName != "server.key")) {
					// 如果没有找到优先文件，使用第一个匹配的
					certFile = path
				}
			}
		}

		// 查找密钥文件
		if keyFile == "" {
			if ext == ".key" || (ext == ".pem" && (baseName == "key.pem" || baseName == "server.key")) {
				keyFile = path
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("遍历证书目录失败: %w", err)
	}

	if certFile == "" {
		return nil, fmt.Errorf("在 %s 目录中未找到证书文件（.crt 或 .pem）", certsDir)
	}
	if keyFile == "" {
		return nil, fmt.Errorf("在 %s 目录中未找到密钥文件（.key 或 key.pem）", certsDir)
	}

	// 加载证书和密钥
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载证书和密钥失败: %w", err)
	}

	// 创建 TLS 配置
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// 尝试加载 CA 证书（用于客户端验证，可选）
	caCertFile := filepath.Join(certsDir, "ca.crt")
	if _, err := os.Stat(caCertFile); err == nil {
		caCertPEM, err := os.ReadFile(caCertFile)
		if err == nil {
			caCertPool := x509.NewCertPool()
			if caCertPool.AppendCertsFromPEM(caCertPEM) {
				config.ClientCAs = caCertPool
				config.ClientAuth = tls.RequireAndVerifyClientCert
			}
		}
	}

	return config, nil
}

// LoadTLSConfigFromFiles 从指定的证书和密钥文件加载 TLS 配置
func LoadTLSConfigFromFiles(certFile, keyFile string) (*tls.Config, error) {
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("证书文件和密钥文件路径不能为空")
	}

	// 加载证书和密钥
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("加载证书和密钥失败: %w", err)
	}

	// 创建 TLS 配置
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	return config, nil
}

// formatBytes 格式化字节数为人类可读的格式（B, KB, MB, GB）
// 输入: bytes - 字节数
// 输出: string - 格式化后的字符串
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// getIPDisplay 格式化IP地址显示（如果为空则显示为"系统默认"）
func getIPDisplay(ip string) string {
	if ip == "" {
		return "系统默认"
	}
	return ip
}
