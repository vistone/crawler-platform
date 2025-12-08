package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 解析命令行参数（配置文件路径和其他覆盖选项）
	configPath := flag.String("config", "", "配置文件路径（默认: ./cmd/grpcclient/config.toml 或 ./config.toml）")
	protocolType := flag.String("protocol", "", "协议类型: grpc, tuic（覆盖配置文件）")
	serverAddr := flag.String("server", "", "服务器地址（覆盖配置文件）")
	clientName := flag.String("name", "", "客户端名称（覆盖配置文件）")
	certsDir := flag.String("certs", "", "证书目录路径（覆盖配置文件）")
	insecureMode := flag.Bool("insecure", false, "使用非加密连接（覆盖配置文件，仅 gRPC）")
	tuicUUID := flag.String("uuid", "", "TUIC UUID（覆盖配置文件，用于真正的 TUIC 协议）")
	tuicPassword := flag.String("password", "", "TUIC 密码（覆盖配置文件，用于真正的 TUIC 协议）")
	tileKey := flag.String("tilekey", "", "瓦片键（覆盖配置文件）")
	epoch := flag.Int("epoch", 0, "主版本号（覆盖配置文件，0 表示使用配置文件的值）")
	taskType := flag.String("tasktype", "", "任务类型（覆盖配置文件）")
	repeatCount := flag.Int("repeat", 0, "重复请求次数（覆盖配置文件，0 表示使用配置文件的值）")
	concurrency := flag.Int("concurrency", 0, "并发请求数量（覆盖配置文件，0 表示使用配置文件的值）")
	flag.Parse()

	// 加载配置文件
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置验证失败: %v", err)
	}

	// 命令行参数覆盖配置文件（如果提供了）
	if *protocolType != "" {
		cfg.Protocol.Type = *protocolType
	}
	if *serverAddr != "" {
		cfg.Server.Address = *serverAddr
	}
	if *clientName != "" {
		cfg.Client.Name = *clientName
	}
	if *certsDir != "" {
		cfg.Server.CertsDir = *certsDir
	}
	if *insecureMode {
		cfg.Server.Insecure = true
	}
	if *tuicUUID != "" {
		cfg.Server.UUID = *tuicUUID
	}
	if *tuicPassword != "" {
		cfg.Server.Password = *tuicPassword
	}
	if *tileKey != "" {
		cfg.Task.TileKey = *tileKey
	}
	if *epoch > 0 {
		cfg.Task.Epoch = int32(*epoch)
	}
	if *taskType != "" {
		cfg.Task.TaskType = *taskType
	}
	if *repeatCount > 0 {
		cfg.Task.RepeatCount = *repeatCount
	}
	if *concurrency > 0 {
		cfg.Task.Concurrency = *concurrency
	}

	// 初始化日志记录器（根据配置文件）
	logger.InitGlobalLogger(logger.NewConsoleLogger(
		cfg.Logger.EnableDebug,
		cfg.Logger.EnableInfo,
		cfg.Logger.EnableWarn,
		cfg.Logger.EnableError,
	))

	if *configPath != "" {
		log.Printf("已加载配置文件: %s", *configPath)
	} else {
		log.Println("使用默认配置（可通过 -config 指定配置文件）")
	}

	ctx := context.Background()

	var client tasksmanager.TasksManagerClient
	var conn *grpc.ClientConn
	var dualClient *DualProtocolClient

	// 根据协议类型创建客户端
	// 如果配置了节点列表，跳过直接连接，只使用节点池
	hasNodeList := len(cfg.Server.Nodes) > 0

	switch cfg.Protocol.Type {
	case "both":
		// 双协议模式：如果配置了节点列表，不创建直接连接，只使用节点池
		if hasNodeList {
			log.Println("📡 检测到节点列表配置，将跳过直接连接，仅使用节点池")
			// 创建一个占位客户端（实际上不会使用）
			client = nil
		} else {
			// 双协议模式：优先使用 TUIC，失败时自动切换到 gRPC
			var err error
			dualClient, err = NewDualProtocolClient(cfg)
			if err != nil {
				log.Fatalf("创建双协议客户端失败: %v", err)
			}
			defer dualClient.Close()
			client = dualClient
		}
	case "tuic":
		// 仅使用 TUIC 客户端
		var tuicClient TUICClient
		if cfg.Server.UUID != "" {
			singBoxClient, err := NewSingBoxTUICClient(cfg.Server.TUICAddress, cfg.Server.UUID, cfg.Server.Password)
			if err != nil {
				log.Printf("创建 sing-box TUIC 客户端失败: %v，将使用 HTTP 接口模式", err)
				tuicClient = NewHTTPTUICClient(cfg.Server.TUICAddress)
				log.Printf("已创建 TUIC 客户端（HTTP 接口模式），连接到: %s", cfg.Server.TUICAddress)
			} else {
				tuicClient = singBoxClient
				log.Printf("已创建 sing-box TUIC 客户端，连接到: %s (UUID: %s)", cfg.Server.TUICAddress, cfg.Server.UUID)
			}
		} else {
			tuicClient = NewHTTPTUICClient(cfg.Server.TUICAddress)
			log.Printf("已创建 TUIC 客户端（HTTP 接口模式），连接到: %s", cfg.Server.TUICAddress)
		}
		client = newTUICClientAdapter(tuicClient)
	case "grpc":
		// 仅使用 gRPC 客户端
		var transportCreds credentials.TransportCredentials
		if cfg.Server.Insecure {
			transportCreds = insecure.NewCredentials()
			log.Printf("使用非加密连接（insecure 模式）")
		} else if cfg.Server.CertsDir != "" {
			tlsConfig, err := LoadTLSConfigFromCertsDir(cfg.Server.CertsDir)
			if err == nil {
				transportCreds = credentials.NewTLS(tlsConfig)
				log.Printf("已加载 TLS 证书，证书目录: %s", cfg.Server.CertsDir)
			} else {
				transportCreds = insecure.NewCredentials()
				log.Printf("加载 TLS 证书失败，使用非加密连接: %v", err)
			}
		} else {
			transportCreds = insecure.NewCredentials()
			log.Printf("未指定证书目录，使用非加密连接")
		}

		var err error
		conn, err = grpc.NewClient(cfg.Server.GRPCAddress, grpc.WithTransportCredentials(transportCreds))
		if err != nil {
			log.Fatalf("连接服务器失败: %v", err)
		}
		defer func() {
			if conn != nil {
				conn.Close()
			}
		}()

		client = tasksmanager.NewTasksManagerClient(conn)
		log.Printf("已创建 gRPC 客户端，连接到: %s", cfg.Server.GRPCAddress)
	default:
		log.Fatalf("不支持的协议类型: %s (支持: grpc, tuic, both)", cfg.Protocol.Type)
	}

	// 提交真实数据请求
	log.Printf("=== 提交真实数据请求 ===")
	log.Printf("任务类型: %s, TileKey: %s, epoch: %d, 重复次数: %d, 并发数: %d",
		cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch, cfg.Task.RepeatCount, cfg.Task.Concurrency)

	// 如果是 gRPC 模式或 both 模式，先注册客户端并初始化节点池
	var nodePool *NodePool
	var clientID string
	if cfg.Protocol.Type == "grpc" || cfg.Protocol.Type == "both" {
		log.Println("\n=== 客户端注册和节点池初始化 ===")

		// 创建节点池管理器
		var nodePoolTLSConfig *tls.Config
		if cfg.Server.CertsDir != "" {
			if config, err := LoadTLSConfigFromCertsDir(cfg.Server.CertsDir); err == nil {
				nodePoolTLSConfig = config
			}
		}
		nodePool = NewNodePool(nodePoolTLSConfig)
		defer nodePool.Close()

		// 如果配置了节点列表，直接连接这些节点
		if len(cfg.Server.Nodes) > 0 {
			log.Printf("📡 使用配置的节点列表，共 %d 个节点", len(cfg.Server.Nodes))
			log.Printf("   将尝试连接到每个节点的 50051（gRPC）和 8443（TUIC）端口")

			// 连接到配置的节点列表
			successCount := 0
			for _, nodeIP := range cfg.Server.Nodes {
				if nodeIP == "" {
					continue
				}
				log.Printf("🔗 正在连接节点: %s", nodeIP)

				// 创建临时节点信息（使用 IP:Port 作为 UUID）
				nodeUUID := fmt.Sprintf("%s:50051", nodeIP)
				nodeInfo := &tasksmanager.GrpcServerNodeInfo{
					NodeUuid: nodeUUID,
					NodeIp:   nodeIP,
					NodePort: "50051",
				}

				// 尝试添加到节点池（会自动连接 gRPC 和获取 TUIC 配置）
				if err := nodePool.AddNode(nodeInfo); err != nil {
					log.Printf("❌ 连接节点 %s 失败: %v", nodeIP, err)
				} else {
					successCount++
					log.Printf("✅ 节点 %s 连接成功", nodeIP)
				}
			}

			log.Printf("📊 节点连接完成: 成功=%d/%d, 节点池总数=%d, 健康=%d",
				successCount, len(cfg.Server.Nodes), nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())

			// 检查节点池是否有可用节点
			if nodePool.GetHealthyNodeCount() == 0 {
				log.Fatalf("❌ 节点池中没有可用节点，无法继续工作")
			}

			// 生成临时 clientID（不需要注册）
			clientID = fmt.Sprintf("client-%s-%d", cfg.Client.Name, time.Now().Unix())
			log.Printf("✅ 节点池准备完成，使用临时客户端 ID: %s", clientID)
		} else {
			// 如果没有配置节点列表，使用服务端发现模式
			// 在 both 模式下，确保使用 gRPC 客户端进行注册（而不是 TUIC 客户端）
			// 因为只有 gRPC 支持返回节点列表
			var registerClient tasksmanager.TasksManagerClient
			if cfg.Protocol.Type == "both" && dualClient != nil {
				registerClient = dualClient.GetGRPCClient()
				log.Println("🔄 both 模式：使用 gRPC 客户端进行注册（用于获取节点列表）")
			} else {
				registerClient = client
			}
			var regResp *tasksmanager.RegisterClientResponse
			var err error
			clientID, regResp, err = testNodeManagementWithResponse(ctx, registerClient, cfg.Client.Name)
			if err != nil {
				log.Printf("客户端注册失败: %v", err)
				return
			}

			// 输出注册响应中的节点信息
			if regResp != nil {
				if regResp.Success {
					log.Printf("✅ 客户端注册成功，响应中包含 %d 个服务器节点", len(regResp.ServerNodes))
				} else {
					log.Printf("⚠️  客户端注册响应 Success=false")
				}
			} else {
				log.Printf("⚠️  客户端注册响应为空")
			}

			// 处理注册响应，自动连接到所有服务器节点
			if regResp != nil && regResp.Success && len(regResp.ServerNodes) > 0 {
				log.Printf("📡 发现 %d 个服务器节点，开始自动连接到节点池", len(regResp.ServerNodes))
				nodePool.AddNodes(regResp.ServerNodes)
				log.Printf("✅ 节点池初始化完成: 总数=%d, 健康=%d", nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())
			} else {
				// 如果注册响应中没有节点列表，尝试通过 GetGrpcServerNodeInfoList 获取
				log.Printf("⚠️  注册响应中未包含节点列表，尝试通过 GetGrpcServerNodeInfoList 获取...")
				nodeListResp, err := registerClient.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
				if err != nil {
					log.Printf("⚠️  获取节点列表失败: %v", err)
				} else if len(nodeListResp.Items) > 0 {
					log.Printf("📡 通过 GetGrpcServerNodeInfoList 发现 %d 个服务器节点", len(nodeListResp.Items))
					// 转换为 ServerNodes 格式
					serverNodes := make([]*tasksmanager.GrpcServerNodeInfo, 0, len(nodeListResp.Items))
					for _, node := range nodeListResp.Items {
						serverNodes = append(serverNodes, node)
					}
					nodePool.AddNodes(serverNodes)
					log.Printf("✅ 节点池初始化完成: 总数=%d, 健康=%d", nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())
				} else {
					log.Printf("⚠️  服务器上暂无可用节点")
				}
			}
		}

		// 启动节点池健康检查
		healthCtx, cancelHealth := context.WithCancel(context.Background())
		defer cancelHealth()
		go nodePool.StartHealthCheck(healthCtx, 30*time.Second)

		// 启动客户端心跳（持续发现新节点）
		// 如果配置了节点列表，不启动心跳（因为不需要发现新节点）
		if !hasNodeList {
			// 在 both 模式下，确保使用 gRPC 客户端进行心跳（而不是 TUIC 客户端）
			// 因为只有 gRPC 支持节点发现功能
			var heartbeatClient tasksmanager.TasksManagerClient
			if cfg.Protocol.Type == "both" && dualClient != nil {
				// both 模式：使用 dualClient 的 gRPC 客户端进行心跳
				// 直接使用 gRPC 客户端（绕过 TUIC 优先逻辑，确保能获取节点信息）
				heartbeatClient = dualClient.GetGRPCClient()
				log.Println("🔄 both 模式：使用 gRPC 客户端进行心跳（用于节点发现）")
			} else {
				heartbeatClient = client
			}
			go startHeartbeatWithNodePool(ctx, heartbeatClient, cfg.Client.Name, clientID, nodePool)
		} else {
			log.Println("📡 使用配置的节点列表，跳过心跳（不需要发现新节点）")
		}

		// 确保节点池有可用节点后才开始任务提交
		if nodePool.GetHealthyNodeCount() == 0 {
			log.Fatalf("❌ 节点池中没有可用节点，无法提交任务")
		}

		// 根据协议类型选择任务提交方式
		// 无论什么模式，都使用节点池进行负载均衡
		log.Printf("🚀 开始提交任务，节点池状态: 总数=%d, 健康=%d", nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())
		if cfg.Task.RepeatCount > 1 {
			if err := submitRealTaskMultipleTimesWithNodePool(ctx, nodePool, cfg.Protocol.Type, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch, cfg.Task.RepeatCount, cfg.Task.Concurrency); err != nil {
				log.Fatalf("批量提交任务失败: %v", err)
			}
		} else {
			if err := submitRealTaskWithNodePool(ctx, nodePool, cfg.Protocol.Type, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch); err != nil {
				log.Fatalf("提交任务失败: %v", err)
			}
		}
	} else {
		// 纯 TUIC 模式，使用原有逻辑
		if cfg.Task.RepeatCount > 1 {
			if err := submitRealTaskMultipleTimes(ctx, client, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch, cfg.Task.RepeatCount, cfg.Task.Concurrency); err != nil {
				log.Fatalf("批量提交任务失败: %v", err)
			}
		} else {
			if err := submitRealTask(ctx, client, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch); err != nil {
				log.Fatalf("提交任务失败: %v", err)
			}
		}
	}

	// 输出协议模式信息
	if cfg.Protocol.Type == "tuic" {
		log.Println("\n=== TUIC 协议模式 ===")
		log.Println("提示: TUIC 协议当前使用 HTTP 接口模式，不支持节点管理和心跳功能")
		log.Println("任务提交功能已测试完成")
	} else if cfg.Protocol.Type == "both" {
		log.Println("\n=== 双协议模式（节点池已初始化）===")
		log.Println("节点池功能: 自动发现和连接到多个服务器节点，支持负载均衡")
		log.Println("任务提交: 使用节点池进行负载均衡（通过 gRPC 协议）")
		log.Println("节点发现: 持续通过心跳发现新节点，自动添加到节点池")
	} else {
		// gRPC 协议：节点池已在上面初始化
		log.Println("\n=== gRPC 协议模式（节点池已初始化）===")
		log.Println("节点池功能: 自动发现和连接到多个服务器节点，支持负载均衡")
	}

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("客户端正在关闭...")
}

