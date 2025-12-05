package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"crawler-platform/cmd/grpcserver/tasksmanager"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// 性能测试统计
type PerformanceStats struct {
	totalRequests   int64
	successRequests int64
	failedRequests  int64
	totalLatency    int64 // 微秒
	minLatency      int64
	maxLatency      int64
	startTime       time.Time
	endTime         time.Time
	mu              sync.Mutex
	latencies       []int64 // 用于计算百分位数
	totalBytes      int64   // 总响应字节数
}

func NewPerformanceStats() *PerformanceStats {
	return &PerformanceStats{
		minLatency: 1<<63 - 1,
		latencies:  make([]int64, 0, 10000),
	}
}

func (s *PerformanceStats) RecordRequest(latency time.Duration, success bool, responseBytes int64) {
	latencyUs := latency.Microseconds()

	atomic.AddInt64(&s.totalRequests, 1)
	if success {
		atomic.AddInt64(&s.successRequests, 1)
		atomic.AddInt64(&s.totalBytes, responseBytes)
	} else {
		atomic.AddInt64(&s.failedRequests, 1)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	atomic.AddInt64(&s.totalLatency, latencyUs)
	s.latencies = append(s.latencies, latencyUs)

	if latencyUs < s.minLatency {
		s.minLatency = latencyUs
	}
	if latencyUs > s.maxLatency {
		s.maxLatency = latencyUs
	}
}

func (s *PerformanceStats) Print() {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := atomic.LoadInt64(&s.totalRequests)
	success := atomic.LoadInt64(&s.successRequests)
	failed := atomic.LoadInt64(&s.failedRequests)
	totalLatency := atomic.LoadInt64(&s.totalLatency)
	totalBytes := atomic.LoadInt64(&s.totalBytes)

	if total == 0 {
		fmt.Println("没有请求记录")
		return
	}

	duration := s.endTime.Sub(s.startTime).Seconds()
	avgLatency := float64(totalLatency) / float64(total) / 1000.0 // 转换为毫秒
	qps := float64(total) / duration
	throughput := float64(totalBytes) / duration / 1024 / 1024 // MB/s

	fmt.Println("\n========== 性能测试结果 ==========")
	fmt.Printf("总请求数: %d\n", total)
	fmt.Printf("成功请求: %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)
	fmt.Printf("测试时长: %.2f 秒\n", duration)
	fmt.Printf("QPS: %.2f\n", qps)
	fmt.Printf("总响应数据: %.2f MB\n", float64(totalBytes)/1024/1024)
	fmt.Printf("吞吐量: %.2f MB/s\n", throughput)
	fmt.Printf("\n延迟统计:\n")
	fmt.Printf("  平均延迟: %.2f ms\n", avgLatency)
	fmt.Printf("  最小延迟: %.2f ms\n", float64(s.minLatency)/1000.0)
	fmt.Printf("  最大延迟: %.2f ms\n", float64(s.maxLatency)/1000.0)

	// 计算百分位数
	if len(s.latencies) > 0 {
		// 简单排序（用于计算百分位数）
		sorted := make([]int64, len(s.latencies))
		copy(sorted, s.latencies)
		// 使用快速排序
		quickSort(sorted, 0, len(sorted)-1)

		p50 := sorted[len(sorted)*50/100]
		p95 := sorted[len(sorted)*95/100]
		p99 := sorted[len(sorted)*99/100]

		fmt.Printf("  P50: %.2f ms\n", float64(p50)/1000.0)
		fmt.Printf("  P95: %.2f ms\n", float64(p95)/1000.0)
		fmt.Printf("  P99: %.2f ms\n", float64(p99)/1000.0)
	}
	fmt.Println("==================================\n")
}

func quickSort(arr []int64, low, high int) {
	if low < high {
		pi := partition(arr, low, high)
		quickSort(arr, low, pi-1)
		quickSort(arr, pi+1, high)
	}
}

func partition(arr []int64, low, high int) int {
	pivot := arr[high]
	i := low - 1

	for j := low; j < high; j++ {
		if arr[j] < pivot {
			i++
			arr[i], arr[j] = arr[j], arr[i]
		}
	}
	arr[i+1], arr[high] = arr[high], arr[i+1]
	return i + 1
}

func main() {
	// 配置参数
	serverAddr := "localhost:50051"
	concurrency := 50     // 并发数
	totalRequests := 1000 // 总请求数
	requestTimeout := 30 * time.Second

	fmt.Printf("性能测试配置:\n")
	fmt.Printf("  服务器地址: %s\n", serverAddr)
	fmt.Printf("  并发数: %d\n", concurrency)
	fmt.Printf("  总请求数: %d\n", totalRequests)
	fmt.Printf("  请求超时: %v\n\n", requestTimeout)

	// 连接服务器
	// 尝试使用 TLS 证书连接（如果证书存在）
	var conn *grpc.ClientConn
	var err error

	certFile := "./certs/cert.pem"
	if _, statErr := os.Stat(certFile); statErr == nil {
		// 证书文件存在，使用 TLS 连接
		certPEM, err := os.ReadFile(certFile)
		if err != nil {
			log.Fatalf("读取证书文件失败: %v", err)
		}

		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(certPEM) {
			log.Fatalf("无法解析证书文件")
		}

		tlsConfig := &tls.Config{
			RootCAs: certPool,
		}

		creds := credentials.NewTLS(tlsConfig)
		conn, err = grpc.NewClient(serverAddr, grpc.WithTransportCredentials(creds))
		if err != nil {
			log.Fatalf("TLS 连接服务器失败: %v", err)
		}
		fmt.Println("✅ 已连接到服务器（使用 TLS）")
	} else {
		// 证书文件不存在，尝试不安全连接
		conn, err = grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("连接服务器失败: %v", err)
		}
		fmt.Println("✅ 已连接到服务器（使用不安全连接）")
	}
	defer conn.Close()

	client := tasksmanager.NewTasksManagerClient(conn)

	stats := NewPerformanceStats()
	stats.startTime = time.Now()

	// 创建请求通道
	requestChan := make(chan int, totalRequests)
	for i := 0; i < totalRequests; i++ {
		requestChan <- i
	}
	close(requestChan)

	// 启动并发请求
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for reqID := range requestChan {
				start := time.Now()
				success, bytes := sendTestRequest(client, requestTimeout, reqID, workerID)
				latency := time.Since(start)
				stats.RecordRequest(latency, success, bytes)
			}
		}(i)
	}

	wg.Wait()
	stats.endTime = time.Now()
	stats.Print()
}

