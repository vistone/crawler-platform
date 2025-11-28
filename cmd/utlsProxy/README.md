# UTLSProxy - 基于TUIC协议的代理服务器

## 概述

UTLSProxy 是一个基于 TUIC (TUIC v5) 协议的代理服务器，部署在 VPS 上，客户端通过 TUIC 协议与服务器握手，然后通过 utlsclient 处理 HTTP 请求。

## 特性

- **TUIC v5 协议支持**: 基于 QUIC 的高性能代理协议
- **uTLS 集成**: 使用 utlsclient 处理所有 HTTP 请求，支持 TLS 指纹伪装
- **热连接池**: 复用连接，提升性能
- **配置灵活**: 支持命令行参数和配置文件
- **优雅关闭**: 支持信号处理和资源清理

## 编译

```bash
go build -o utlsProxy ./cmd/utlsProxy
```

## 使用方法

### 基本用法

```bash
./utlsProxy \
  -listen 0.0.0.0:443 \
  -token your-tuic-token \
  -cert server.crt \
  -key server.key
```

### 命令行参数

- `-listen`: 监听地址，格式 `host:port` (默认: `0.0.0.0:443`)
- `-token`: TUIC 认证令牌（必需）
- `-cert`: TLS 证书文件路径 (默认: `server.crt`)
- `-key`: TLS 私钥文件路径 (默认: `server.key`)
- `-config`: 连接池配置文件路径（可选，默认使用合并配置）
- `-log`: 日志级别: `debug`, `info`, `warn`, `error` (默认: `info`)

### 配置文件

可以使用 `-config` 参数指定连接池配置文件，或使用项目默认的配置加载机制（合并 `config.toml` 和 `config/config.toml`）。

## 生成TLS证书

在部署前，需要生成TLS证书：

```bash
# 生成自签名证书（仅用于测试）
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes

# 或使用 Let's Encrypt 等CA签发的证书
```

## 部署示例

### systemd 服务配置

创建 `/etc/systemd/system/utlsproxy.service`:

```ini
[Unit]
Description=UTLSProxy TUIC Proxy Server
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/crawler-platform
ExecStart=/path/to/utlsProxy \
  -listen 0.0.0.0:443 \
  -token your-secure-token \
  -cert /path/to/server.crt \
  -key /path/to/server.key \
  -log info
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl enable utlsproxy
sudo systemctl start utlsproxy
```

## 客户端配置

客户端需要使用支持 TUIC v5 协议的客户端，例如：

- TUIC 官方客户端
- 其他支持 TUIC 的代理客户端

客户端配置示例：

```json
{
  "server": "your-vps-ip",
  "port": 443,
  "token": "your-tuic-token",
  "protocol": "tuic",
  "version": 5
}
```

## 架构说明

```
客户端 (TUIC协议)
    ↓
UTLSProxy (QUIC/TLS)
    ↓
TUIC协议解析
    ↓
HTTP请求提取
    ↓
UTLSClient (热连接池)
    ↓
目标服务器 (HTTPS)
```

## 注意事项

1. **TUIC Token**: 必须设置强密码作为 token，确保安全性
2. **TLS证书**: 生产环境应使用有效的TLS证书
3. **防火墙**: 确保VPS防火墙开放相应端口
4. **性能**: 连接池配置会影响性能，根据实际需求调整

## 故障排除

### 连接失败

- 检查防火墙设置
- 验证TLS证书有效性
- 检查日志输出 (`-log debug`)

### 性能问题

- 调整连接池配置（`max_connections`, `max_conns_per_host`）
- 检查网络延迟
- 查看连接池统计信息

## 开发说明

### TUIC协议实现

当前实现是简化版本，支持基本的TUIC v5协议格式。完整的TUIC协议实现可以参考：
- https://github.com/EAimTY/tuic-go
- https://github.com/EAimTY/TUIC

### 扩展功能

- [ ] 完整的TUIC token验证
- [ ] 支持更多TUIC协议特性
- [ ] 性能监控和统计
- [ ] 配置热重载