// testBasicConnection 测试基础连接
func testBasicConnection(ctx context.Context, client tasksmanager.TasksManagerClient) error {
	// 获取客户端列表
	resp, err := client.GetTaskClientInfoList(ctx, &tasksmanager.TaskClientInfoListRequest{})
	if err != nil {
		return fmt.Errorf("获取客户端列表失败: %w", err)
	}
	log.Printf("当前客户端数量: %d", len(resp.Items))

	// 获取节点列表
	nodeResp, err := client.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
	if err != nil {
		return fmt.Errorf("获取节点列表失败: %w", err)
	}
	log.Printf("当前节点数量: %d", len(nodeResp.Items))

	return nil
}

// testNodeManagement 测试节点管理（保持向后兼容）
func testNodeManagement(ctx context.Context, client tasksmanager.TasksManagerClient, clientName string) (string, error) {
	clientID, _, err := testNodeManagementWithResponse(ctx, client, clientName)
	return clientID, err
}

// testNodeManagementWithResponse 客户端注册并返回响应
func testNodeManagementWithResponse(ctx context.Context, client tasksmanager.TasksManagerClient, clientName string) (string, *tasksmanager.RegisterClientResponse, error) {
	// 获取真实的系统信息
	_, systemInfo, cpuInfo, memoryInfo, _ := getRealSystemInfo()

	// 创建客户端信息（不是服务器节点信息）
	clientInfo := &tasksmanager.TaskClientInfo{
		ClientUuid:           fmt.Sprintf("client-%s-%d", clientName, time.Now().Unix()),
		ClientName:           clientName,
		ClientIp:             "127.0.0.1",
		ClientSystem:         systemInfo,
		ClientVersion:        "1.0.0",
		ClientCpu:            cpuInfo,
		ClientMemory:         memoryInfo,
		ClientCreateTime:     time.Now().Format(time.RFC3339),
		ClientLastActiveTime: time.Now().Format(time.RFC3339),
		ClientTaskStatus:     tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE,
	}

	// 注册客户端（使用客户端专用接口）
	regResp, err := client.RegisterClient(ctx, clientInfo)
	if err != nil {
		return "", nil, fmt.Errorf("客户端注册失败: %w", err)
	}

	if regResp.Success {
		log.Printf("客户端注册成功: %s", clientInfo.ClientUuid)
		if len(regResp.ServerNodes) > 0 {
			log.Printf("📡 发现 %d 个服务器节点，将自动连接", len(regResp.ServerNodes))
		}
	}

	return clientInfo.ClientUuid, regResp, nil
}

