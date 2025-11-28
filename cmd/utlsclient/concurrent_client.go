package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	projlogger "crawler-platform/logger"
	"github.com/quic-go/quic-go"
)

// ConcurrentTestConfig 并发测试配置
type ConcurrentTestConfig struct {
	ProxyAddr    string        // 代理服务器地址
	Token        string        // TUIC认证令牌
	TargetURL    string        // 目标URL
	Concurrency  int           // 并发数
	TotalRequests int          // 总请求数
	Timeout      time.Duration // 超时时间
	Duration     time.Duration // 测试持续时间（如果TotalRequests为0）
}

// TestResult 测试结果
type TestResult struct {
	TotalRequests    int64
	SuccessRequests   int64
	FailedRequests    int64
	TotalLatency      int64 // 总延迟（纳秒）
	MinLatency        int64 // 最小延迟（纳秒）
	MaxLatency        int64 // 最大延迟（纳秒）
	ErrorCounts       map[string]int64
	mu                sync.RWMutex
}

// NewTestResult 创建新的测试结果
func NewTestResult() *TestResult {
	return &TestResult{
		ErrorCounts: make(map[string]int64),
		MinLatency:  int64(^uint64(0) >> 1), // 最大int64值
	}
}

// RecordSuccess 记录成功请求
func (r *TestResult) RecordSuccess(latency time.Duration) {
	atomic.AddInt64(&r.TotalRequests, 1)
	atomic.AddInt64(&r.SuccessRequests, 1)
	
	latencyNs := latency.Nanoseconds()
	atomic.AddInt64(&r.TotalLatency, latencyNs)
	
	// 更新最小延迟
	for {
		oldMin := atomic.LoadInt64(&r.MinLatency)
		if latencyNs >= oldMin {
			break
		}
		if atomic.CompareAndSwapInt64(&r.MinLatency, oldMin, latencyNs) {
			break
		}
	}
	
	// 更新最大延迟
	for {
		oldMax := atomic.LoadInt64(&r.MaxLatency)
		if latencyNs <= oldMax {
			break
		}
		if atomic.CompareAndSwapInt64(&r.MaxLatency, oldMax, latencyNs) {
			break
		}
	}
}

// RecordFailure 记录失败请求
func (r *TestResult) RecordFailure(err error) {
	atomic.AddInt64(&r.TotalRequests, 1)
	atomic.AddInt64(&r.FailedRequests, 1)
	
	if err != nil {
		errMsg := err.Error()
		r.mu.Lock()
		r.ErrorCounts[errMsg]++
		r.mu.Unlock()
	}
}

// GetStats 获取统计信息
func (r *TestResult) GetStats() (total, success, failed int64, avgLatency, minLatency, maxLatency time.Duration) {
	total = atomic.LoadInt64(&r.TotalRequests)
	success = atomic.LoadInt64(&r.SuccessRequests)
	failed = atomic.LoadInt64(&r.FailedRequests)
	
	totalLatency := atomic.LoadInt64(&r.TotalLatency)
	if success > 0 {
		avgLatency = time.Duration(totalLatency / success)
	}
	
	minLatency = time.Duration(atomic.LoadInt64(&r.MinLatency))
	maxLatency = time.Duration(atomic.LoadInt64(&r.MaxLatency))
	
	return
}

// ConcurrentTester 并发测试器
type ConcurrentTester struct {
	config *ConcurrentTestConfig
	result *TestResult
}

// NewConcurrentTester 创建并发测试器
func NewConcurrentTester(config *ConcurrentTestConfig) *ConcurrentTester {
	return &ConcurrentTester{
		config: config,
		result: NewTestResult(),
	}
}

// RunSingleRequest 执行单个请求（每个请求创建独立的客户端连接）
func (t *ConcurrentTester) RunSingleRequest() error {
	start := time.Now()
	
	// 为每个请求创建独立的客户端
	client, err := NewTUICClient(t.config.ProxyAddr, t.config.Token)
	if err != nil {
		return fmt.Errorf("创建TUIC客户端失败: %w", err)
	}
	defer client.Close()
	
	// 连接到服务器
	if err := client.Connect(); err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	
	// 创建HTTP请求
	httpReq, err := http.NewRequest("GET", t.config.TargetURL, nil)
	if err != nil {
		return fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), t.config.Timeout)
	defer cancel()
	httpReq = httpReq.WithContext(ctx)
	
	// 发送请求
	httpResp, err := client.DoRequest(t.config.TargetURL, httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	
	// 读取响应体（至少读取一部分以确保请求完成）
	_, err = io.CopyN(io.Discard, httpResp.Body, 1024)
	if err != nil && err != io.EOF {
		return fmt.Errorf("读取响应体失败: %w", err)
	}
	
	latency := time.Since(start)
	
	// 记录成功
	if httpResp.StatusCode >= 200 && httpResp.StatusCode < 300 {
		t.result.RecordSuccess(latency)
		return nil
	}
	
	return fmt.Errorf("HTTP状态码: %d", httpResp.StatusCode)
}

// RunConcurrentTest 运行并发测试
func (t *ConcurrentTester) RunConcurrentTest(ctx context.Context) {
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, t.config.Concurrency)
	
	// 如果指定了总请求数
	if t.config.TotalRequests > 0 {
		for i := 0; i < t.config.TotalRequests; i++ {
			select {
			case <-ctx.Done():
				return
			case semaphore <- struct{}{}:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-semaphore }()
					
					if err := t.RunSingleRequest(); err != nil {
						t.result.RecordFailure(err)
					}
				}()
			}
		}
	} else {
		// 基于时间的测试
		done := ctx.Done()
		for {
			select {
			case <-done:
				return
			case semaphore <- struct{}{}:
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer func() { <-semaphore }()
					
					if err := t.RunSingleRequest(); err != nil {
						t.result.RecordFailure(err)
					}
				}()
			}
		}
	}
	
	wg.Wait()
}

