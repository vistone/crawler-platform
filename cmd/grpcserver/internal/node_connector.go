package grpcserver

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NodeConnector 节点连接管理器
// 负责自动发现和连接到其他节点
type NodeConnector struct {
	// 本节点信息
	nodeID  string
	address string
	port    string

	// 已连接的节点客户端映射 (nodeUUID -> client)
	connectedNodes   map[string]tasksmanager.TasksManagerClient
	connectedNodesMu sync.RWMutex

	// 节点连接映射 (nodeUUID -> grpc.Conn)
	nodeConnections   map[string]*grpc.ClientConn
	nodeConnectionsMu sync.RWMutex

	// 已知节点列表
	knownNodes   map[string]*tasksmanager.GrpcServerNodeInfo
	knownNodesMu sync.RWMutex

	// 日志记录器
	logger logger.Logger

	// 停止通道
	stopChan chan struct{}
}

// NewNodeConnector 创建新的节点连接管理器
func NewNodeConnector(nodeID, address, port string) *NodeConnector {
	return &NodeConnector{
		nodeID:          nodeID,
		address:         address,
		port:            port,
		connectedNodes:  make(map[string]tasksmanager.TasksManagerClient),
		nodeConnections: make(map[string]*grpc.ClientConn),
		knownNodes:      make(map[string]*tasksmanager.GrpcServerNodeInfo),
		logger:          logger.GetGlobalLogger(),
		stopChan:        make(chan struct{}),
	}
}

// Start 启动节点连接管理器
// 开始自动发现和连接其他节点
func (nc *NodeConnector) Start() {
	go nc.autoDiscoverAndConnect()
}

// Bootstrap 引导连接到已知节点
// 连接到指定的引导节点，获取节点列表，然后连接到所有已知节点
func (nc *NodeConnector) Bootstrap(bootstrapAddresses []string) error {
	if len(bootstrapAddresses) == 0 {
		return nil // 没有引导节点，跳过
	}

	nc.logger.Info("开始引导连接，引导节点数量: %d", len(bootstrapAddresses))

	// 连接到第一个可用的引导节点
	for _, addr := range bootstrapAddresses {
		// 如果引导节点地址包含 0.0.0.0，替换为 localhost
		actualAddr := addr
		if strings.Contains(addr, "0.0.0.0") {
			actualAddr = strings.Replace(addr, "0.0.0.0", "127.0.0.1", 1)
			nc.logger.Info("引导节点地址 %s 转换为 %s", addr, actualAddr)
		}

		nc.logger.Info("尝试连接到引导节点: %s", actualAddr)

		conn, err := grpc.NewClient(actualAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			nc.logger.Warn("连接引导节点失败 %s: %v", addr, err)
			continue
		}

		client := tasksmanager.NewTasksManagerClient(conn)

		// 获取节点列表
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		nodeListResp, err := client.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
		cancel()

		if err != nil {
			conn.Close()
			nc.logger.Warn("从引导节点获取节点列表失败: %v", err)
			continue
		}

		nc.logger.Info("从引导节点获取到 %d 个节点", len(nodeListResp.Items))

		// 连接到所有发现的节点
		for _, node := range nodeListResp.Items {
			if node.NodeUuid != nc.nodeID {
				nc.knownNodesMu.Lock()
				nc.knownNodes[node.NodeUuid] = node
				nc.knownNodesMu.Unlock()

				// 异步连接到节点
				go nc.ConnectToNode(node)
			}
		}

		// 向引导节点注册自己
		hostname, _ := GetRealHostname()
		sysInfo, _ := GetAllSystemInfo()

		nodeInfo := &tasksmanager.GrpcServerNodeInfo{
			NodeUuid:           nc.nodeID,
			NodeName:           hostname,
			NodeIp:             nc.address,
			NodePort:           nc.port,
			NodeSystem:         sysInfo.SystemInfo,
			NodeVersion:        "1.0.0",
			NodeCpu:            sysInfo.CPUInfo,
			NodeMemory:         sysInfo.MemoryInfo,
			NodeCreateTime:     time.Now().Format(time.RFC3339),
			NodeLastActiveTime: time.Now().Format(time.RFC3339),
		}

		regReq := &tasksmanager.NodeRegistrationRequest{
			NodeInfo:   nodeInfo,
			KnownNodes: []string{},
		}

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		regResp, err := client.RegisterNode(ctx, regReq)
		cancel()

		if err == nil && regResp.Success {
			nc.logger.Info("已向引导节点注册，发现 %d 个已知节点", len(regResp.KnownNodes))

			// 连接到注册响应中的已知节点
			for _, node := range regResp.KnownNodes {
				if node.NodeUuid != nc.nodeID {
					nc.knownNodesMu.Lock()
					nc.knownNodes[node.NodeUuid] = node
					nc.knownNodesMu.Unlock()

					go nc.ConnectToNode(node)
				}
			}
		}

		// 保存引导节点的连接（可选）
		nc.nodeConnectionsMu.Lock()
		nc.nodeConnections["bootstrap-"+actualAddr] = conn
		nc.nodeConnectionsMu.Unlock()

		nc.connectedNodesMu.Lock()
		nc.connectedNodes["bootstrap-"+actualAddr] = client
		nc.connectedNodesMu.Unlock()

		nc.logger.Info("引导连接成功，已连接到引导节点: %s", actualAddr)
		return nil // 成功连接到一个引导节点即可
	}

	return fmt.Errorf("无法连接到任何引导节点")
}