// startHeartbeatWithNodePool 启动客户端心跳（包含节点池管理器）
func startHeartbeatWithNodePool(ctx context.Context, client tasksmanager.TasksManagerClient, clientName, clientID string, nodePool *NodePool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// 初始化系统信息
	_, systemInfo, cpuInfo, memoryInfo, _ := getRealSystemInfo()
	firstRun := true

	for range ticker.C {
		// 获取真实的系统信息
		cpuUsage, err := getCPUUsage()
		if err != nil {
			log.Printf("获取 CPU 使用率失败: %v", err)
			cpuUsage = 0
		}

		memoryUsed, memoryTotal, err := getMemoryUsage()
		if err != nil {
			log.Printf("获取内存使用情况失败: %v", err)
			memoryUsed = 0
			memoryTotal = 0
		}

		// 获取网络使用情况（首次调用需要等待1秒）
		networkRx, networkTx, err := getNetworkUsage()
		if err != nil {
			if firstRun {
				// 首次运行需要初始化，跳过本次
				firstRun = false
				continue
			}
			log.Printf("获取网络使用情况失败: %v", err)
			networkRx = 0
			networkTx = 0
		}
		firstRun = false

		// 获取磁盘使用情况
		diskUsed, diskTotal, err := getDiskUsage()
		if err != nil {
			log.Printf("获取磁盘使用情况失败: %v", err)
			diskUsed = 0
			diskTotal = 0
		}

		// 创建客户端信息（不是服务器节点信息）
		clientInfo := &tasksmanager.TaskClientInfo{
			ClientUuid:           clientID,
			ClientName:           clientName,
			ClientIp:             "127.0.0.1",
			ClientSystem:         systemInfo,
			ClientVersion:        "1.0.0",
			ClientCpu:            cpuInfo,
			ClientMemory:         memoryInfo,
			ClientCreateTime:     time.Now().Format(time.RFC3339),
			ClientLastActiveTime: time.Now().Format(time.RFC3339),
			ClientTaskStatus:     tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE,
			CpuUsagePercent:      &cpuUsage,
			MemoryUsedBytes:      &memoryUsed,
			MemoryTotalBytes:     &memoryTotal,
			NetworkRxBytesPerSec: &networkRx,
			NetworkTxBytesPerSec: &networkTx,
			DiskUsedBytes:        &diskUsed,
			DiskTotalBytes:       &diskTotal,
		}

		updateTime := time.Now().Format(time.RFC3339)
		clientInfo.ResourceUpdateTime = &updateTime

		// 使用客户端心跳接口（不是服务器节点心跳）
		resp, err := client.ClientHeartbeat(ctx, clientInfo)
		if err != nil {
			log.Printf("客户端心跳发送失败: %v", err)
			continue
		}

		if resp.Success {
			memPercent := float64(0)
			if memoryTotal > 0 {
				memPercent = float64(memoryUsed) / float64(memoryTotal) * 100
			}
			log.Printf("客户端心跳发送成功: %s (CPU: %.1f%%, Memory: %.1f%% [%.2fGB/%.2fGB], 网络: ↓%.2fMB/s ↑%.2fMB/s)",
				clientID, cpuUsage, memPercent,
				float64(memoryUsed)/1024/1024/1024, float64(memoryTotal)/1024/1024/1024,
				networkRx/1024/1024, networkTx/1024/1024)

			// 处理心跳响应中的新服务器节点信息
			if nodePool != nil {
				if len(resp.NewServerNodes) > 0 {
					oldCount := nodePool.GetNodeCount()
					log.Printf("📡 心跳响应中发现 %d 个新服务器节点，正在添加到节点池...", len(resp.NewServerNodes))
					for _, node := range resp.NewServerNodes {
						log.Printf("  - 新节点: %s (%s:%s)", node.NodeUuid, node.NodeIp, node.NodePort)
					}
					nodePool.AddNodes(resp.NewServerNodes)
					newCount := nodePool.GetNodeCount()
					log.Printf("✅ 节点池更新完成: 总数=%d (新增 %d 个), 健康=%d", newCount, newCount-oldCount, nodePool.GetHealthyNodeCount())
					log.Printf("🔄 新节点已加入负载均衡，后续任务将自动分担到所有节点")
				} else {
					// 即使没有新节点，也定期输出节点池状态（每10次心跳输出一次）
					// 这里简化处理，每次心跳都输出当前节点池状态（DEBUG级别）
					log.Printf("[DEBUG] 节点池当前状态: 总数=%d, 健康=%d", nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())
				}
			}
		}
	}
}

