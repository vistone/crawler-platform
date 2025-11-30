package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestProxyIntegration 集成测试：启动服务器和客户端，进行真实测试
func TestProxyIntegration(t *testing.T) {
	// 跳过集成测试，除非设置环境变量
	if os.Getenv("RUN_INTEGRATION_TEST") == "" {
		t.Skip("跳过集成测试，设置 RUN_INTEGRATION_TEST=1 来运行")
	}

	// 生成测试证书
	certFile, keyFile, err := generateTestCert(t)
	if err != nil {
		t.Fatalf("生成测试证书失败: %v", err)
	}
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	// 启动代理服务器
	proxyPort := 18443
	proxyCmd := exec.Command("./utlsProxy",
		"-listen", fmt.Sprintf("127.0.0.1:%d", proxyPort),
		"-token", "test-token-123",
		"-cert", certFile,
		"-key", keyFile,
		"-log", "info",
	)
	proxyCmd.Dir = "."

	proxyStdout, _ := proxyCmd.StdoutPipe()
	proxyStderr, _ := proxyCmd.StderrPipe()
	
	// 启动goroutine读取并输出代理服务器日志
	go func() {
		io.Copy(os.Stdout, proxyStdout)
	}()
	go func() {
		io.Copy(os.Stderr, proxyStderr)
	}()
	
	if err := proxyCmd.Start(); err != nil {
		t.Fatalf("启动代理服务器失败: %v", err)
	}
	defer func() {
		proxyCmd.Process.Kill()
		proxyCmd.Wait()
	}()

	// 等待服务器启动
	time.Sleep(2 * time.Second)

	// 检查服务器是否在运行
	if proxyCmd.ProcessState != nil && proxyCmd.ProcessState.Exited() {
		stderr, _ := io.ReadAll(proxyStderr)
		t.Fatalf("代理服务器启动失败: %s", string(stderr))
	}

	t.Logf("代理服务器已启动 (PID: %d)", proxyCmd.Process.Pid)

	// 运行客户端测试
	testURL := "https://www.google.com"
	clientCmd := exec.Command("./utlsclient-cli",
		"-proxy", fmt.Sprintf("127.0.0.1:%d", proxyPort),
		"-token", "test-token-123",
		"-url", testURL,
		"-timeout", "10s",
	)
	clientCmd.Dir = "."

	clientStdout, _ := clientCmd.StdoutPipe()
	clientStderr, _ := clientCmd.StderrPipe()

	if err := clientCmd.Start(); err != nil {
		t.Fatalf("启动客户端失败: %v", err)
	}

	// 等待客户端完成（最多30秒）
	done := make(chan error, 1)
	go func() {
		done <- clientCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			stderr, _ := io.ReadAll(clientStderr)
			stdout, _ := io.ReadAll(clientStdout)
			t.Errorf("客户端测试失败: %v\nSTDOUT:\n%s\nSTDERR:\n%s", err, string(stdout), string(stderr))
		} else {
			stdout, _ := io.ReadAll(clientStdout)
			t.Logf("客户端测试成功:\n%s", string(stdout))
		}
	case <-time.After(30 * time.Second):
		clientCmd.Process.Kill()
		t.Fatal("客户端测试超时")
	}
}

// generateTestCert 生成测试用的自签名证书
func generateTestCert(t *testing.T) (certFile, keyFile string, err error) {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
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
		return "", "", err
	}

	// 创建临时文件
	certFile = filepath.Join(t.TempDir(), "test.crt")
	keyFile = filepath.Join(t.TempDir(), "test.key")

	// 写入证书
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return "", "", err
	}

	// 写入私钥
	keyDER := x509.MarshalPKCS1PrivateKey(privateKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return "", "", err
	}

	return certFile, keyFile, nil
}
