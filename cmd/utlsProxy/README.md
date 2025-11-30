# UTLSProxy - 基于QUIC的代理服务器

## ⚠️ 重要说明

**TUIC协议的正确理解**：

**TUIC ≠ TUN + QUIC**

- **TUN设备**：网络层（L3）虚拟设备，用于捕获/注入IP数据包
- **QUIC协议**：传输层（L4）协议，提供可靠、安全、高效的数据传输
- **TUIC协议**：应用层（L7）协议，专门用于代理流量的数据交换协议

**当前实现状态**：

- ✅ **QUIC传输层**（L4）：完全实现
- ✅ **TUIC协议层**（L7）：基本实现，支持CONNECT和PACKET命令
- ✅ **IP数据包处理**（L3）：已实现，支持TCP/UDP数据包解析和转发
- ⚠️ **协议完整性**：部分实现，部分特性待完善

## 概述

UTLSProxy 是一个基于 QUIC 协议的代理服务器，部署在 VPS 上，客户端通过 QUIC 连接与服务器通信，服务器端通过 utlsclient 处理目标请求。

## 特性

- **QUIC传输层**: 基于 QUIC 的高性能传输协议
- **TCP代理**: 支持TCP连接代理转发
- **uTLS 集成**: 使用 utlsclient 处理所有 HTTP 请求，支持 TLS 指纹伪装
- **热连接池**: 复用连接，提升性能
- **配置灵活**: 支持命令行参数和配置文件
- **优雅关闭**: 支持信号处理和资源清理

## ⚠️ 实现状态

### 已实现的功能

1. **QUIC传输层**：完全实现，支持TLS、多流、0-RTT等特性
2. **TUIC协议**：
   - ✅ CONNECT命令：TCP代理模式
   - ✅ PACKET命令：IP数据包隧道模式
3. **IP数据包处理**：
   - ✅ IPv4/IPv6数据包解析
   - ✅ TCP连接管理和状态机
   - ✅ UDP数据包转发
   - ✅ IP数据包封装和回传

### 待完善的功能

1. **协议支持**：ICMP消息处理（待实现）
2. **协议完整性**：
   - ⚠️ Token验证机制（基本实现，待完善）
   - ⚠️ 协议版本协商（待实现）
   - ⚠️ 完整的错误码定义（部分实现）
3. **性能优化**：流量控制、拥塞控制优化

详细说明请参考: [TUIC_PROTOCOL.md](TUIC_PROTOCOL.md)

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

当前实现支持基本的TUIC v5协议格式。完整的TUIC V5协议标准请参考：

**官方资源**：
- [TUIC V5 协议规范](https://github.com/apernet/tuic/blob/v5/protocol.md) - **官方 V5 规范文档**
- [TUIC 主仓库](https://github.com/apernet/tuic) - V5 分支

**详细文档**：
- [TUIC_V5_STANDARD.md](TUIC_V5_STANDARD.md) - TUIC V5 标准实现指南
- [TUIC_PROTOCOL.md](TUIC_PROTOCOL.md) - 协议实现说明
- [TUIC_PROTOCOL_CLARIFICATION.md](TUIC_PROTOCOL_CLARIFICATION.md) - 协议理解澄清

**参考实现**：
- [TUIC-Server (Go)](https://github.com/apernet/tuic-go) - 官方 Go 实现

### 扩展功能

- [ ] 完整的TUIC token验证
- [ ] 支持更多TUIC协议特性
- [ ] 性能监控和统计
- [ ] 配置热重载