// sendTestRequest 发送测试请求
func sendTestRequest(client tasksmanager.TasksManagerClient, timeout time.Duration, reqID, workerID int) (bool, int64) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 创建测试任务请求
	// 使用 Q2 任务类型进行测试（因为 GoogleEarthDesktopDataConfig 已启用）
	taskType := tasksmanager.TaskType_TASK_TYPE_GOOGLE_EARTH_Q2
	tileKey := fmt.Sprintf("test-tile-%d-%d", workerID, reqID)
	epoch := int32(1234567890)

	req := &tasksmanager.TaskRequest{
		TaskClientId: fmt.Sprintf("test-client-%d", workerID),
		TaskType:     taskType,
		TileKey:      tileKey,
		Epoch:        epoch,
		TaskBody:     []byte(fmt.Sprintf("test-body-%d", reqID)),
	}

	resp, err := client.SubmitTask(ctx, req)
	if err != nil {
		// 记录错误详情用于调试
		if reqID%100 == 0 { // 每100个请求记录一次错误
			fmt.Printf("请求失败 (reqID=%d, workerID=%d): %v\n", reqID, workerID, err)
		}
		return false, 0
	}

	// 检查响应状态码
	// 200 表示成功
	// 503 表示服务暂时不可用（连接问题），应该重试，但这里统计为失败
	// 500 表示服务器内部错误（配置错误等），应该统计为失败
	if resp.TaskResponseStatusCode != nil {
		statusCode := *resp.TaskResponseStatusCode
		if statusCode == 200 {
			return true, int64(len(resp.TaskResponseBody))
		}
		// 记录非200状态码（包括503和500）
		if reqID%100 == 0 {
			fmt.Printf("请求返回非200状态码 (reqID=%d, workerID=%d): %d\n", reqID, workerID, statusCode)
		}
	}

	return false, 0
}
