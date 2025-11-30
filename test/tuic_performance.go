package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// PerformanceMetrics 性能指标
type PerformanceMetrics struct {
	TotalRequests      int64
	SuccessRequests    int64
	FailedRequests     int64
	TotalBytes         int64
	TotalDuration      time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	AvgLatency         time.Duration
	ConnectionTime     time.Duration
	FirstByteTime      time.Duration
	Throughput         float64 // MB/s
	RequestsPerSecond  float64
}

// TestResult 测试结果
type TestResult struct {
	TestName      string
	Metrics       PerformanceMetrics
	Concurrency   int
	RequestCount  int
	DataSize      int
	ErrorMessages []string
}

// generateTestCert 生成测试证书
func generateTestCert() (certFile, keyFile string, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test"},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:     []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", "", err
	}

	certFile = filepath.Join(os.TempDir(), "test_perf.crt")
	keyFile = filepath.Join(os.TempDir(), "test_perf.key")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return "", "", err
	}

	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return "", "", err
	}

	return certFile, keyFile, nil
}

// startProxyServer 启动代理服务器
func startProxyServer(port int, certFile, keyFile string) (*exec.Cmd, error) {
	cmd := exec.Command("./utlsProxy",
		"-listen", fmt.Sprintf("127.0.0.1:%d", port),
		"-token", "test-token-perf",
		"-cert", certFile,
		"-key", keyFile,
		"-log", "error", // 减少日志输出
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// 等待服务器启动
	time.Sleep(2 * time.Second)

	// 检查服务器是否在运行
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return nil, fmt.Errorf("服务器启动失败")
	}

	return cmd, nil
}

// testConnectionSpeed 测试连接建立速度
func testConnectionSpeed(proxyAddr string, iterations int) time.Duration {
	var totalTime time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()

		// 解析地址
		addr, _ := net.ResolveUDPAddr("udp", proxyAddr)
		udpConn, _ := net.ListenUDP("udp", nil)
		udpConn.SetReadBuffer(8 * 1024 * 1024)
		udpConn.SetWriteBuffer(8 * 1024 * 1024)

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"tuic"},
		}

		quicConfig := &quic.Config{
			KeepAlivePeriod: 5 * time.Second,
			MaxIdleTimeout:  60 * time.Second,
			MaxIncomingStreams: 1000,
			InitialStreamReceiveWindow: 8 * 1024 * 1024,
			InitialConnectionReceiveWindow: 16 * 1024 * 1024,
			Allow0RTT: true,
		}

		conn, err := quic.Dial(context.Background(), udpConn, addr, tlsConfig, quicConfig)
		if err != nil {
			continue
		}

		conn.CloseWithError(0, "test")
		udpConn.Close()

		totalTime += time.Since(start)
	}

	return totalTime / time.Duration(iterations)
}

// testDataTransferSpeed 测试数据传输速度
func testDataTransferSpeed(proxyAddr string, dataSize int, iterations int) (float64, time.Duration) {
	var totalBytes int64
	var totalTime time.Duration

	// 生成测试数据
	testData := make([]byte, dataSize)
	rand.Read(testData)

	for i := 0; i < iterations; i++ {
		start := time.Now()

		// 建立连接
		addr, _ := net.ResolveUDPAddr("udp", proxyAddr)
		udpConn, _ := net.ListenUDP("udp", nil)
		udpConn.SetReadBuffer(8 * 1024 * 1024)
		udpConn.SetWriteBuffer(8 * 1024 * 1024)

		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"tuic"},
		}

		quicConfig := &quic.Config{
			KeepAlivePeriod: 5 * time.Second,
			MaxIdleTimeout:  60 * time.Second,
			MaxIncomingStreams: 1000,
			InitialStreamReceiveWindow: 8 * 1024 * 1024,
			InitialConnectionReceiveWindow: 16 * 1024 * 1024,
			Allow0RTT: true,
		}

		conn, err := quic.Dial(context.Background(), udpConn, addr, tlsConfig, quicConfig)
		if err != nil {
			continue
		}

		// 打开流并传输数据
		stream, err := conn.OpenStreamSync(context.Background())
		if err != nil {
			conn.CloseWithError(0, "test")
			udpConn.Close()
			continue
		}

		// 发送数据
		stream.Write(testData)

		// 读取响应（简化测试，只发送）
		stream.Close()

		conn.CloseWithError(0, "test")
		udpConn.Close()

		totalBytes += int64(dataSize)
		totalTime += time.Since(start)
	}

	throughput := float64(totalBytes) / totalTime.Seconds() / (1024 * 1024) // MB/s
	return throughput, totalTime / time.Duration(iterations)
}

