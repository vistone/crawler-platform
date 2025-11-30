package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"crawler-platform/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger.InitGlobalLogger(logger.NewConsoleLogger(true, true, true, true))

	// 连接到服务器
	conn, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	defer conn.Close()

	client := tasksmanager.NewTasksManagerClient(conn)
	ctx := context.Background()

	fmt.Println("=== gRPC 功能测试 ===\n")

	// 测试1: 基础连接
	fmt.Println("1. 测试基础连接")
	testBasicConnection(ctx, client)

	// 测试2: 节点注册
	fmt.Println("\n2. 测试节点注册")
	testNodeRegistration(ctx, client)

	// 测试3: 节点心跳
	fmt.Println("\n3. 测试节点心跳")
	testNodeHeartbeat(ctx, client)

	// 测试4: 节点列表同步
	fmt.Println("\n4. 测试节点列表同步")
	testSyncNodeList(ctx, client)

	// 测试5: 节点消息传递
	fmt.Println("\n5. 测试节点消息传递")
	testSendMessage(ctx, client)

	// 测试6: 任务提交
	fmt.Println("\n6. 测试任务提交")
	testSubmitTask(ctx, client)

	// 测试7: 实时监控
	fmt.Println("\n7. 测试实时监控")
	testRealTimeMonitoring(ctx, client)

	fmt.Println("\n=== 所有测试完成 ===")
}

func testBasicConnection(ctx context.Context, client tasksmanager.TasksManagerClient) {
	// 获取客户端列表
	resp, err := client.GetTaskClientInfoList(ctx, &tasksmanager.TaskClientInfoListRequest{})
	if err != nil {
		log.Printf("❌ 获取客户端列表失败: %v", err)
		return
	}
	fmt.Printf("✅ 获取客户端列表成功，数量: %d\n", len(resp.Items))

	// 获取节点列表
	nodeResp, err := client.GetGrpcServerNodeInfoList(ctx, &tasksmanager.GrpcServerNodeInfoListRequest{})
	if err != nil {
		log.Printf("❌ 获取节点列表失败: %v", err)
		return
	}
	fmt.Printf("✅ 获取节点列表成功，数量: %d\n", len(nodeResp.Items))
}

