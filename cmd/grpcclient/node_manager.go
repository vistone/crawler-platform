package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NodeManager 节点管理器
// 负责自动发现和连接到其他节点
type NodeManager struct {
	// 主服务器客户端
	mainClient tasksmanager.TasksManagerClient
	mainConn   *grpc.ClientConn

	// 已连接的节点客户端映射 (nodeUUID -> client)
	connectedNodes   map[string]tasksmanager.TasksManagerClient
	connectedNodesMu sync.RWMutex

	// 节点连接映射 (nodeUUID -> grpc.Conn)
	nodeConnections   map[string]*grpc.ClientConn
	nodeConnectionsMu sync.RWMutex

	// 已知节点列表
	knownNodes   map[string]*tasksmanager.GrpcServerNodeInfo
	knownNodesMu sync.RWMutex

	// 节点 ID
	nodeID string
}

// NewNodeManager 创建新的节点管理器
func NewNodeManager(mainClient tasksmanager.TasksManagerClient, mainConn *grpc.ClientConn, nodeID string) *NodeManager {
	return &NodeManager{
		mainClient:      mainClient,
		mainConn:        mainConn,
		connectedNodes:  make(map[string]tasksmanager.TasksManagerClient),
		nodeConnections: make(map[string]*grpc.ClientConn),
		knownNodes:      make(map[string]*tasksmanager.GrpcServerNodeInfo),
		nodeID:          nodeID,
	}
}

// ConnectToNode 连接到指定的节点
func (nm *NodeManager) ConnectToNode(nodeInfo *tasksmanager.GrpcServerNodeInfo) error {
	// 不连接自己
	if nodeInfo.NodeUuid == nm.nodeID {
		return nil
	}

	// 检查是否已连接（连接复用）
	nm.connectedNodesMu.RLock()
	if _, exists := nm.connectedNodes[nodeInfo.NodeUuid]; exists {
		// 检查连接是否仍然有效（通过检查连接状态）
		nm.nodeConnectionsMu.RLock()
		conn, connExists := nm.nodeConnections[nodeInfo.NodeUuid]
		nm.nodeConnectionsMu.RUnlock()

		if connExists && conn != nil {
			// 连接存在且有效，复用连接
			nm.connectedNodesMu.RUnlock()
			return nil // 已连接，复用现有连接
		} else {
			// 连接已断开，清理并重新连接
			nm.connectedNodesMu.RUnlock()
			nm.connectedNodesMu.Lock()
			delete(nm.connectedNodes, nodeInfo.NodeUuid)
			nm.connectedNodesMu.Unlock()
			nm.nodeConnectionsMu.Lock()
			delete(nm.nodeConnections, nodeInfo.NodeUuid)
			nm.nodeConnectionsMu.Unlock()
			log.Printf("⚠️ 检测到节点 %s 连接已断开，将重新连接", nodeInfo.NodeUuid)
		}
	} else {
		nm.connectedNodesMu.RUnlock()
	}

	// 构建节点地址
	// 如果节点 IP 是 0.0.0.0，则使用 localhost
	nodeIP := nodeInfo.NodeIp
	if nodeIP == "0.0.0.0" || nodeIP == "" {
		nodeIP = "127.0.0.1"
	}
	nodeAddr := fmt.Sprintf("%s:%s", nodeIP, nodeInfo.NodePort)

	log.Printf("🔗 正在自动连接到服务器节点: %s (%s)", nodeInfo.NodeUuid, nodeAddr)

	// 建立 gRPC 连接（带超时）
	conn, err := grpc.NewClient(nodeAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(5*time.Second))

	if err != nil {
		log.Printf("❌ 连接服务器节点失败 %s (%s): %v", nodeInfo.NodeUuid, nodeAddr, err)
		return fmt.Errorf("连接节点失败 %s: %w", nodeAddr, err)
	}

	// 验证连接是否真的可用（尝试调用一个简单的 RPC）
	testClient := tasksmanager.NewTasksManagerClient(conn)
	testCtx, testCancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, testErr := testClient.GetGrpcServerNodeInfoList(testCtx, &tasksmanager.GrpcServerNodeInfoListRequest{})
	testCancel()

	if testErr != nil {
		conn.Close()
		log.Printf("❌ 服务器节点连接验证失败 %s (%s): %v", nodeInfo.NodeUuid, nodeAddr, testErr)
		return fmt.Errorf("连接验证失败 %s: %w", nodeAddr, testErr)
	}

	// 连接成功，保存连接和客户端到连接池
	nm.nodeConnectionsMu.Lock()
	nm.nodeConnections[nodeInfo.NodeUuid] = conn
	nm.nodeConnectionsMu.Unlock()

	nm.connectedNodesMu.Lock()
	nm.connectedNodes[nodeInfo.NodeUuid] = testClient
	nm.connectedNodesMu.Unlock()

	// 添加到已知节点列表
	nm.knownNodesMu.Lock()
	nm.knownNodes[nodeInfo.NodeUuid] = nodeInfo
	nm.knownNodesMu.Unlock()

	log.Printf("✅ 成功连接到服务器节点: %s (%s)，连接池大小: %d",
		nodeInfo.NodeUuid, nodeAddr, len(nm.nodeConnections))

	return nil
}

