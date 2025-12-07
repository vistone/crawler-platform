package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	switch cfg.Protocol.Type {
	case "both":
		// 双协议模式：优先使用 TUIC，失败时自动切换到 gRPC
		var err error
		dualClient, err = NewDualProtocolClient(cfg)
		if err != nil {
			log.Fatalf("创建双协议客户端失败: %v", err)
		}
		defer dualClient.Close()
		client = dualClient
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
	if cfg.Task.RepeatCount > 1 {
		if err := submitRealTaskMultipleTimes(ctx, client, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch, cfg.Task.RepeatCount, cfg.Task.Concurrency); err != nil {
			log.Fatalf("批量提交任务失败: %v", err)
		}
	} else {
		if err := submitRealTask(ctx, client, cfg.Client.Name, cfg.Task.TaskType, cfg.Task.TileKey, cfg.Task.Epoch); err != nil {
			log.Fatalf("提交任务失败: %v", err)
		}
	}

	// 如果是 TUIC 协议或双协议模式，跳过 gRPC 特有的功能（节点管理、心跳等）
	// 注意：双协议模式下，如果切换到 gRPC，这些功能仍然不可用（因为使用的是适配器）
	if cfg.Protocol.Type == "tuic" || cfg.Protocol.Type == "both" {
		log.Println("\n=== TUIC 协议模式 ===")
		if cfg.Protocol.Type == "both" {
			log.Println("双协议模式: 优先使用 TUIC 协议，失败时自动切换到 gRPC 协议")
		} else {
			log.Println("提示: TUIC 协议当前使用 HTTP 接口模式，不支持节点管理和心跳功能")
		}
		log.Println("任务提交功能已测试完成")
	} else {
		// gRPC 协议：执行完整的客户端注册和节点管理流程
		// 测试客户端注册
		log.Println("\n=== 客户端注册 ===")
		clientID, regResp, err := testNodeManagementWithResponse(ctx, client, cfg.Client.Name)
		if err != nil {
			log.Printf("客户端注册失败: %v", err)
			return
		}

		// 创建节点管理器（用于管理到服务器节点的连接）
		// 传递 TLS 配置以便连接到其他节点时使用
		var nodeManagerTLSConfig *tls.Config
		if cfg.Server.CertsDir != "" {
			if config, err := LoadTLSConfigFromCertsDir(cfg.Server.CertsDir); err == nil {
				nodeManagerTLSConfig = config
			}
		}
		// 注意：nodeManager 需要 conn，但 TUIC 模式下 conn 为 nil，所以这里只在 gRPC 模式下执行
		nodeManager := NewNodeManagerWithTLS(client, conn, clientID, nodeManagerTLSConfig)
		defer nodeManager.Close()

		// 处理注册响应，自动连接到所有服务器节点
		if regResp != nil && regResp.Success && len(regResp.ServerNodes) > 0 {
			log.Printf("📡 发现 %d 个服务器节点，开始自动连接", len(regResp.ServerNodes))
			nodeManager.OnNodesDiscovered(regResp.ServerNodes)
		}

		// 启动自动发现
		log.Println("\n=== 启动自动节点发现 ===")
		discoveryCtx, cancelDiscovery := context.WithCancel(context.Background())
		defer cancelDiscovery()
		go nodeManager.StartAutoDiscovery(discoveryCtx)

		// 启动连接池健康检查
		log.Println("\n=== 启动连接池健康检查 ===")
		healthCtx, cancelHealth := context.WithCancel(context.Background())
		defer cancelHealth()
		go nodeManager.StartConnectionHealthCheck(healthCtx)

		// 启动客户端心跳（包含自动连接新服务器节点功能）
		log.Println("\n=== 启动客户端心跳（自动发现新服务器节点）===")
		go startHeartbeatWithNodeManager(ctx, client, cfg.Client.Name, clientID, nodeManager)
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

// startHeartbeatWithNodeManager 启动客户端心跳（包含节点管理器）
func startHeartbeatWithNodeManager(ctx context.Context, client tasksmanager.TasksManagerClient, clientName, clientID string, nodeManager *NodeManager) {
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
			if nodeManager != nil && len(resp.NewServerNodes) > 0 {
				log.Printf("📡 心跳响应中发现 %d 个新服务器节点，正在自动连接...", len(resp.NewServerNodes))
				nodeManager.OnNodesDiscovered(resp.NewServerNodes)
			}
		}
	}
}

// startHeartbeat 启动心跳（旧版本，保持向后兼容）
func startHeartbeat(ctx context.Context, client tasksmanager.TasksManagerClient, clientName string) {
	clientID := fmt.Sprintf("client-%s-%d", clientName, time.Now().Unix())
	startHeartbeatWithNodeManager(ctx, client, clientName, clientID, nil)
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
	if concurrency <= 0 {
		concurrency = 100 // 默认并发数量
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