// startHeartbeat 启动心跳（旧版本，保持向后兼容）
func startHeartbeat(ctx context.Context, client tasksmanager.TasksManagerClient, clientName string) {
	clientID := fmt.Sprintf("client-%s-%d", clientName, time.Now().Unix())
	startHeartbeatWithNodePool(ctx, client, clientName, clientID, nil)
}

// submitRealTask 提交真实的数据请求任务
func submitRealTask(ctx context.Context, client tasksmanager.TasksManagerClient, clientID, taskTypeStr, tileKey string, epoch int32) error {
	// 解析任务类型
	var taskType tasksmanager.TaskType
	switch taskTypeStr {
	case "q2":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2
	case "imagery":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_IMAGERY
	case "terrain":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_TERRAIN
	default:
		return fmt.Errorf("不支持的任务类型: %s (支持: q2, imagery, terrain)", taskTypeStr)
	}

	taskMethod := tasksmanager.TaskMethod_TASK_METHOD_GET
	taskStatus := tasksmanager.TaskStatus_TASK_STATUS_PENDING

	// 使用反射创建请求（因为 proto 可能还未重新生成）
	req := &tasksmanager.TaskRequest{
		TaskClientId: clientID,
		TaskType:     taskType,
		TaskMethod:   &taskMethod,
		TaskStatus:   &taskStatus,
	}

	// 设置 TileKey 和 Epoch 字段（proto 文件已重新生成）
	req.TileKey = tileKey
	req.Epoch = epoch

	log.Printf("提交任务请求: task_type=%s, TileKey=%s, epoch=%d", taskTypeStr, tileKey, epoch)

	// 发送任务请求（单次请求，状态码非 200 视为失败）
	startTime := time.Now()
	resp, err := client.SubmitTask(ctx, req)
	elapsed := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("提交任务失败: %w", err)
	}

	// 打印结果
	log.Println()
	log.Println("=== 任务执行结果 ===")
	statusCode := getResponseStatusCode(resp)
	log.Printf("状态码: %d", statusCode)

	bodySize := getResponseBodySize(resp)
	log.Printf("响应体大小: %d 字节 (%.2f KB, %.2f MB)",
		bodySize,
		float64(bodySize)/1024,
		float64(bodySize)/1024/1024)
	log.Printf("请求耗时: %v", elapsed)

	// 获取响应中的 TileKey 和 Epoch
	log.Printf("响应 TileKey: %s", resp.TileKey)
	log.Printf("响应 Epoch: %d", resp.Epoch)

	if statusCode != 200 {
		return fmt.Errorf("任务返回非 200 状态码: %d", statusCode)
	}

	return nil
}

