package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 解析命令行参数
	serverAddr := flag.String("server", "localhost:50051", "gRPC 服务器地址")
	clientName := flag.String("name", "client-1", "客户端名称")
	flag.Parse()

	// 初始化日志记录器
	logger.InitGlobalLogger(logger.NewConsoleLogger(true, true, true, true))

	// 连接到服务器
	conn, err := grpc.NewClient(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	defer conn.Close()

	// 创建客户端
	client := tasksmanager.NewTasksManagerClient(conn)
	ctx := context.Background()

	// 测试基础连接
	log.Println("=== 测试基础连接 ===")
	if err := testBasicConnection(ctx, client); err != nil {
		log.Printf("基础连接测试失败: %v", err)
	}

	// 测试客户端注册
	log.Println("\n=== 客户端注册 ===")
	clientID, regResp, err := testNodeManagementWithResponse(ctx, client, *clientName)
	if err != nil {
		log.Printf("客户端注册失败: %v", err)
		return
	}
	
	// 创建节点管理器（用于管理到服务器节点的连接）
	nodeManager := NewNodeManager(client, conn, clientID)
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
	go startHeartbeatWithNodeManager(ctx, client, *clientName, clientID, nodeManager)

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
		ClientUuid:          fmt.Sprintf("client-%s-%d", clientName, time.Now().Unix()),
		ClientName:          clientName,
		ClientIp:            "127.0.0.1",
		ClientSystem:        systemInfo,
		ClientVersion:       "1.0.0",
		ClientCpu:           cpuInfo,
		ClientMemory:        memoryInfo,
		ClientCreateTime:    time.Now().Format(time.RFC3339),
		ClientLastActiveTime: time.Now().Format(time.RFC3339),
		ClientTaskStatus:    tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE,
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
			ClientUuid:          clientID,
			ClientName:          clientName,
			ClientIp:            "127.0.0.1",
			ClientSystem:        systemInfo,
			ClientVersion:       "1.0.0",
			ClientCpu:           cpuInfo,
			ClientMemory:        memoryInfo,
			ClientCreateTime:    time.Now().Format(time.RFC3339),
			ClientLastActiveTime: time.Now().Format(time.RFC3339),
			ClientTaskStatus:    tasksmanager.ClientTaskStatus_CLIENT_TASK_STATUS_ONLINE,
			CpuUsagePercent:     &cpuUsage,
			MemoryUsedBytes:     &memoryUsed,
			MemoryTotalBytes:    &memoryTotal,
			NetworkRxBytesPerSec: &networkRx,
			NetworkTxBytesPerSec: &networkTx,
			DiskUsedBytes:       &diskUsed,
			DiskTotalBytes:      &diskTotal,
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