// Stop 停止节点连接管理器
func (nc *NodeConnector) Stop() {
	close(nc.stopChan)

	// 关闭所有连接
	nc.nodeConnectionsMu.Lock()
	for _, conn := range nc.nodeConnections {
		conn.Close()
	}
	nc.nodeConnectionsMu.Unlock()
}

// ConnectToNode 连接到指定的节点
func (nc *NodeConnector) ConnectToNode(nodeInfo *tasksmanager.GrpcServerNodeInfo) error {
	// 不连接自己
	if nodeInfo.NodeUuid == nc.nodeID {
		return nil
	}

	// 检查是否已连接（连接复用）
	nc.connectedNodesMu.RLock()
	if _, exists := nc.connectedNodes[nodeInfo.NodeUuid]; exists {
		// 检查连接是否仍然有效
		nc.nodeConnectionsMu.RLock()
		conn, connExists := nc.nodeConnections[nodeInfo.NodeUuid]
		nc.nodeConnectionsMu.RUnlock()

		if connExists && conn != nil {
			// 连接存在且有效，复用连接
			nc.connectedNodesMu.RUnlock()
			nc.logger.Debug("节点已连接，复用现有连接: %s", nodeInfo.NodeUuid)
			return nil
		} else {
			// 连接已断开，清理并重新连接
			nc.connectedNodesMu.RUnlock()
			nc.connectedNodesMu.Lock()
			delete(nc.connectedNodes, nodeInfo.NodeUuid)
			nc.connectedNodesMu.Unlock()
			nc.nodeConnectionsMu.Lock()
			delete(nc.nodeConnections, nodeInfo.NodeUuid)
			nc.nodeConnectionsMu.Unlock()
			nc.logger.Warn("检测到节点 %s 连接已断开，将重新连接", nodeInfo.NodeUuid)
		}
	} else {
		nc.connectedNodesMu.RUnlock()
	}

	// 构建节点地址
	// 如果节点 IP 是 0.0.0.0，则使用 localhost（因为 0.0.0.0 是监听地址，不能用于连接）
	nodeIP := nodeInfo.NodeIp
	if nodeIP == "0.0.0.0" || nodeIP == "" {
		nodeIP = "127.0.0.1"
	}
	nodeAddr := fmt.Sprintf("%s:%s", nodeIP, nodeInfo.NodePort)

	nc.logger.Info("正在连接到新节点: %s (%s)", nodeInfo.NodeUuid, nodeAddr)

	// 建立 gRPC 连接
	conn, err := grpc.NewClient(nodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("连接节点失败 %s: %w", nodeAddr, err)
	}

	// 创建客户端
	client := tasksmanager.NewTasksManagerClient(conn)

	// 保存连接和客户端
	nc.nodeConnectionsMu.Lock()
	nc.nodeConnections[nodeInfo.NodeUuid] = conn
	nc.nodeConnectionsMu.Unlock()

	nc.connectedNodesMu.Lock()
	nc.connectedNodes[nodeInfo.NodeUuid] = client
	nc.connectedNodesMu.Unlock()

	// 添加到已知节点列表
	nc.knownNodesMu.Lock()
	nc.knownNodes[nodeInfo.NodeUuid] = nodeInfo
	nc.knownNodesMu.Unlock()

	nc.logger.Info("成功连接到节点: %s (%s)", nodeInfo.NodeUuid, nodeAddr)

	return nil
}

// OnNewNodeDiscovered 当发现新节点时调用
func (nc *NodeConnector) OnNewNodeDiscovered(nodeInfo *tasksmanager.GrpcServerNodeInfo) {
	// 检查是否已知
	nc.knownNodesMu.RLock()
	_, known := nc.knownNodes[nodeInfo.NodeUuid]
	nc.knownNodesMu.RUnlock()

	if !known {
		nc.logger.Info("发现新节点: %s (%s:%s)", nodeInfo.NodeUuid, nodeInfo.NodeIp, nodeInfo.NodePort)
		// 自动连接到新节点
		go nc.ConnectToNode(nodeInfo)
	}
}

// OnNodesDiscovered 批量处理发现的节点
func (nc *NodeConnector) OnNodesDiscovered(nodes []*tasksmanager.GrpcServerNodeInfo) {
	for _, node := range nodes {
		nc.OnNewNodeDiscovered(node)
	}
}

// GetConnectedNodes 获取已连接的节点客户端
func (nc *NodeConnector) GetConnectedNodes() map[string]tasksmanager.TasksManagerClient {
	nc.connectedNodesMu.RLock()
	defer nc.connectedNodesMu.RUnlock()

	result := make(map[string]tasksmanager.TasksManagerClient)
	for k, v := range nc.connectedNodes {
		result[k] = v
	}
	return result
}

// autoDiscoverAndConnect 自动发现和连接节点
func (nc *NodeConnector) autoDiscoverAndConnect() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-nc.stopChan:
			return
		case <-ticker.C:
			// 定期同步已知节点列表
			nc.knownNodesMu.RLock()
			nodes := make([]*tasksmanager.GrpcServerNodeInfo, 0, len(nc.knownNodes))
			for _, node := range nc.knownNodes {
				if node.NodeUuid != nc.nodeID {
					nodes = append(nodes, node)
				}
			}
			nc.knownNodesMu.RUnlock()

			// 尝试连接未连接的节点
			for _, node := range nodes {
				nc.connectedNodesMu.RLock()
				_, connected := nc.connectedNodes[node.NodeUuid]
				nc.connectedNodesMu.RUnlock()

				if !connected {
					nc.ConnectToNode(node)
				}
			}
		}
	}
}
