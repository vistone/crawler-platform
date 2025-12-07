package grpcserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// NodeConnector 节点连接管理器
// 负责自动发现和连接到其他节点
type NodeConnector struct {
	// 本节点信息
	nodeID  string
	address string
	port    string

	// TLS 配置（用于连接到其他节点）
	tlsConfig *tls.Config

	// 已连接的节点客户端映射 (nodeAddr -> client)，使用 IP:Port 作为 key
	connectedNodes   map[string]tasksmanager.TasksManagerClient
	connectedNodesMu sync.RWMutex

	// 节点连接映射 (nodeAddr -> grpc.Conn)，使用 IP:Port 作为 key
	nodeConnections   map[string]*grpc.ClientConn
	nodeConnectionsMu sync.RWMutex

	// 已知节点列表 (nodeAddr -> nodeInfo)，使用 IP:Port 作为 key
	knownNodes   map[string]*tasksmanager.GrpcServerNodeInfo
	knownNodesMu sync.RWMutex

	// 日志记录器
	logger logger.Logger

	// 停止通道
	stopChan chan struct{}
}

// NewNodeConnector 创建新的节点连接管理器（保持向后兼容）
func NewNodeConnector(nodeID, address, port string) *NodeConnector {
	return NewNodeConnectorWithTLS(nodeID, address, port, nil)
}

// NewNodeConnectorWithTLS 创建新的节点连接管理器（带 TLS 配置）
func NewNodeConnectorWithTLS(nodeID, address, port string, tlsConfig *tls.Config) *NodeConnector {
	return &NodeConnector{
		nodeID:          nodeID,
		address:         address,
		port:            port,
		tlsConfig:       tlsConfig,
		connectedNodes:  make(map[string]tasksmanager.TasksManagerClient),
		nodeConnections: make(map[string]*grpc.ClientConn),
		knownNodes:      make(map[string]*tasksmanager.GrpcServerNodeInfo),
		logger:          logger.GetGlobalLogger(),
		stopChan:        make(chan struct{}),
	}
}