func testNodeRegistration(ctx context.Context, client tasksmanager.TasksManagerClient) {
	nodeInfo := &tasksmanager.GrpcServerNodeInfo{
		NodeUuid:           fmt.Sprintf("test-node-%d", time.Now().Unix()),
		NodeName:           "测试节点",
		NodeIp:             "127.0.0.1",
		NodePort:           "50052",
		NodeSystem:         "Linux",
		NodeVersion:        "1.0.0",
		NodeCpu:            "Intel Core i7",
		NodeMemory:         "16GB",
		NodeCreateTime:     time.Now().Format(time.RFC3339),
		NodeLastActiveTime: time.Now().Format(time.RFC3339),
	}

	req := &tasksmanager.NodeRegistrationRequest{
		NodeInfo:   nodeInfo,
		KnownNodes: []string{},
	}

	resp, err := client.RegisterNode(ctx, req)
	if err != nil {
		log.Printf("❌ 节点注册失败: %v", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ 节点注册成功: %s\n", nodeInfo.NodeUuid)
		fmt.Printf("   已知节点数量: %d\n", len(resp.KnownNodes))
	} else {
		fmt.Printf("❌ 节点注册失败: %s\n", resp.Message)
	}
}

func testNodeHeartbeat(ctx context.Context, client tasksmanager.TasksManagerClient) {
	nodeID := fmt.Sprintf("test-node-%d", time.Now().Unix())
	cpuUsage := 45.5
	memoryUsed := int64(8 * 1024 * 1024 * 1024)
	memoryTotal := int64(16 * 1024 * 1024 * 1024)
	networkRx := 1200000.0
	networkTx := 600000.0

	nodeInfo := &tasksmanager.GrpcServerNodeInfo{
		NodeUuid:            nodeID,
		NodeName:            "测试节点",
		NodeIp:              "127.0.0.1",
		NodePort:            "50052",
		NodeSystem:          "Linux",
		NodeVersion:         "1.0.0",
		NodeCpu:             "Intel Core i7",
		NodeMemory:          "16GB",
		NodeCreateTime:      time.Now().Format(time.RFC3339),
		NodeLastActiveTime:  time.Now().Format(time.RFC3339),
		CpuUsagePercent:     &cpuUsage,
		MemoryUsedBytes:     &memoryUsed,
		MemoryTotalBytes:    &memoryTotal,
		NetworkRxBytesPerSec: &networkRx,
		NetworkTxBytesPerSec: &networkTx,
	}

	updateTime := time.Now().Format(time.RFC3339)
	nodeInfo.ResourceUpdateTime = &updateTime

	req := &tasksmanager.NodeHeartbeatRequest{
		NodeUuid:  nodeID,
		NodeInfo:  nodeInfo,
		Timestamp: time.Now().UnixMilli(),
	}

	resp, err := client.NodeHeartbeat(ctx, req)
	if err != nil {
		log.Printf("❌ 心跳发送失败: %v", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ 心跳发送成功: %s\n", nodeID)
		fmt.Printf("   CPU 使用率: %.1f%%\n", cpuUsage)
		fmt.Printf("   内存使用: %.1f%%\n", float64(memoryUsed)/float64(memoryTotal)*100)
		fmt.Printf("   网络下行: %.2f MB/s\n", networkRx/1024/1024)
		fmt.Printf("   网络上行: %.2f MB/s\n", networkTx/1024/1024)
	}
}

func testSyncNodeList(ctx context.Context, client tasksmanager.TasksManagerClient) {
	req := &tasksmanager.SyncNodeListRequest{
		RequestingNodeUuid: fmt.Sprintf("test-sync-%d", time.Now().Unix()),
		KnownNodeUuids:     []string{},
		LastSyncTime:       time.Now().UnixMilli(),
	}

	resp, err := client.SyncNodeList(ctx, req)
	if err != nil {
		log.Printf("❌ 节点列表同步失败: %v", err)
		return
	}

	fmt.Printf("✅ 节点列表同步成功\n")
	fmt.Printf("   新增节点: %d\n", len(resp.NodesToAdd))
	fmt.Printf("   更新节点: %d\n", len(resp.NodesToUpdate))
	fmt.Printf("   移除节点: %d\n", len(resp.NodesToRemove))
}

func testSendMessage(ctx context.Context, client tasksmanager.TasksManagerClient) {
	msg := &tasksmanager.NodeMessage{
		MessageId:    fmt.Sprintf("msg-%d", time.Now().Unix()),
		FromNodeUuid: "test-from-node",
		ToNodeUuid:   "", // 广播消息
		MessageType:  "TEST_MESSAGE",
		Payload:      []byte("这是一条测试消息"),
		Timestamp:    time.Now().UnixMilli(),
	}

	ttl := int32(10)
	msg.Ttl = &ttl

	req := &tasksmanager.NodeMessageRequest{
		Message: msg,
	}

	resp, err := client.SendNodeMessage(ctx, req)
	if err != nil {
		log.Printf("❌ 发送消息失败: %v", err)
		return
	}

	if resp.Success {
		fmt.Printf("✅ 消息发送成功: %s\n", msg.MessageId)
		fmt.Printf("   消息类型: %s\n", msg.MessageType)
		fmt.Printf("   消息内容: %s\n", string(msg.Payload))
	}
}

func testSubmitTask(ctx context.Context, client tasksmanager.TasksManagerClient) {
	taskReq := &tasksmanager.TaskRequest{
		TaskClientId: "test-client-1",
		TaskType:     tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2,
		TaskUri:      "https://example.com/api/data",
		TaskMethod:   &[]tasksmanager.TaskMethod{tasksmanager.TaskMethod_TASK_METHOD_GET}[0],
		TaskStatus:   &[]tasksmanager.TaskStatus{tasksmanager.TaskStatus_TASK_STATUS_PENDING}[0],
	}

	resp, err := client.SubmitTask(ctx, taskReq)
	if err != nil {
		log.Printf("❌ 任务提交失败: %v", err)
		return
	}

	fmt.Printf("✅ 任务提交成功\n")
	fmt.Printf("   任务类型: %s\n", resp.TaskType)
	fmt.Printf("   任务 URI: %s\n", resp.TaskUri)
}

func testRealTimeMonitoring(ctx context.Context, client tasksmanager.TasksManagerClient) {
	// 发送几次心跳以模拟实时监控
	for i := 0; i < 3; i++ {
		nodeID := "monitoring-node"
		cpuUsage := 30.0 + float64(i)*10
		memoryUsed := int64((8 + i) * 1024 * 1024 * 1024)
		memoryTotal := int64(16 * 1024 * 1024 * 1024)

		nodeInfo := &tasksmanager.GrpcServerNodeInfo{
			NodeUuid:           nodeID,
			NodeName:           "监控节点",
			NodeIp:             "127.0.0.1",
			NodePort:           "50053",
			NodeSystem:         "Linux",
			NodeVersion:        "1.0.0",
			NodeCpu:            "Intel Core i7",
			NodeMemory:         "16GB",
			NodeCreateTime:     time.Now().Format(time.RFC3339),
			NodeLastActiveTime: time.Now().Format(time.RFC3339),
			CpuUsagePercent:    &cpuUsage,
			MemoryUsedBytes:    &memoryUsed,
			MemoryTotalBytes:   &memoryTotal,
		}

		updateTime := time.Now().Format(time.RFC3339)
		nodeInfo.ResourceUpdateTime = &updateTime

		req := &tasksmanager.NodeHeartbeatRequest{
			NodeUuid:  nodeID,
			NodeInfo:  nodeInfo,
			Timestamp: time.Now().UnixMilli(),
		}

		_, err := client.NodeHeartbeat(ctx, req)
		if err != nil {
			log.Printf("❌ 监控数据上报失败: %v", err)
			continue
		}

		fmt.Printf("✅ 监控数据上报 #%d: CPU=%.1f%%, Memory=%.1f%%\n", 
			i+1, cpuUsage, float64(memoryUsed)/float64(memoryTotal)*100)
		time.Sleep(1 * time.Second)
	}
}

