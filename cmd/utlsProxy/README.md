# UTLSProxy - 基于QUIC的代理服务器

## ⚠️ 重要说明

**当前实现状态**: 本实现目前是**基于QUIC的TCP代理服务器**，而不是完整的TUIC协议实现。

虽然使用了QUIC协议作为传输层，但**并没有实现真正的TUN层功能**（IP数据包隧道）。当前实现本质上是：
- QUIC传输层（已实现）
- TCP连接代理（已实现）
- **缺少真正的TUN层**（IP数据包处理）

## 概述

UTLSProxy 是一个基于 QUIC 协议的代理服务器，部署在 VPS 上，客户端通过 QUIC 连接与服务器通信，服务器端通过 utlsclient 处理目标请求。

## 特性

- **QUIC传输层**: 基于 QUIC 的高性能传输协议
- **TCP代理**: 支持TCP连接代理转发
- **uTLS 集成**: 使用 utlsclient 处理所有 HTTP 请求，支持 TLS 指纹伪装
- **热连接池**: 复用连接，提升性能
- **配置灵活**: 支持命令行参数和配置文件
- **优雅关闭**: 支持信号处理和资源清理

## ⚠️ 实现限制

### 当前实现的不足

1. **不是真正的TUN层**: 当前实现是TCP代理，不是IP数据包隧道
2. **协议简化**: 没有完全实现TUIC v5协议的所有特性
3. **功能限制**: 只支持TCP连接代理，不支持UDP/ICMP等其他协议

### 真正的TUN层应该包括

- IP数据包解析和处理
- 支持所有IP层协议（TCP/UDP/ICMP等）
- 网络层路由和转发
- 完整的协议状态管理

详细说明请参考: [README_TUN.md](README_TUN.md)

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