// OnNewNodeDiscovered 当发现新节点时调用
func (nm *NodeManager) OnNewNodeDiscovered(nodeInfo *tasksmanager.GrpcServerNodeInfo) {
	// 不连接自己
	if nodeInfo.NodeUuid == nm.nodeID {
		return
	}

	// 检查是否已知
	nm.knownNodesMu.RLock()
	_, known := nm.knownNodes[nodeInfo.NodeUuid]
	nm.knownNodesMu.RUnlock()

	if !known {
		log.Printf("🔍 发现新服务器节点: %s (%s:%s)", nodeInfo.NodeUuid, nodeInfo.NodeIp, nodeInfo.NodePort)

		// 先添加到已知列表（防止重复连接）
		nm.knownNodesMu.Lock()
		nm.knownNodes[nodeInfo.NodeUuid] = nodeInfo
		nm.knownNodesMu.Unlock()

		// 自动连接到新节点（同步调用，以便检查错误）
		if err := nm.ConnectToNode(nodeInfo); err != nil {
			log.Printf("❌ 连接服务器节点失败 %s: %v", nodeInfo.NodeUuid, err)
			// 连接失败，从已知列表中移除，以便下次重试
			nm.knownNodesMu.Lock()
			delete(nm.knownNodes, nodeInfo.NodeUuid)
			nm.knownNodesMu.Unlock()
		}
	} else {
		// 节点已知道，静默跳过
	}
}

// OnNodesDiscovered 批量处理发现的节点
func (nm *NodeManager) OnNodesDiscovered(nodes []*tasksmanager.GrpcServerNodeInfo) {
	for _, node := range nodes {
		nm.OnNewNodeDiscovered(node)
	}
}

// ProcessClientHeartbeatResponse 处理客户端心跳响应，检查是否有新上线的服务器节点
func (nm *NodeManager) ProcessClientHeartbeatResponse(resp *tasksmanager.ClientHeartbeatResponse) {
	// 客户端心跳响应中包含新上线的服务器节点
	if resp != nil && resp.Success && len(resp.NewServerNodes) > 0 {
		// 过滤出真正的新节点（客户端不知道的）
		newNodes := make([]*tasksmanager.GrpcServerNodeInfo, 0)
		nm.knownNodesMu.RLock()
		for _, node := range resp.NewServerNodes {
			if _, known := nm.knownNodes[node.NodeUuid]; !known && node.NodeUuid != nm.nodeID {
				newNodes = append(newNodes, node)
			}
		}
		nm.knownNodesMu.RUnlock()

		if len(newNodes) > 0 {
			log.Printf("📡 心跳响应中发现 %d 个新上线的服务器节点，正在自动连接...", len(newNodes))
			for _, node := range newNodes {
				nm.OnNewNodeDiscovered(node)
			}
		}
		// 如果没有新节点，静默跳过
	}
}

// ProcessRegistrationResponse 处理客户端注册响应，连接到服务器节点
func (nm *NodeManager) ProcessRegistrationResponse(resp *tasksmanager.RegisterClientResponse) {
	if resp != nil && resp.Success && len(resp.ServerNodes) > 0 {
		log.Printf("📡 客户端注册响应中发现 %d 个服务器节点，开始自动连接", len(resp.ServerNodes))
		nm.OnNodesDiscovered(resp.ServerNodes)
	}
}