// getNodeAddr 获取节点的地址标识符（IP:Port）
// 这是节点的唯一标识，而不是 UUID
func (nc *NodeConnector) getNodeAddr(nodeInfo *tasksmanager.GrpcServerNodeInfo) string {
	nodeIP := nodeInfo.NodeIp
	if nodeIP == "0.0.0.0" || nodeIP == "" {
		nodeIP = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%s", nodeIP, nodeInfo.NodePort)
}

// getSelfAddr 获取本节点的地址标识符
func (nc *NodeConnector) getSelfAddr() string {
	addr := nc.address
	if addr == "0.0.0.0" || addr == "" {
		addr = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%s", addr, nc.port)
}

// Start 启动节点连接管理器
// 开始自动发现和连接其他节点，并启动心跳发送
func (nc *NodeConnector) Start() {
	go nc.autoDiscoverAndConnect()
	go nc.startNodeHeartbeat()
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

		// 检查地址是否包含端口，如果没有则添加默认端口
		if !strings.Contains(actualAddr, ":") {
			// 地址中没有端口，添加默认端口
			actualAddr = actualAddr + ":50051"
			nc.logger.Info("引导节点地址缺少端口，添加默认端口 50051: %s", actualAddr)
		}

		nc.logger.Info("尝试连接到引导节点: %s", actualAddr)

		// 选择传输凭证（根据 TLS 配置）
		var transportCreds credentials.TransportCredentials
		if nc.tlsConfig != nil {
			// 创建客户端 TLS 配置，跳过证书验证（因为使用自签名证书）
			clientTLSConfig := nc.tlsConfig.Clone()
			clientTLSConfig.InsecureSkipVerify = true // 跳过证书验证，允许自签名证书
			transportCreds = credentials.NewTLS(clientTLSConfig)
			nc.logger.Debug("使用 TLS 连接到引导节点: %s (跳过证书验证)", actualAddr)
		} else {
			transportCreds = insecure.NewCredentials()
			nc.logger.Debug("使用非加密连接（insecure）连接到引导节点: %s", actualAddr)
		}

		// 配置 gRPC keepalive 参数，保持连接活跃
		keepaliveParams := keepalive.ClientParameters{
			Time:                10 * time.Second, // 每 10 秒发送一次 keepalive ping
			Timeout:             3 * time.Second,  // keepalive ping 超时时间
			PermitWithoutStream: true,             // 即使没有活跃的流也发送 keepalive ping
		}

		conn, err := grpc.NewClient(actualAddr,
			grpc.WithTransportCredentials(transportCreds),
			grpc.WithKeepaliveParams(keepaliveParams))
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
			nodeAddr := nc.getNodeAddr(node)
			selfAddr := nc.getSelfAddr()
			if nodeAddr != selfAddr {
				nc.knownNodesMu.Lock()
				nc.knownNodes[nodeAddr] = node
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
				nodeAddr := nc.getNodeAddr(node)
				selfAddr := nc.getSelfAddr()
				if nodeAddr != selfAddr {
					nc.knownNodesMu.Lock()
					nc.knownNodes[nodeAddr] = node
					nc.knownNodesMu.Unlock()

					go nc.ConnectToNode(node)
				}
			}
		}

		// 保存引导节点的连接（使用 IP:Port 作为 key）
		nc.nodeConnectionsMu.Lock()
		nc.nodeConnections[actualAddr] = conn
		nc.nodeConnectionsMu.Unlock()

		nc.connectedNodesMu.Lock()
		nc.connectedNodes[actualAddr] = client
		nc.connectedNodesMu.Unlock()

		// 尝试获取引导节点的信息并添加到 knownNodes
		bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 3*time.Second)
		bootstrapNodeListResp, bootstrapErr := client.GetGrpcServerNodeInfoList(bootstrapCtx, &tasksmanager.GrpcServerNodeInfoListRequest{})
		bootstrapCancel()
		if bootstrapErr == nil && bootstrapNodeListResp != nil {
			// 查找匹配的节点（需要处理 IP 地址格式差异）
			found := false
			for _, node := range bootstrapNodeListResp.Items {
				checkAddr := nc.getNodeAddr(node)
				// 直接匹配
				if checkAddr == actualAddr {
					// 找到匹配的节点，添加到 knownNodes
					nc.knownNodesMu.Lock()
					nc.knownNodes[actualAddr] = node
					nc.knownNodesMu.Unlock()
					nc.logger.Debug("引导节点 %s 已添加到已知列表 (UUID: %s)", actualAddr, node.NodeUuid)
					found = true
					break
				}
				// 尝试解析地址，处理端口匹配但 IP 格式不同的情况
				// 例如：0.0.0.0:50051 vs 45.78.5.252:50051
				if strings.Contains(actualAddr, ":") && strings.Contains(checkAddr, ":") {
					addrParts := strings.Split(actualAddr, ":")
					checkParts := strings.Split(checkAddr, ":")
					if len(addrParts) == 2 && len(checkParts) == 2 {
						// 端口匹配，IP 可能是 0.0.0.0 或实际 IP
						if addrParts[1] == checkParts[1] {
							// 端口匹配，使用 actualAddr 作为 key（因为这是实际连接的地址）
							nc.knownNodesMu.Lock()
							nc.knownNodes[actualAddr] = node
							nc.knownNodesMu.Unlock()
							nc.logger.Debug("引导节点 %s 已添加到已知列表（通过端口匹配）(UUID: %s, 节点报告地址: %s)", actualAddr, node.NodeUuid, checkAddr)
							found = true
							break
						}
					}
				}
			}
			if !found {
				nc.logger.Debug("引导节点 %s 未在节点列表中找到匹配项（返回了 %d 个节点），将使用连接地址作为标识", actualAddr, len(bootstrapNodeListResp.Items))
				// 如果找不到匹配的节点，创建一个临时节点信息
				tempNode := &tasksmanager.GrpcServerNodeInfo{
					NodeUuid:           "unknown",
					NodeIp:             strings.Split(actualAddr, ":")[0],
					NodePort:           strings.Split(actualAddr, ":")[1],
					NodeLastActiveTime: time.Now().Format(time.RFC3339),
				}
				nc.knownNodesMu.Lock()
				nc.knownNodes[actualAddr] = tempNode
				nc.knownNodesMu.Unlock()
			}
		} else {
			// 如果获取节点列表失败，创建一个临时节点信息
			nc.logger.Debug("无法获取引导节点 %s 的节点信息，创建临时节点信息", actualAddr)
			tempNode := &tasksmanager.GrpcServerNodeInfo{
				NodeUuid:           "unknown",
				NodeIp:             strings.Split(actualAddr, ":")[0],
				NodePort:           strings.Split(actualAddr, ":")[1],
				NodeLastActiveTime: time.Now().Format(time.RFC3339),
			}
			nc.knownNodesMu.Lock()
			nc.knownNodes[actualAddr] = tempNode
			nc.knownNodesMu.Unlock()
		}

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
	// 获取节点地址（IP:Port），这是节点的唯一标识
	nodeAddr := nc.getNodeAddr(nodeInfo)
	selfAddr := nc.getSelfAddr()

	// 不连接自己
	if nodeAddr == selfAddr {
		return nil
	}

	// 检查是否已连接（连接复用）
	nc.connectedNodesMu.RLock()
	if _, exists := nc.connectedNodes[nodeAddr]; exists {
		// 检查连接是否仍然有效
		nc.nodeConnectionsMu.RLock()
		conn, connExists := nc.nodeConnections[nodeAddr]
		nc.nodeConnectionsMu.RUnlock()

		if connExists && conn != nil {
			// 连接存在且有效，复用连接
			nc.connectedNodesMu.RUnlock()
			nc.logger.Debug("节点已连接，复用现有连接: %s (%s)", nodeAddr, nodeInfo.NodeUuid)
			return nil
		} else {
			// 连接已断开，清理并重新连接
			nc.connectedNodesMu.RUnlock()
			nc.connectedNodesMu.Lock()
			delete(nc.connectedNodes, nodeAddr)
			nc.connectedNodesMu.Unlock()
			nc.nodeConnectionsMu.Lock()
			delete(nc.nodeConnections, nodeAddr)
			nc.nodeConnectionsMu.Unlock()
			nc.logger.Warn("检测到节点 %s (%s) 连接已断开，将重新连接", nodeAddr, nodeInfo.NodeUuid)
		}
	} else {
		nc.connectedNodesMu.RUnlock()
	}

	nc.logger.Info("正在连接到新节点: %s (UUID: %s)", nodeAddr, nodeInfo.NodeUuid)

	// 选择传输凭证（根据 TLS 配置）
	var transportCreds credentials.TransportCredentials
	if nc.tlsConfig != nil {
		// 创建客户端 TLS 配置，跳过证书验证（因为使用自签名证书）
		clientTLSConfig := nc.tlsConfig.Clone()
		clientTLSConfig.InsecureSkipVerify = true // 跳过证书验证，允许自签名证书
		transportCreds = credentials.NewTLS(clientTLSConfig)
		nc.logger.Debug("使用 TLS 连接到节点: %s (跳过证书验证)", nodeAddr)
	} else {
		transportCreds = insecure.NewCredentials()
		nc.logger.Debug("使用非加密连接（insecure）连接到节点: %s", nodeAddr)
	}

	// 配置 gRPC keepalive 参数，保持连接活跃
	keepaliveParams := keepalive.ClientParameters{
		Time:                10 * time.Second, // 每 10 秒发送一次 keepalive ping
		Timeout:             3 * time.Second,  // keepalive ping 超时时间
		PermitWithoutStream: true,             // 即使没有活跃的流也发送 keepalive ping
	}

	// 建立 gRPC 连接
	conn, err := grpc.NewClient(nodeAddr,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithKeepaliveParams(keepaliveParams))
	if err != nil {
		return fmt.Errorf("连接节点失败 %s: %w", nodeAddr, err)
	}

	// 创建客户端
	client := tasksmanager.NewTasksManagerClient(conn)

	// 保存连接和客户端（使用 IP:Port 作为 key）
	nc.nodeConnectionsMu.Lock()
	nc.nodeConnections[nodeAddr] = conn
	nc.nodeConnectionsMu.Unlock()

	nc.connectedNodesMu.Lock()
	nc.connectedNodes[nodeAddr] = client
	nc.connectedNodesMu.Unlock()

	// 添加到已知节点列表（使用 IP:Port 作为 key）
	nc.knownNodesMu.Lock()
	nc.knownNodes[nodeAddr] = nodeInfo
	nc.knownNodesMu.Unlock()

	nc.logger.Info("成功连接到节点: %s (UUID: %s)", nodeAddr, nodeInfo.NodeUuid)

	return nil
}

