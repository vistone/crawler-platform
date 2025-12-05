package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	var (
		mode      = flag.String("mode", "self", "证书模式: self(自签名域名证书), ip(为IP地址生成自签名证书)")
		domain    = flag.String("domain", "", "域名（用于自签名证书的CN和DNS名称）")
		ips       = flag.String("ips", "", "IP地址列表，逗号分隔（用于IP证书，例如: 1.2.3.4,2001:db8::1）")
		outputDir = flag.String("output", "./certs", "证书输出目录")
		keySize   = flag.Int("key-size", 2048, "RSA密钥长度（2048或4096）")
		validDays = flag.Int("valid-days", 365, "自签名证书有效期（天数）")
	)
	flag.Parse()

	if *outputDir == "" {
		log.Fatal("输出目录不能为空")
	}

	// 确保输出目录存在
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}

	switch *mode {
	case "self":
		// 生成自签名证书（基于域名）
		if *domain == "" {
			log.Fatal("自签名模式需要指定 -domain 参数")
		}
		// 如果同时指定了 IP，也包含在证书中
		var ipAddrs []net.IP
		if *ips != "" {
			ipAddrs = parseIPs(*ips)
		}
		if err := generateSelfSignedCert(*domain, ipAddrs, *outputDir, *keySize, *validDays); err != nil {
			log.Fatalf("生成自签名证书失败: %v", err)
		}
		log.Printf("✅ 自签名证书生成成功，已保存到 %s", *outputDir)

	case "ip":
		// 为 IP 地址生成自签名证书
		if *ips == "" {
			log.Fatal("IP模式需要指定 -ips 参数（逗号分隔的IP地址列表）")
		}
		ipList := parseIPs(*ips)
		if len(ipList) == 0 {
			log.Fatal("无效的IP地址列表")
		}
		if err := generateIPCert(ipList, *outputDir, *keySize, *validDays); err != nil {
			log.Fatalf("生成IP证书失败: %v", err)
		}
		log.Printf("✅ IP证书生成成功，已保存到 %s", *outputDir)

	default:
		log.Fatalf("未知模式: %s，支持的模式: self, ip", *mode)
	}
}

// generateSelfSignedCert 生成自签名证书（基于域名）
func generateSelfSignedCert(domain string, ipAddrs []net.IP, outputDir string, keySize, validDays int) error {
	log.Printf("生成自签名证书，域名: %s", domain)

	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	// 创建证书模板
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Crawler Platform"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
			CommonName:    domain,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, validDays),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{domain},
		IPAddresses:           ipAddrs,
	}

	// 自签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("创建证书失败: %w", err)
	}

	// 保存证书
	certFile := filepath.Join(outputDir, "server.crt")
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return fmt.Errorf("保存证书失败: %w", err)
	}

	// 保存私钥
	keyFile := filepath.Join(outputDir, "server.key")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("保存私钥失败: %w", err)
	}

	log.Printf("证书已保存: %s", certFile)
	log.Printf("私钥已保存: %s", keyFile)

	return nil
}

// generateIPCert 为 IP 地址生成自签名证书
func generateIPCert(ips []net.IP, outputDir string, keySize, validDays int) error {
	log.Printf("生成IP证书，IP地址: %v", ips)

	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("生成私钥失败: %w", err)
	}

	// 创建证书模板
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("生成序列号失败: %w", err)
	}

	// 构建 CN（使用第一个 IP）
	cn := ips[0].String()
	if len(ips) > 1 {
		cn = fmt.Sprintf("IP Certificate (%d IPs)", len(ips))
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization:  []string{"Crawler Platform"},
			Country:       []string{"CN"},
			CommonName:    cn,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(0, 0, validDays),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           ips,
	}

	// 自签名
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("创建证书失败: %w", err)
	}

	// 保存证书
	certFile := filepath.Join(outputDir, "server.crt")
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	if err := os.WriteFile(certFile, certPEM, 0644); err != nil {
		return fmt.Errorf("保存证书失败: %w", err)
	}

	// 保存私钥
	keyFile := filepath.Join(outputDir, "server.key")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("保存私钥失败: %w", err)
	}

	log.Printf("证书已保存: %s", certFile)
	log.Printf("私钥已保存: %s", keyFile)
	log.Printf("⚠️  注意: 这是自签名证书，客户端需要手动信任或配置跳过验证")

	return nil
}

// parseIPs 解析逗号分隔的IP地址列表
func parseIPs(ipStr string) []net.IP {
	parts := strings.Split(ipStr, ",")
	ips := make([]net.IP, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		ip := net.ParseIP(part)
		if ip != nil {
			ips = append(ips, ip)
		} else {
			log.Printf("警告: 无效的IP地址: %s", part)
		}
	}

	return ips
}

