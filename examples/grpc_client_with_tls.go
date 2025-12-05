//go:build ignore
// +build ignore

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"os"

	"crawler-platform/cmd/grpcserver/tasksmanager"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// 示例：如何配置 gRPC 客户端连接使用 TLS 的服务器
// 支持三种模式：
// 1. 不安全连接（仅用于开发/测试）
// 2. 信任自签名证书（用于生产环境，使用自签名 IP 证书）
// 3. 使用系统 CA 信任的证书（用于生产环境，使用商业 CA 证书）

func main() {
	serverAddr := "1.2.3.4:50051" // 替换为你的 VPS IP 和端口
	certFile := "./certs/server.crt" // 自签名证书路径（可选）

	// 方式1：不安全连接（仅用于开发/测试，不推荐生产环境）
	conn1, err := connectInsecure(serverAddr)
	if err != nil {
		log.Printf("不安全连接失败: %v", err)
	} else {
		defer conn1.Close()
		log.Println("✅ 不安全连接成功（仅用于开发/测试）")
		testConnection(conn1)
	}

	// 方式2：信任自签名证书（推荐用于生产环境，使用自签名 IP 证书）
	conn2, err := connectWithSelfSignedCert(serverAddr, certFile)
	if err != nil {
		log.Printf("自签名证书连接失败: %v", err)
	} else {
		defer conn2.Close()
		log.Println("✅ 自签名证书连接成功")
		testConnection(conn2)
	}

	// 方式3：使用系统 CA 信任的证书（如果使用商业 CA 证书，直接这样即可）
	// conn3, err := connectWithSystemCA(serverAddr)
	// if err != nil {
	// 	log.Printf("系统 CA 连接失败: %v", err)
	// } else {
	// 	defer conn3.Close()
	// 	log.Println("✅ 系统 CA 连接成功")
	// 	testConnection(conn3)
	// }
}

// connectInsecure 不安全连接（仅用于开发/测试）
func connectInsecure(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// connectWithSelfSignedCert 使用自签名证书连接（用于生产环境）
// 这种方式会信任指定的自签名证书，适合使用工具生成的 IP 证书
func connectWithSelfSignedCert(addr, certFile string) (*grpc.ClientConn, error) {
	// 如果证书文件不存在，返回错误
	if certFile == "" {
		return nil, fmt.Errorf("证书文件路径不能为空")
	}

	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("读取证书文件失败: %w", err)
	}

	// 创建证书池
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(certPEM) {
		return nil, fmt.Errorf("无法解析证书文件")
	}

	// 创建 TLS 配置
	// 注意：这里不设置 ServerName，因为证书是为 IP 地址签发的
	// 如果证书是为域名签发的，需要设置 ServerName
	tlsConfig := &tls.Config{
		RootCAs: certPool,
		// ServerName: "example.com", // 如果证书是为域名签发的，需要设置这个
	}

	creds := credentials.NewTLS(tlsConfig)
	return grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
}

// connectWithSystemCA 使用系统 CA 信任的证书连接（如果使用商业 CA 证书）
func connectWithSystemCA(addr string) (*grpc.ClientConn, error) {
	// 使用系统默认的 CA 证书池
	creds := credentials.NewTLS(&tls.Config{
		// 使用系统默认的 RootCAs
	})
	return grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
}

// testConnection 测试连接
func testConnection(conn *grpc.ClientConn) {
	client := tasksmanager.NewTasksManagerClient(conn)
	ctx := context.Background()

	// 测试获取客户端列表
	resp, err := client.GetTaskClientInfoList(ctx, &tasksmanager.TaskClientInfoListRequest{})
	if err != nil {
		log.Printf("测试请求失败: %v", err)
		return
	}

	fmt.Printf("✅ 连接测试成功，客户端数量: %d\n", len(resp.Items))
}