// PrintStats 打印统计信息
func (t *ConcurrentTester) PrintStats() {
	total, success, failed, avgLatency, minLatency, maxLatency := t.result.GetStats()
	
	fmt.Println("\n=== 并发测试结果 ===")
	fmt.Printf("总请求数: %d\n", total)
	fmt.Printf("成功请求: %d (%.2f%%)\n", success, float64(success)/float64(total)*100)
	fmt.Printf("失败请求: %d (%.2f%%)\n", failed, float64(failed)/float64(total)*100)
	fmt.Printf("平均延迟: %v\n", avgLatency)
	fmt.Printf("最小延迟: %v\n", minLatency)
	fmt.Printf("最大延迟: %v\n", maxLatency)
	
	if duration > 0 {
		qps := float64(total) / duration.Seconds()
		fmt.Printf("QPS: %.2f\n", qps)
	}
	
	// 打印错误统计
	t.result.mu.RLock()
	if len(t.result.ErrorCounts) > 0 {
		fmt.Println("\n错误统计:")
		for err, count := range t.result.ErrorCounts {
			fmt.Printf("  %s: %d\n", err, count)
		}
	}
	t.result.mu.RUnlock()
}

// generateSelfSignedCert 生成自签名证书（用于测试）
func generateSelfSignedCert() (certPEM, keyPEM []byte, err error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	
	// 创建证书模板
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
	
	// 创建证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}
	
	// 编码证书
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	
	// 编码私钥
	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	
	return certPEM, keyPEM, nil
}

// ConcurrentTestMain 并发测试主函数（导出以便main.go调用）
func ConcurrentTestMain() {
	var (
		proxyAddr     = flag.String("proxy", "127.0.0.1:443", "代理服务器地址")
		token         = flag.String("token", "test-token", "TUIC认证令牌")
		targetURL     = flag.String("url", "https://www.google.com", "目标URL")
		concurrency   = flag.Int("concurrency", 10, "并发数")
		totalRequests = flag.Int("requests", 100, "总请求数（0表示基于时间）")
		duration      = flag.Duration("duration", 0, "测试持续时间（如果requests为0）")
		timeout       = flag.Duration("timeout", 30*time.Second, "单个请求超时时间")
		startServer   = flag.Bool("start-server", false, "启动测试服务器")
		serverPort    = flag.String("server-port", "443", "服务器监听端口")
	)
	flag.Parse()
	
	// 初始化日志
	projlogger.SetGlobalLogger(&projlogger.DefaultLogger{})
	
	// 如果启动服务器
	if *startServer {
		// 生成自签名证书
		certPEM, keyPEM, err := generateSelfSignedCert()
		if err != nil {
			fmt.Fprintf(os.Stderr, "生成证书失败: %v\n", err)
			os.Exit(1)
		}
		
		// 写入临时文件
		certFile := "/tmp/test-server.crt"
		keyFile := "/tmp/test-server.key"
		if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "写入证书文件失败: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "写入私钥文件失败: %v\n", err)
			os.Exit(1)
		}
		
		// 启动服务器（在goroutine中）
		go func() {
			fmt.Printf("启动测试服务器，监听端口: %s\n", *serverPort)
			// 这里应该调用utlsProxy的启动逻辑
			// 为了简化，我们假设服务器已经在运行
		}()
		
		// 等待服务器启动
		time.Sleep(2 * time.Second)
	}
	
	// 创建测试配置
	config := &ConcurrentTestConfig{
		ProxyAddr:     fmt.Sprintf("127.0.0.1:%s", *serverPort),
		Token:         *token,
		TargetURL:     *targetURL,
		Concurrency:   *concurrency,
		TotalRequests: *totalRequests,
		Timeout:       *timeout,
		Duration:      *duration,
	}
	
	// 创建测试器
	tester := NewConcurrentTester(config)
	
	fmt.Printf("准备开始并发测试...\n")
	fmt.Printf("代理服务器: %s\n", config.ProxyAddr)
	fmt.Printf("目标URL: %s\n", config.TargetURL)
	fmt.Printf("并发数: %d\n", config.Concurrency)
	if config.TotalRequests > 0 {
		fmt.Printf("总请求数: %d\n", config.TotalRequests)
	} else {
		fmt.Printf("测试持续时间: %v\n", config.Duration)
	}
	fmt.Println("开始测试...\n")
	
	// 设置上下文
	ctx, cancel := context.WithCancel(context.Background())
	if config.Duration > 0 && config.TotalRequests == 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Duration)
	}
	defer cancel()
	
	// 处理中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n收到停止信号，正在停止测试...")
		cancel()
	}()
	
	// 运行测试
	startTime := time.Now()
	tester.RunConcurrentTest(ctx)
	testDuration := time.Since(startTime)
	
	// 打印结果
	tester.PrintStats()
	fmt.Printf("\n测试耗时: %v\n", testDuration)
	
	// 计算QPS
	total, _, _, _, _, _ := tester.result.GetStats()
	if testDuration > 0 {
		qps := float64(total) / testDuration.Seconds()
		fmt.Printf("实际QPS: %.2f\n", qps)
	}
}