// OnNewNodeDiscovered 当发现新节点时调用
func (nc *NodeConnector) OnNewNodeDiscovered(nodeInfo *tasksmanager.GrpcServerNodeInfo) {
	// 获取节点地址（IP:Port），这是节点的唯一标识
	nodeAddr := nc.getNodeAddr(nodeInfo)
	selfAddr := nc.getSelfAddr()

	// 不连接自己
	if nodeAddr == selfAddr {
		return
	}

	// 检查是否已知（使用 IP:Port 作为 key）
	nc.knownNodesMu.RLock()
	_, known := nc.knownNodes[nodeAddr]
	nc.knownNodesMu.RUnlock()

	if !known {
		nc.logger.Info("发现新节点: %s (UUID: %s)", nodeAddr, nodeInfo.NodeUuid)
		// 先添加到已知列表（防止重复连接）
		nc.knownNodesMu.Lock()
		nc.knownNodes[nodeAddr] = nodeInfo
		nc.knownNodesMu.Unlock()
		// 自动连接到新节点
		go nc.ConnectToNode(nodeInfo)
	} else {
		// 节点已知道，但 UUID 可能已更新，更新节点信息
		nc.knownNodesMu.Lock()
		nc.knownNodes[nodeAddr] = nodeInfo
		nc.knownNodesMu.Unlock()
		nc.logger.Debug("节点 %s 已存在，更新节点信息 (UUID: %s -> %s)", nodeAddr, nc.knownNodes[nodeAddr].NodeUuid, nodeInfo.NodeUuid)
	}
}