// getResponseStatusCode 获取响应状态码
func getResponseStatusCode(resp *tasksmanager.TaskResponse) int32 {
	if resp.TaskResponseStatusCode != nil {
		return *resp.TaskResponseStatusCode
	}
	return 0
}

// getResponseBodySize 获取响应体大小
func getResponseBodySize(resp *tasksmanager.TaskResponse) int {
	if resp.TaskResponseBody != nil {
		return len(resp.TaskResponseBody)
	}
	return 0
}

// （客户端侧重试逻辑已移除，是否重试由调用方或服务端控制）

// submitRealTaskMultipleTimes 重复提交同一个任务请求多次（用于性能测试）
func submitRealTaskMultipleTimes(ctx context.Context, client tasksmanager.TasksManagerClient, clientID, taskTypeStr, tileKey string, epoch int32, repeatCount, concurrency int) error {
	// 解析任务类型
	var taskType tasksmanager.TaskType
	switch taskTypeStr {
	case "q2":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2
	case "imagery":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_IMAGERY
	case "terrain":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_TERRAIN
	default:
		return fmt.Errorf("不支持的任务类型: %s (支持: q2, imagery, terrain)", taskTypeStr)
	}

	taskMethod := tasksmanager.TaskMethod_TASK_METHOD_GET
	taskStatus := tasksmanager.TaskStatus_TASK_STATUS_PENDING

	// 创建请求
	req := &tasksmanager.TaskRequest{
		TaskClientId: clientID,
		TaskType:     taskType,
		TaskMethod:   &taskMethod,
		TaskStatus:   &taskStatus,
		TileKey:      tileKey,
		Epoch:        epoch,
	}

	log.Printf("开始批量提交任务: task_type=%s, TileKey=%s, epoch=%d, 重复次数=%d", taskTypeStr, tileKey, epoch, repeatCount)
	log.Println()

	// 统计变量
	var (
		completedTasks   int64
		failedTasks      int64
		totalBytes       int64
		firstRequestTime time.Duration
		firstRequestOnce sync.Once // 确保只记录第一次请求时间
		requestTimes     []time.Duration
		requestTimesMu   sync.Mutex
	)

	// 记录总开始时间
	totalStartTime := time.Now()

	// 高并发发送请求
	// 优化：根据任务数量动态调整默认并发数
	if concurrency <= 0 {
		// 默认并发数：根据任务数量动态调整
		if repeatCount < 1000 {
			concurrency = 100
		} else if repeatCount < 10000 {
			concurrency = 500
		} else {
			concurrency = 1000
		}
	}
	if repeatCount < concurrency {
		concurrency = repeatCount
	}

	log.Printf("并发配置: %d 个工作 goroutine, 总任务数: %d", concurrency, repeatCount)

	// 创建任务通道和工作 goroutine
	taskChan := make(chan int, repeatCount)
	var wg sync.WaitGroup

	// 启动工作 goroutine
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for taskID := range taskChan {
				startTime := time.Now()
				resp, err := client.SubmitTask(ctx, req)
				elapsed := time.Since(startTime)

				if err != nil {
					atomic.AddInt64(&failedTasks, 1)
					log.Printf("❌ [Worker %d] 请求 #%d 失败: %v", workerID, taskID+1, err)
					continue
				}

				statusCode := getResponseStatusCode(resp)
				if statusCode != 200 {
					atomic.AddInt64(&failedTasks, 1)
					log.Printf("❌ [Worker %d] 请求 #%d 返回非 200 状态码: %d", workerID, taskID+1, statusCode)
					continue
				}

				atomic.AddInt64(&completedTasks, 1)

				// 记录第一次请求的时间（使用 sync.Once 确保线程安全）
				firstRequestOnce.Do(func() {
					firstRequestTime = elapsed
				})

				// 保存请求耗时
				requestTimesMu.Lock()
				requestTimes = append(requestTimes, elapsed)
				requestTimesMu.Unlock()

				// 统计响应体大小
				if resp.TaskResponseBody != nil {
					atomic.AddInt64(&totalBytes, int64(len(resp.TaskResponseBody)))
				}

				// 每次成功请求输出进度
				log.Printf("✅ [Worker %d] 请求 #%d: 状态码=%d, 耗时=%v, 响应大小=%d 字节",
					workerID, taskID+1, statusCode, elapsed, getResponseBodySize(resp))

				// 如果是第一次请求，输出详细信息
				if taskID == 0 {
					log.Printf("   首次请求: TileKey=%s, Epoch=%d", resp.TileKey, resp.Epoch)
				}
			}
		}(i)
	}

	// 发送所有任务
	for i := 0; i < repeatCount; i++ {
		taskChan <- i
	}
	close(taskChan)

	// 等待所有 goroutine 完成
	wg.Wait()

	totalElapsed := time.Since(totalStartTime)
	completed := atomic.LoadInt64(&completedTasks)
	failed := atomic.LoadInt64(&failedTasks)
	totalBytesCount := atomic.LoadInt64(&totalBytes)

	// 计算统计信息
	var (
		avgTime    time.Duration
		minTime    time.Duration
		maxTime    time.Duration
		medianTime time.Duration
	)

	if len(requestTimes) > 0 {
		// 计算平均、最小、最大耗时
		var sum time.Duration
		minTime = requestTimes[0]
		maxTime = requestTimes[0]

		for _, t := range requestTimes {
			sum += t
			if t < minTime {
				minTime = t
			}
			if t > maxTime {
				maxTime = t
			}
		}
		avgTime = sum / time.Duration(len(requestTimes))

		// 计算中位数
		sortedTimes := make([]time.Duration, len(requestTimes))
		copy(sortedTimes, requestTimes)
		// 简单排序（冒泡排序，数量不多时足够用）
		for i := 0; i < len(sortedTimes)-1; i++ {
			for j := i + 1; j < len(sortedTimes); j++ {
				if sortedTimes[i] > sortedTimes[j] {
					sortedTimes[i], sortedTimes[j] = sortedTimes[j], sortedTimes[i]
				}
			}
		}
		if len(sortedTimes) > 0 {
			medianTime = sortedTimes[len(sortedTimes)/2]
		}
	}

	// 输出统计结果
	log.Println()
	log.Println("=" + strings.Repeat("=", 60))
	log.Println("=== 批量请求性能统计 ===")
	log.Printf("总请求数: %d", repeatCount)
	log.Printf("成功: %d", completed)
	log.Printf("失败: %d", failed)
	log.Printf("总耗时: %v", totalElapsed)
	log.Printf("平均 QPS: %.2f 请求/秒", float64(completed)/totalElapsed.Seconds())
	log.Printf("总传输数据: %.2f KB (%.2f MB)", float64(totalBytesCount)/1024, float64(totalBytesCount)/1024/1024)
	log.Println()
	log.Println("--- 请求耗时统计 ---")
	if firstRequestTime > 0 {
		log.Printf("首次请求耗时: %v", firstRequestTime)
	}
	if avgTime > 0 {
		log.Printf("平均耗时: %v", avgTime)
	}
	if minTime > 0 {
		log.Printf("最快请求: %v", minTime)
	}
	if maxTime > 0 {
		log.Printf("最慢请求: %v", maxTime)
	}
	if medianTime > 0 {
		log.Printf("中位数耗时: %v", medianTime)
	}

	// 分析连接复用效果
	if len(requestTimes) >= 2 && firstRequestTime > 0 {
		// 计算第二次及后续请求的平均耗时
		subsequentTimes := requestTimes[1:]
		if len(subsequentTimes) > 0 {
			var subsequentSum time.Duration
			for _, t := range subsequentTimes {
				subsequentSum += t
			}
			avgSubsequentTime := subsequentSum / time.Duration(len(subsequentTimes))

			log.Println()
			log.Println("--- 连接复用效果分析 ---")
			log.Printf("首次请求耗时: %v", firstRequestTime)
			log.Printf("后续请求平均耗时: %v (共 %d 个)", avgSubsequentTime, len(subsequentTimes))
			if avgSubsequentTime < firstRequestTime {
				improvement := float64(firstRequestTime-avgSubsequentTime) / float64(firstRequestTime) * 100
				log.Printf("✅ 后续请求加速: %.1f%% (连接复用生效)", improvement)
			} else {
				log.Printf("⚠️  后续请求未加速，可能需要检查连接池配置")
			}
		}
	}

	log.Println("=" + strings.Repeat("=", 60))

	return nil
}

