package grpcserver

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"

	"github.com/google/uuid"
	"google.golang.org/grpc"
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

	// 记录节点注册时间，用于判断新节点
	nodeRegisterTimes  map[string]time.Time       // 节点UUID -> 注册时间
	lastHeartbeatNodes map[string]map[string]bool // 客户端UUID -> 已知节点UUID集合（用于返回新节点）

	// gRPC 服务器实例
	grpcServer *grpc.Server

	// 节点连接管理器（用于自动发现和连接其他节点）
	nodeConnector *NodeConnector

	// 日志记录器
	logger logger.Logger
}

// NewServer 创建新的 gRPC 服务器实例
func NewServer(address, port string) *Server {
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

	s.grpcServer = grpc.NewServer()
	tasksmanager.RegisterTasksManagerServer(s.grpcServer, s)

	s.logger.Info("gRPC 服务器启动在 %s:%s", s.address, s.port)

	// 启动心跳检查
	go s.startHeartbeatChecker()

	return s.grpcServer.Serve(lis)
}

// Stop 停止 gRPC 服务器
func (s *Server) Stop() {
	// 停止节点连接管理器
	if s.nodeConnector != nil {
		s.nodeConnector.Stop()
	}

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
		s.logger.Info("gRPC 服务器已停止")
	}
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

// SubmitTask 提交任务请求
func (s *Server) SubmitTask(ctx context.Context, req *tasksmanager.TaskRequest) (*tasksmanager.TaskResponse, error) {
	taskID := generateTaskID()

	s.tasksMu.Lock()
	s.tasks[taskID] = req
	s.tasksMu.Unlock()

	s.logger.Info("收到任务请求: %s, 类型: %s, URI: %s", taskID, req.TaskType, req.TaskUri)

	// TODO: 实际执行任务逻辑

	return &tasksmanager.TaskResponse{
		TaskClientId: req.TaskClientId,
		TaskType:     req.TaskType,
		TaskUri:      req.TaskUri,
		// TaskResponseBody 和 TaskResponseStatusCode 需要在任务执行完成后设置
	}, nil
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