// OnNodesDiscovered 批量处理发现的节点
func (nc *NodeConnector) OnNodesDiscovered(nodes []*tasksmanager.GrpcServerNodeInfo) {
	for _, node := range nodes {
		nc.OnNewNodeDiscovered(node)
	}
}

// GetConnectedNodes 获取已连接的节点客户端
// 返回的 map key 是 IP:Port（节点的唯一标识）
func (nc *NodeConnector) GetConnectedNodes() map[string]tasksmanager.TasksManagerClient {
	nc.connectedNodesMu.RLock()
	defer nc.connectedNodesMu.RUnlock()

	result := make(map[string]tasksmanager.TasksManagerClient)
	for k, v := range nc.connectedNodes {
		result[k] = v
	}
	return result
}

// startNodeHeartbeat 启动节点心跳发送
// 定期向所有已连接的节点发送心跳，保持连接活跃并更新节点状态
func (nc *NodeConnector) startNodeHeartbeat() {
	ticker := time.NewTicker(30 * time.Second) // 每 30 秒发送一次心跳
	defer ticker.Stop()

	for {
		select {
		case <-nc.stopChan:
			return
		case <-ticker.C:
			// 获取系统信息（每次心跳都获取最新信息）
			hostname, _ := GetRealHostname()
			sysInfo, err := GetAllSystemInfo()
			if err != nil {
				nc.logger.Warn("获取系统信息失败: %v，使用默认值", err)
				sysInfo = &SystemInfo{
					Hostname:   hostname,
					SystemInfo: GetRealSystemInfo(),
					CPUInfo:    GetRealCPUInfo(),
					MemoryInfo: GetRealMemoryInfo(),
				}
			}

			// 获取所有已连接的节点
			nc.connectedNodesMu.RLock()
			connectedNodes := make(map[string]tasksmanager.TasksManagerClient)
			for nodeAddr, client := range nc.connectedNodes {
				connectedNodes[nodeAddr] = client
			}
			nc.connectedNodesMu.RUnlock()

			// 向每个节点发送心跳
			for nodeAddr, client := range connectedNodes {
				go func(addr string, c tasksmanager.TasksManagerClient) {
					// 获取节点信息（用于日志）
					nc.knownNodesMu.RLock()
					remoteNodeInfo, exists := nc.knownNodes[addr]
					nc.knownNodesMu.RUnlock()
					if !exists {
						// 节点在 connectedNodes 中但不在 knownNodes 中，说明状态不一致
						// 尝试从连接中获取节点信息
						nc.logger.Warn("节点 %s 在连接列表中但不在已知列表中，尝试修复...", addr)

						// 尝试通过 GetGrpcServerNodeInfoList 获取节点信息
						repairCtx, repairCancel := context.WithTimeout(context.Background(), 3*time.Second)
						repairResp, repairErr := c.GetGrpcServerNodeInfoList(repairCtx, &tasksmanager.GrpcServerNodeInfoListRequest{})
						repairCancel()

						if repairErr == nil && repairResp != nil {
							// 查找匹配的节点（需要处理 IP 地址格式差异）
							found := false
							for _, node := range repairResp.Items {
								checkAddr := nc.getNodeAddr(node)
								// 直接匹配
								if checkAddr == addr {
									// 找到匹配的节点，添加到 knownNodes
									nc.knownNodesMu.Lock()
									nc.knownNodes[addr] = node
									nc.knownNodesMu.Unlock()
									nc.logger.Info("已修复节点 %s 的状态，添加到已知列表 (UUID: %s)", addr, node.NodeUuid)
									remoteNodeInfo = node
									exists = true
									found = true
									break
								}
								// 尝试解析地址，处理端口匹配但 IP 格式不同的情况
								// 例如：0.0.0.0:50051 vs 45.78.5.252:50051
								if strings.Contains(addr, ":") && strings.Contains(checkAddr, ":") {
									addrParts := strings.Split(addr, ":")
									checkParts := strings.Split(checkAddr, ":")
									if len(addrParts) == 2 && len(checkParts) == 2 {
										// 端口匹配，IP 可能是 0.0.0.0 或实际 IP
										if addrParts[1] == checkParts[1] {
											// 端口匹配，使用 addr 作为 key（因为这是实际连接的地址）
											nc.knownNodesMu.Lock()
											nc.knownNodes[addr] = node
											nc.knownNodesMu.Unlock()
											nc.logger.Info("已修复节点 %s 的状态（通过端口匹配），添加到已知列表 (UUID: %s, 节点报告地址: %s)", addr, node.NodeUuid, checkAddr)
											remoteNodeInfo = node
											exists = true
											found = true
											break
										}
									}
								}
							}
							if !found {
								nc.logger.Warn("无法修复节点 %s 的状态：在节点列表中未找到匹配的节点（返回了 %d 个节点）", addr, len(repairResp.Items))
							}
						} else {
							nc.logger.Warn("无法修复节点 %s 的状态：获取节点列表失败: %v", addr, repairErr)
						}

						if !exists {
							nc.logger.Warn("无法修复节点 %s 的状态，跳过心跳", addr)
							return
						}
					}
					// 构建心跳请求（本节点的信息）
					selfNodeInfo := &tasksmanager.GrpcServerNodeInfo{
						NodeUuid:           nc.nodeID,
						NodeName:           hostname,
						NodeIp:             nc.address,
						NodePort:           nc.port,
						NodeSystem:         sysInfo.SystemInfo,
						NodeVersion:        "2.0.0",
						NodeCpu:            sysInfo.CPUInfo,
						NodeMemory:         sysInfo.MemoryInfo,
						NodeCreateTime:     time.Now().Format(time.RFC3339),
						NodeLastActiveTime: time.Now().Format(time.RFC3339),
					}

					req := &tasksmanager.NodeHeartbeatRequest{
						NodeUuid:  nc.nodeID,
						NodeInfo:  selfNodeInfo,
						Timestamp: time.Now().UnixMilli(),
					}

					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()

					resp, err := c.NodeHeartbeat(ctx, req)
					if err != nil {
						nc.logger.Debug("向节点 %s (UUID: %s) 发送心跳失败: %v", addr, remoteNodeInfo.NodeUuid, err)
						// 心跳失败，可能连接已断开，但不立即移除（等待连接检查）
						return
					}

					if resp.Success {
						nc.logger.Debug("向节点 %s (UUID: %s) 发送心跳成功", addr, remoteNodeInfo.NodeUuid)

						// 处理响应中的新节点信息
						if len(resp.UpdatedNodes) > 0 {
							nc.logger.Info("心跳响应中发现 %d 个新节点", len(resp.UpdatedNodes))
							nc.OnNodesDiscovered(resp.UpdatedNodes)
						}
					}
				}(nodeAddr, client)
			}
		}
	}
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
			selfAddr := nc.getSelfAddr()
			nc.knownNodesMu.RLock()
			nodes := make([]*tasksmanager.GrpcServerNodeInfo, 0, len(nc.knownNodes))
			for nodeAddr, node := range nc.knownNodes {
				if nodeAddr != selfAddr {
					nodes = append(nodes, node)
				}
			}
			nc.knownNodesMu.RUnlock()

			// 尝试连接未连接的节点
			for _, node := range nodes {
				nodeAddr := nc.getNodeAddr(node)
				nc.connectedNodesMu.RLock()
				_, connected := nc.connectedNodes[nodeAddr]
				nc.connectedNodesMu.RUnlock()

				if !connected {
					nc.ConnectToNode(node)
				}
			}
		}
	}
}
