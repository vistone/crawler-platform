# 证书管理说明

本目录用于存放 TLS 证书和私钥文件。

## 证书类型说明

### 1. 域名证书（推荐）

**适用场景**：如果你有域名，推荐使用域名证书。

**优点**：
- 可以使用 Let's Encrypt 免费申请
- 浏览器自动信任
- 支持自动续期

**申请方式**：
- **Let's Encrypt（免费）**：使用 `certbot` 或 `acme.sh` 等工具自动申请
- **商业 CA**：从 DigiCert、GlobalSign 等购买

**示例（使用 certbot）**：
```bash
# 安装 certbot
sudo apt-get install certbot

# 申请证书（需要域名已解析到服务器IP）
sudo certbot certonly --standalone -d your-domain.com

# 证书会保存在 /etc/letsencrypt/live/your-domain.com/
# 复制到 certs 目录：
sudo cp /etc/letsencrypt/live/your-domain.com/fullchain.pem certs/server.crt
sudo cp /etc/letsencrypt/live/your-domain.com/privkey.pem certs/server.key
```

### 2. IP 地址证书

**适用场景**：只有公网 IP，没有域名。

**限制**：
- ❌ **Let's Encrypt 不支持**直接为 IP 地址签发证书
- ✅ 可以使用**商业 CA**（如 GlobalSign、DigiCert）申请，但需要：
  - 验证 IP 所有权
  - 提供企业资质（OV 证书）
  - 费用较高（通常 $200-500/年）
- ✅ 可以使用**自签名证书**（免费，但客户端需要手动信任）

**自签名证书生成**：
```bash
# 为 IPv4 和 IPv6 生成自签名证书
go run tools/cert_manager/main.go -mode ip \
  -ips "1.2.3.4,2001:db8::1" \
  -output ./certs \
  -key-size 2048 \
  -valid-days 365
```

**商业 CA 申请 IP 证书**：
1. 选择支持 IP 证书的 CA（如 GlobalSign、DigiCert）
2. 生成 CSR（证书签名请求）
3. 提交申请并提供 IP 所有权证明
4. 完成验证后下载证书

### 3. 混合方案（推荐用于公网 IP）

**最佳实践**：
1. **如果有域名**：使用域名证书（Let's Encrypt 免费）
2. **如果只有 IP**：
   - **开发/测试环境**：使用自签名证书
   - **生产环境**：
     - 方案 A：购买域名，使用 Let's Encrypt（最经济）
     - 方案 B：使用商业 CA 的 IP 证书（成本较高）
     - 方案 C：使用自签名证书 + 客户端配置信任（适合内网或可控客户端）

## 使用工具生成自签名证书

### 生成域名自签名证书

```bash
go run tools/cert_manager/main.go -mode self \
  -domain "example.com" \
  -output ./certs \
  -key-size 2048 \
  -valid-days 365
```

### 生成 IP 地址自签名证书（支持 IPv4 和 IPv6）

```bash
# 单个 IPv4
go run tools/cert_manager/main.go -mode ip \
  -ips "1.2.3.4" \
  -output ./certs

# 多个 IP（IPv4 + IPv6）
go run tools/cert_manager/main.go -mode ip \
  -ips "1.2.3.4,2001:db8::1,192.168.1.1" \
  -output ./certs \
  -key-size 4096 \
  -valid-days 730
```

### 生成包含域名和 IP 的自签名证书

```bash
go run tools/cert_manager/main.go -mode self \
  -domain "example.com" \
  -ips "1.2.3.4,2001:db8::1" \
  -output ./certs
```

## 文件命名规范

工具会生成以下文件：
- `server.crt` - 证书文件（PEM 格式）
- `server.key` - 私钥文件（PEM 格式）

gRPC 服务器的 `LoadTLSConfigFromCertsDir` 函数会自动识别这些文件。

## 证书验证

### 查看证书信息

```bash
# 查看证书内容
openssl x509 -in certs/server.crt -text -noout

# 查看证书有效期
openssl x509 -in certs/server.crt -noout -dates
```

### 测试证书

```bash
# 使用 openssl 测试
openssl s_client -connect localhost:443 -servername example.com
```

## 注意事项

1. **私钥安全**：`server.key` 文件权限应为 `600`，不要泄露
2. **证书续期**：Let's Encrypt 证书有效期为 90 天，需要设置自动续期
3. **自签名证书**：客户端连接时会提示证书不受信任，需要：
   - 手动导入 CA 证书到客户端信任列表
   - 或在客户端配置中跳过证书验证（仅用于开发/测试）
4. **IP 证书限制**：
   - 自签名 IP 证书：客户端需要手动信任
   - 商业 CA IP 证书：浏览器可能仍会显示警告（因为 IP 证书不如域名证书常见）

## 常见问题

### Q: Let's Encrypt 可以为 IP 地址签发证书吗？
A: 不可以。Let's Encrypt 只支持域名证书，不支持 IP 地址证书。

### Q: 如何为公网 IPv4 和 IPv6 申请证书？
A: 
- **推荐方案**：购买域名，使用 Let's Encrypt 申请域名证书（证书会自动覆盖域名解析到的所有 IP）
- **备选方案**：使用商业 CA 申请 IP 证书（费用较高）
- **开发方案**：使用自签名证书（免费，但需要客户端信任）

### Q: 自签名证书可以用于生产环境吗？
A: 可以，但需要：
1. 客户端能够配置信任该证书
2. 确保私钥安全
3. 定期更新证书（在过期前）

### Q: 如何实现证书自动续期？
A: 
- **Let's Encrypt**：使用 `certbot` 的 `--renew` 命令，配合 cron 定时任务
- **自签名证书**：编写脚本定期重新生成证书

## gRPC 客户端配置

### 使用自签名 IP 证书连接 gRPC 服务器

如果你的 gRPC 服务器使用自签名 IP 证书，客户端需要配置信任该证书。参考 `examples/grpc_client_with_tls.go` 中的示例代码。

**关键代码**：
```go
// 读取服务器证书
certPEM, err := os.ReadFile("./certs/server.crt")
certPool := x509.NewCertPool()
certPool.AppendCertsFromPEM(certPEM)

// 创建 TLS 配置（不设置 ServerName，因为证书是为 IP 签发的）
tlsConfig := &tls.Config{
    RootCAs: certPool,
}

creds := credentials.NewTLS(tlsConfig)
conn, err := grpc.NewClient("1.2.3.4:50051", grpc.WithTransportCredentials(creds))
```

**重要提示**：
- 自签名证书是**免费的**，但需要客户端手动配置信任
- 商业 CA 签发的 IP 证书是**收费的**（$200-500/年），但客户端自动信任
- 如果有域名，推荐使用 Let's Encrypt（免费且自动信任）

## 相关资源

- [Let's Encrypt 官网](https://letsencrypt.org/)
- [certbot 文档](https://certbot.eff.org/)
- [GlobalSign IP 证书](https://www.globalsign.cn/ssl-certificates/ov-ssl-certificate)
- [RFC 5280 - X.509 证书标准](https://tools.ietf.org/html/rfc5280)