// submitRealTaskWithNodePool 使用节点池提交任务（负载均衡）
func submitRealTaskWithNodePool(ctx context.Context, nodePool *NodePool, protocolType, clientID, taskTypeStr, tileKey string, epoch int32) error {
	// 选择节点
	node, err := nodePool.SelectNode()
	if err != nil {
		return fmt.Errorf("选择节点失败: %w", err)
	}

	// 如果是 both 模式，优先使用 TUIC 协议
	if protocolType == "both" {
		// 尝试获取 TUIC 客户端
		tuicClient, err := nodePool.GetTUICClient(node.GrpcInfo.NodeUuid)
		if err == nil && tuicClient != nil {
			// 使用 TUIC 客户端提交任务
			log.Printf("使用节点 %s (%s:%s) 提交任务（TUIC 协议）", node.GrpcInfo.NodeUuid, node.GrpcInfo.NodeIp, node.GrpcInfo.NodePort)
			tuicAdapter := newTUICClientAdapter(tuicClient)
			return submitRealTask(ctx, tuicAdapter, clientID, taskTypeStr, tileKey, epoch)
		}
		// TUIC 不可用，回退到 gRPC
		log.Printf("节点 %s TUIC 不可用，使用 gRPC 协议", node.GrpcInfo.NodeUuid)
	}

	// 获取 gRPC 客户端
	grpcClient, err := nodePool.GetGRPCClient(node.GrpcInfo.NodeUuid)
	if err != nil {
		return fmt.Errorf("获取 gRPC 客户端失败: %w", err)
	}

	// 使用选中的节点提交任务
	log.Printf("使用节点 %s (%s:%s) 提交任务（gRPC 协议）", node.GrpcInfo.NodeUuid, node.GrpcInfo.NodeIp, node.GrpcInfo.NodePort)
	return submitRealTask(ctx, grpcClient, clientID, taskTypeStr, tileKey, epoch)
}