// ProcessNodeMessage 处理节点消息（处理 NODE_DISCOVERED 消息）
func (nm *NodeManager) ProcessNodeMessage(msg *tasksmanager.NodeMessage) {
	if msg.MessageType == "NODE_DISCOVERED" {
		// 解析新节点信息
		var nodeInfo struct {
			NodeUUID string `json:"node_uuid"`
			NodeIP   string `json:"node_ip"`
			NodePort string `json:"node_port"`
		}

		if err := json.Unmarshal(msg.Payload, &nodeInfo); err != nil {
			log.Printf("解析新节点消息失败: %v", err)
			return
		}

		// 通过主服务器获取完整节点信息
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		nodeListResp, err := nm.mainClient.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
		if err == nil {
			for _, node := range nodeListResp.Items {
				if node.NodeUuid == nodeInfo.NodeUUID {
					nm.OnNewNodeDiscovered(node)
					break
				}
			}
		}
	}
}

// StartAutoDiscovery 启动自动发现
func (nm *NodeManager) StartAutoDiscovery(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 定期从主服务器获取节点列表
			nodeListResp, err := nm.mainClient.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
			if err == nil {
				// 检查是否有新节点
				for _, node := range nodeListResp.Items {
					if node.NodeUuid != nm.nodeID {
						nm.OnNewNodeDiscovered(node)
					}
				}
			}
		}
	}
}

// GetConnectionPoolSize 获取连接池大小
func (nm *NodeManager) GetConnectionPoolSize() int {
	nm.nodeConnectionsMu.RLock()
	defer nm.nodeConnectionsMu.RUnlock()
	return len(nm.nodeConnections)
}

// GetConnectedNodeUUIDs 获取所有已连接的节点 UUID 列表
func (nm *NodeManager) GetConnectedNodeUUIDs() []string {
	nm.nodeConnectionsMu.RLock()
	defer nm.nodeConnectionsMu.RUnlock()
	uuids := make([]string, 0, len(nm.nodeConnections))
	for uuid := range nm.nodeConnections {
		uuids = append(uuids, uuid)
	}
	return uuids
}

// CheckConnectionHealth 检查连接池健康状态
func (nm *NodeManager) CheckConnectionHealth() {
	nm.nodeConnectionsMu.RLock()
	brokenConnections := make([]string, 0)
	for uuid, conn := range nm.nodeConnections {
		if conn == nil {
			brokenConnections = append(brokenConnections, uuid)
			continue
		}

		// 尝试调用一个简单的 RPC 来验证连接
		client, exists := nm.connectedNodes[uuid]
		if exists {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			_, err := client.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
			cancel()
			if err != nil {
				brokenConnections = append(brokenConnections, uuid)
				log.Printf("⚠️ 检测到连接池中节点 %s 连接已断开: %v", uuid, err)
			}
		}
	}
	nm.nodeConnectionsMu.RUnlock()

	// 清理断开的连接
	if len(brokenConnections) > 0 {
		nm.nodeConnectionsMu.Lock()
		nm.connectedNodesMu.Lock()
		for _, uuid := range brokenConnections {
			if conn, exists := nm.nodeConnections[uuid]; exists && conn != nil {
				conn.Close()
			}
			delete(nm.nodeConnections, uuid)
			delete(nm.connectedNodes, uuid)
			log.Printf("🧹 从连接池中移除断开的连接: %s", uuid)
		}
		nm.connectedNodesMu.Unlock()
		nm.nodeConnectionsMu.Unlock()
	}
}

// StartConnectionHealthCheck 启动连接健康检查
func (nm *NodeManager) StartConnectionHealthCheck(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nm.CheckConnectionHealth()
			log.Printf("📊 连接池状态: 已连接 %d 个服务器节点", nm.GetConnectionPoolSize())
		}
	}
}

// Close 关闭所有连接（清理连接池）
func (nm *NodeManager) Close() {
	log.Printf("🔌 正在关闭连接池，共有 %d 个连接", nm.GetConnectionPoolSize())

	nm.nodeConnectionsMu.Lock()
	for uuid, conn := range nm.nodeConnections {
		if conn != nil {
			conn.Close()
			log.Printf("🔌 已关闭连接: %s", uuid)
		}
	}
	nm.nodeConnections = make(map[string]*grpc.ClientConn)
	nm.nodeConnectionsMu.Unlock()

	nm.connectedNodesMu.Lock()
	nm.connectedNodes = make(map[string]tasksmanager.TasksManagerClient)
	nm.connectedNodesMu.Unlock()

	log.Printf("✅ 连接池已关闭")
}