// testConcurrentRequests 测试并发请求性能
func testConcurrentRequests(proxyAddr string, concurrency, requestsPerWorker int) TestResult {
	var successCount, failCount int64
	var totalBytes int64
	var latencies []time.Duration
	var mu sync.Mutex

	startTime := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < requestsPerWorker; j++ {
				reqStart := time.Now()

				// 使用客户端工具发送请求
				cmd := exec.Command("./utlsclient-cli",
					"-proxy", proxyAddr,
					"-token", "test-token-perf",
					"-url", "https://www.google.com",
					"-timeout", "10s",
				)
				cmd.Stdout = nil
				cmd.Stderr = nil

				err := cmd.Run()
				latency := time.Since(reqStart)

				mu.Lock()
				if err == nil {
					atomic.AddInt64(&successCount, 1)
					latencies = append(latencies, latency)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	// 计算统计信息
	var minLatency, maxLatency, sumLatency time.Duration
	if len(latencies) > 0 {
		minLatency = latencies[0]
		maxLatency = latencies[0]
		for _, lat := range latencies {
			if lat < minLatency {
				minLatency = lat
			}
			if lat > maxLatency {
				maxLatency = lat
			}
			sumLatency += lat
		}
	}

	avgLatency := sumLatency / time.Duration(len(latencies))
	rps := float64(successCount) / totalTime.Seconds()

	return TestResult{
		TestName: "并发性能测试",
		Metrics: PerformanceMetrics{
			TotalRequests:     int64(concurrency * requestsPerWorker),
			SuccessRequests:   successCount,
			FailedRequests:    failCount,
			TotalBytes:        totalBytes,
			TotalDuration:     totalTime,
			MinLatency:        minLatency,
			MaxLatency:        maxLatency,
			AvgLatency:        avgLatency,
			RequestsPerSecond: rps,
		},
		Concurrency:  concurrency,
		RequestCount: concurrency * requestsPerWorker,
	}
}

// printResults 打印测试结果
func printResults(results []TestResult) {
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("TUIC 高速传输性能测试报告")
	fmt.Println(strings.Repeat("=", 80))

	for _, result := range results {
		fmt.Printf("\n【%s】\n", result.TestName)
		fmt.Printf("并发数: %d\n", result.Concurrency)
		fmt.Printf("总请求数: %d\n", result.Metrics.TotalRequests)
		fmt.Printf("成功请求: %d\n", result.Metrics.SuccessRequests)
		fmt.Printf("失败请求: %d\n", result.Metrics.FailedRequests)
		fmt.Printf("成功率: %.2f%%\n", float64(result.Metrics.SuccessRequests)/float64(result.Metrics.TotalRequests)*100)
		fmt.Printf("总耗时: %v\n", result.Metrics.TotalDuration)
		fmt.Printf("最小延迟: %v\n", result.Metrics.MinLatency)
		fmt.Printf("最大延迟: %v\n", result.Metrics.MaxLatency)
		fmt.Printf("平均延迟: %v\n", result.Metrics.AvgLatency)
		fmt.Printf("吞吐量: %.2f MB/s\n", result.Metrics.Throughput)
		fmt.Printf("请求/秒: %.2f\n", result.Metrics.RequestsPerSecond)
	}

	fmt.Println("\n" + strings.Repeat("=", 80))
}

func main() {
	fmt.Println("启动 TUIC 高速传输性能测试...")

	// 生成测试证书
	certFile, keyFile, err := generateTestCert()
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成测试证书失败: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	// 启动代理服务器
	proxyPort := 18444
	fmt.Printf("启动代理服务器 (端口: %d)...\n", proxyPort)
	proxyCmd, err := startProxyServer(proxyPort, certFile, keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "启动代理服务器失败: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		proxyCmd.Process.Kill()
		proxyCmd.Wait()
	}()

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", proxyPort)
	fmt.Printf("代理服务器已启动: %s\n", proxyAddr)

	time.Sleep(1 * time.Second)

	var results []TestResult

	// 测试1: 连接建立速度
	fmt.Println("\n【测试1】连接建立速度测试...")
	connTime := testConnectionSpeed(proxyAddr, 10)
	fmt.Printf("平均连接建立时间: %v\n", connTime)
	results = append(results, TestResult{
		TestName: "连接建立速度",
		Metrics: PerformanceMetrics{
			ConnectionTime: connTime,
		},
	})

	// 测试2: 数据传输速度
	fmt.Println("\n【测试2】数据传输速度测试...")
	throughput, avgTime := testDataTransferSpeed(proxyAddr, 1024*1024, 5) // 1MB数据，5次迭代
	fmt.Printf("平均传输时间: %v\n", avgTime)
	fmt.Printf("吞吐量: %.2f MB/s\n", throughput)
	results = append(results, TestResult{
		TestName: "数据传输速度",
		Metrics: PerformanceMetrics{
			Throughput:    throughput,
			AvgLatency:    avgTime,
			TotalBytes:    5 * 1024 * 1024,
		},
		DataSize: 1024 * 1024,
	})

	// 测试3: 并发性能（小规模）
	fmt.Println("\n【测试3】并发性能测试 (5并发, 每并发2请求)...")
	result1 := testConcurrentRequests(proxyAddr, 5, 2)
	results = append(results, result1)

	// 测试4: 并发性能（中等规模）
	fmt.Println("\n【测试4】并发性能测试 (10并发, 每并发5请求)...")
	result2 := testConcurrentRequests(proxyAddr, 10, 5)
	results = append(results, result2)

	// 打印汇总结果
	printResults(results)

	fmt.Println("\n测试完成！")
}