// submitRealTaskMultipleTimesWithNodePool 使用节点池批量提交任务（负载均衡）
func submitRealTaskMultipleTimesWithNodePool(ctx context.Context, nodePool *NodePool, protocolType, clientID, taskTypeStr, tileKey string, epoch int32, repeatCount, concurrency int) error {
	// 解析任务类型
	var taskType tasksmanager.TaskType
	switch taskTypeStr {
	case "q2":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2
	case "imagery":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_IMAGERY
	case "terrain":
		taskType = tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_TERRAIN
	default:
		return fmt.Errorf("不支持的任务类型: %s (支持: q2, imagery, terrain)", taskTypeStr)
	}

	taskMethod := tasksmanager.TaskMethod_TASK_METHOD_GET
	taskStatus := tasksmanager.TaskStatus_TASK_STATUS_PENDING

	// 创建请求
	req := &tasksmanager.TaskRequest{
		TaskClientId: clientID,
		TaskType:     taskType,
		TaskMethod:   &taskMethod,
		TaskStatus:   &taskStatus,
		TileKey:      tileKey,
		Epoch:        epoch,
	}

	log.Printf("开始批量提交任务（使用节点池负载均衡）: task_type=%s, TileKey=%s, epoch=%d, 重复次数=%d", taskTypeStr, tileKey, epoch, repeatCount)
	log.Printf("节点池状态: 总数=%d, 健康=%d", nodePool.GetNodeCount(), nodePool.GetHealthyNodeCount())
	log.Println()

	// 统计变量
	var (
		completedTasks   int64
		failedTasks      int64
		totalBytes       int64
		firstRequestTime time.Duration
		firstRequestOnce sync.Once
		// 优化：使用预分配的slice，避免频繁append和锁竞争
		// 只采样部分请求的耗时（每10个采样1个），减少内存和锁开销
		requestTimes   = make([]time.Duration, 0, repeatCount/10+100)
		requestTimesMu sync.Mutex
		// 优化：预先初始化所有节点的使用计数器，避免运行时加锁
		nodeUsage   = make(map[string]*int64) // 节点使用统计（使用原子操作）
		nodeUsageMu sync.Mutex
		// 全局节点选择计数器（所有worker共享，确保真正的轮询）
		globalNodeIndex int64
	)

	// 记录总开始时间
	totalStartTime := time.Now()

	// 高并发发送请求
	// 优化：根据任务数量和节点数量动态调整默认并发数，充分利用多节点资源
	if concurrency <= 0 {
		// 获取可用节点数量
		healthyNodeCount := nodePool.GetHealthyNodeCount()
		if healthyNodeCount == 0 {
			healthyNodeCount = 1 // 防止除零
		}

		// 默认并发数：根据任务数量和节点数量动态调整
		// 基础并发数：根据任务数量
		var baseConcurrency int
		if repeatCount < 1000 {
			// 小任务：每个节点至少100并发，充分利用多节点
			baseConcurrency = 200 // 提高小任务的并发数
		} else if repeatCount < 10000 {
			baseConcurrency = 500
		} else {
			baseConcurrency = 1000
		}

		// 根据节点数量调整：多节点时可以支持更高的并发
		// 每个节点可以处理更多并发请求（TUIC/QUIC支持多路复用）
		// 节点数越多，总并发数可以更高
		concurrency = baseConcurrency * healthyNodeCount

		// 设置上限：避免创建过多goroutine
		// 对于小任务，允许更高的并发上限（充分利用多节点）
		var maxConcurrency int
		if repeatCount < 1000 {
			maxConcurrency = 500 * healthyNodeCount // 小任务允许更高并发
		} else {
			maxConcurrency = 2000 * healthyNodeCount
		}
		if concurrency > maxConcurrency {
			concurrency = maxConcurrency
		}
	}
	if repeatCount < concurrency {
		concurrency = repeatCount
	}

	log.Printf("并发配置: %d 个工作 goroutine, 总任务数: %d", concurrency, repeatCount)

	// 预先获取所有节点的客户端引用，避免每次请求都加锁
	type nodeClient struct {
		nodeUUID   string
		tuicClient TUICClient
		grpcClient tasksmanager.TasksManagerClient
		hasTUIC    bool
		nodeAddr   string
		tuicAddr   string
	}
	nodeClients := make([]*nodeClient, 0)
	nodePool.nodesMu.RLock()
	for uuid, node := range nodePool.nodes {
		if !node.Healthy {
			continue
		}
		nc := &nodeClient{
			nodeUUID: uuid,
			nodeAddr: fmt.Sprintf("%s:%s", node.GrpcInfo.NodeIp, node.GrpcInfo.NodePort),
		}

		// 获取 gRPC 客户端
		nodePool.grpcClientsMu.RLock()
		if grpcClient, exists := nodePool.grpcClients[uuid]; exists {
			nc.grpcClient = grpcClient
		}
		nodePool.grpcClientsMu.RUnlock()

		// 获取 TUIC 客户端
		if protocolType == "both" {
			nodePool.tuicClientsMu.RLock()
			if tuicClient, exists := nodePool.tuicClients[uuid]; exists {
				nc.tuicClient = tuicClient
				nc.hasTUIC = true
				if node.TUICConfig != nil {
					nc.tuicAddr = fmt.Sprintf("%s:%s", node.GrpcInfo.NodeIp, node.TUICConfig.Port)
				}
			}
			nodePool.tuicClientsMu.RUnlock()
		}

		if nc.grpcClient != nil {
			nodeClients = append(nodeClients, nc)
		}
	}
	nodePool.nodesMu.RUnlock()

	if len(nodeClients) == 0 {
		return fmt.Errorf("没有可用的节点客户端")
	}

	// 预先初始化所有节点的使用计数器，避免运行时加锁
	nodeUsageMu.Lock()
	for _, nc := range nodeClients {
		if nodeUsage[nc.nodeUUID] == nil {
			var count int64
			nodeUsage[nc.nodeUUID] = &count
		}
	}
	nodeUsageMu.Unlock()

	log.Printf("✅ 已预加载 %d 个节点的客户端引用，减少锁竞争", len(nodeClients))

	// 创建任务通道和工作 goroutine
	// 使用缓冲通道，容量设为任务数的2倍，以便重试任务可以重新入队
	taskChan := make(chan int, repeatCount*2)
	var wg sync.WaitGroup
	// 优化：使用 sync.Map 减少锁竞争（并发安全的map）
	taskCompleted := &sync.Map{} // map[int]bool

	// 启动工作 goroutine
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for taskID := range taskChan {
				// 优化：使用 sync.Map 快速检查任务是否已完成（无锁读取）
				if _, completed := taskCompleted.Load(taskID); completed {
					continue // 任务已完成，跳过
				}

				// 优化：使用全局原子计数器进行真正的轮询（所有worker共享）
				nodeIndex := atomic.AddInt64(&globalNodeIndex, 1)
				selectedNode := nodeClients[int(nodeIndex-1)%len(nodeClients)]

				// 记录节点使用（使用原子操作，无需加锁，因为已经预先初始化）
				countPtr := nodeUsage[selectedNode.nodeUUID]
				atomic.AddInt64(countPtr, 1)

				var resp *tasksmanager.TaskResponse
				var err2 error
				var protocolUsed string
				startTime := time.Now()

				// 如果是 both 模式，优先使用 TUIC 协议
				if protocolType == "both" && selectedNode.hasTUIC && selectedNode.tuicClient != nil {
					// 直接使用预加载的 TUIC 客户端（无锁，复用已建立的连接）
					resp, err2 = selectedNode.tuicClient.SubmitTask(ctx, req)
					protocolUsed = "TUIC"

					// 如果 TUIC 失败，回退到 gRPC
					if err2 != nil {
						protocolUsed = "gRPC"
						resp, err2 = selectedNode.grpcClient.SubmitTask(ctx, req)
					}
				} else {
					// 使用 gRPC（无锁，使用预加载的客户端）
					protocolUsed = "gRPC"
					resp, err2 = selectedNode.grpcClient.SubmitTask(ctx, req)
				}

				elapsed := time.Since(startTime)

				if err2 != nil {
					// 网络错误：重新放入任务池，让其他节点重试
					// 先检查任务是否已完成（可能其他worker已经完成了）
					if _, alreadyCompleted := taskCompleted.Load(taskID); !alreadyCompleted {
						nodeAddr := selectedNode.nodeAddr
						if protocolUsed == "TUIC" && selectedNode.tuicAddr != "" {
							nodeAddr = selectedNode.tuicAddr
						}
						log.Printf("⚠️ [Worker %d] 请求 #%d 失败，重新放入任务池 (节点: %s, 协议: %s): %v", workerID, taskID+1, nodeAddr, protocolUsed, err2)

						// 重新放入任务池，让其他节点重试
						select {
						case taskChan <- taskID:
							// 成功重新入队
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				statusCode := getResponseStatusCode(resp)
				if statusCode != 200 {
					// 非200状态码：重新放入任务池，让其他节点重试
					// 先检查任务是否已完成（可能其他worker已经完成了）
					if _, alreadyCompleted := taskCompleted.Load(taskID); !alreadyCompleted {
						nodeAddr := selectedNode.nodeAddr
						if protocolUsed == "TUIC" && selectedNode.tuicAddr != "" {
							nodeAddr = selectedNode.tuicAddr
						}
						log.Printf("⚠️ [Worker %d] 请求 #%d 返回非 200 状态码，重新放入任务池 (节点: %s, 协议: %s): %d", workerID, taskID+1, nodeAddr, protocolUsed, statusCode)

						// 重新放入任务池，让其他节点重试
						select {
						case taskChan <- taskID:
							// 成功重新入队
						case <-ctx.Done():
							return
						}
					}
					continue
				}

				// 成功完成：状态码200
				// 使用 sync.Map 的 LoadOrStore 原子操作，避免重复计数
				if _, loaded := taskCompleted.LoadOrStore(taskID, true); !loaded {
					// 第一次标记为完成，增加计数
					atomic.AddInt64(&completedTasks, 1)
				}

				// 记录第一次请求的时间
				firstRequestOnce.Do(func() {
					firstRequestTime = elapsed
				})

				// 优化：只采样部分请求的耗时（每10个采样1个），减少内存和锁开销
				if (taskID+1)%10 == 0 {
					requestTimesMu.Lock()
					requestTimes = append(requestTimes, elapsed)
					requestTimesMu.Unlock()
				}

				// 统计响应体大小
				if resp.TaskResponseBody != nil {
					atomic.AddInt64(&totalBytes, int64(len(resp.TaskResponseBody)))
				}

				// 减少日志输出频率：每100个请求输出一次进度，避免日志I/O阻塞
				if (taskID+1)%100 == 0 || taskID == 0 {
					nodeAddr := selectedNode.nodeAddr
					if protocolUsed == "TUIC" && selectedNode.tuicAddr != "" {
						nodeAddr = selectedNode.tuicAddr
					}
					log.Printf("✅ [Worker %d] 请求 #%d (节点: %s, 协议: %s): 状态码=%d, 耗时=%v, 响应大小=%d 字节",
						workerID, taskID+1, nodeAddr, protocolUsed, statusCode, elapsed, getResponseBodySize(resp))
				}
			}
		}(i)
	}

	// 发送所有初始任务
	for i := 0; i < repeatCount; i++ {
		taskChan <- i
	}

	// 等待所有任务完成（包括重试的任务）
	// 使用一个单独的goroutine来监控任务完成情况
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // 减少检查间隔，更快响应
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				completed := atomic.LoadInt64(&completedTasks)
				if completed >= int64(repeatCount) {
					// 所有任务都已完成，关闭通道让worker退出
					close(taskChan)
					close(done)
					return
				}
			case <-ctx.Done():
				close(taskChan)
				close(done)
				return
			}
		}
	}()

	// 等待所有 goroutine 完成
	wg.Wait()
	<-done // 确保监控goroutine也完成

	totalElapsed := time.Since(totalStartTime)
	completed := atomic.LoadInt64(&completedTasks)
	failed := atomic.LoadInt64(&failedTasks)
	totalBytesCount := atomic.LoadInt64(&totalBytes)

	// 计算统计信息（与 submitRealTaskMultipleTimes 相同）
	var (
		avgTime    time.Duration
		minTime    time.Duration
		maxTime    time.Duration
		medianTime time.Duration
	)

	if len(requestTimes) > 0 {
		var sum time.Duration
		minTime = requestTimes[0]
		maxTime = requestTimes[0]

		for _, t := range requestTimes {
			sum += t
			if t < minTime {
				minTime = t
			}
			if t > maxTime {
				maxTime = t
			}
		}
		avgTime = sum / time.Duration(len(requestTimes))

		// 优化：使用标准库的快速排序（O(n log n)），而不是O(n²)的冒泡排序
		sortedTimes := make([]time.Duration, len(requestTimes))
		copy(sortedTimes, requestTimes)
		if len(sortedTimes) > 0 {
			sort.Slice(sortedTimes, func(i, j int) bool {
				return sortedTimes[i] < sortedTimes[j]
			})
			medianTime = sortedTimes[len(sortedTimes)/2]
		}
	}

	// 输出统计结果
	log.Println()
	log.Println("=" + strings.Repeat("=", 60))
	log.Println("=== 批量请求性能统计（节点池负载均衡）===")
	log.Printf("总请求数: %d", repeatCount)
	log.Printf("成功: %d", completed)
	log.Printf("失败: %d", failed)
	log.Printf("总耗时: %v", totalElapsed)
	log.Printf("平均 QPS: %.2f 请求/秒", float64(completed)/totalElapsed.Seconds())
	log.Printf("总传输数据: %.2f KB (%.2f MB)", float64(totalBytesCount)/1024, float64(totalBytesCount)/1024/1024)
	log.Println()
	log.Println("--- 请求耗时统计 ---")
	if firstRequestTime > 0 {
		log.Printf("首次请求耗时: %v", firstRequestTime)
	}
	if avgTime > 0 {
		log.Printf("平均耗时: %v", avgTime)
	}
	if minTime > 0 {
		log.Printf("最快请求: %v", minTime)
	}
	if maxTime > 0 {
		log.Printf("最慢请求: %v", maxTime)
	}
	if medianTime > 0 {
		log.Printf("中位数耗时: %v", medianTime)
	}

	// 输出节点使用统计
	log.Println()
	log.Println("--- 节点使用统计（负载均衡效果）---")
	nodeUsageMu.Lock()
	for _, nc := range nodeClients {
		if countPtr := nodeUsage[nc.nodeUUID]; countPtr != nil {
			count := atomic.LoadInt64(countPtr)
			if completed > 0 {
				percentage := float64(count) / float64(completed) * 100
				log.Printf("节点 %s: %d 次请求 (%.1f%%)", nc.nodeUUID, count, percentage)
			}
		}
	}
	nodeUsageMu.Unlock()

	log.Println("=" + strings.Repeat("=", 60))

	return nil
}
