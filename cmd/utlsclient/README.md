# UTLSClient - HTTP客户端工具

## 概述

UTLSClient 是一个基于 uTLS 的 HTTP 客户端工具，支持两种模式：
1. **直接模式**: 直接使用热连接池发送 HTTP 请求
2. **代理模式**: 通过 TUIC 协议代理服务器发送请求

## 编译

```bash
go build -o utlsclient-cli ./cmd/utlsclient
```

## 使用方法

### 直接模式（默认）

直接使用热连接池发送请求，不经过代理：

```bash
./utlsclient-cli -url https://example.com/path
```

**命令行参数**:
- `-url`: 目标 HTTPS URL（必需）
- `-X`: HTTP 方法，如 GET/POST/HEAD（默认: GET）
- `-ua`: 自定义 User-Agent
- `-timeout`: 请求超时时间（默认: 30s）
- `-head`: 使用 HEAD 请求进行快速探测
- `-version`: 显示版本号

**示例**:
```bash
# GET 请求
./utlsclient-cli -url https://www.google.com

# POST 请求
./utlsclient-cli -url https://api.example.com/data -X POST

# 自定义 User-Agent
./utlsclient-cli -url https://example.com -ua "MyBot/1.0"

# 设置超时
./utlsclient-cli -url https://example.com -timeout 60s
```

### 代理模式

通过 TUIC 协议代理服务器发送请求：

```bash
./utlsclient-cli \
  -use-proxy \
  -proxy your-proxy-server:443 \
  -token your-tuic-token \
  -url https://example.com/path
```

或者使用简化的参数：

```bash
./utlsclient-cli \
  -proxy your-proxy-server:443 \
  -token your-tuic-token \
  -url https://example.com/path
```

**代理模式参数**:
- `-use-proxy` 或 `-proxy`: 启用代理模式并指定代理服务器地址（格式: `host:port`）
- `-token`: TUIC 认证令牌（必需）
- `-url`: 目标 URL（必需）
- `-X`: HTTP 方法（默认: GET）
- `-timeout`: 请求超时时间（默认: 30s）

**示例**:
```bash
# 通过代理发送 GET 请求
./utlsclient-cli \
  -proxy 192.168.1.100:443 \
  -token my-secret-token \
  -url https://www.google.com

# 通过代理发送 POST 请求
./utlsclient-cli \
  -proxy proxy.example.com:443 \
  -token my-secret-token \
  -url https://api.example.com/data \
  -X POST \
  -timeout 60s
```

## 架构说明

### 直接模式架构

```
UTLSClient
    ↓
热连接池 (UTLSHotConnPool)
    ↓
UTLS连接 (TLS指纹伪装)
    ↓
目标服务器 (HTTPS)
```

### 代理模式架构

```
UTLSClient
    ↓ QUIC连接
TUIC代理服务器 (utlsProxy)
    ↓ TUIC协议解析
提取HTTP请求
    ↓ 使用热连接池
UTLSClient (服务器端)
    ↓
目标服务器 (HTTPS)
```

## 功能特性

### 直接模式特性

- ✅ TLS 指纹伪装（支持多种浏览器指纹）
- ✅ HTTP/1.1 和 HTTP/2 自动协商
- ✅ 热连接池复用，提升性能
- ✅ 连接健康检查
- ✅ 自动重试机制

### 代理模式特性

- ✅ TUIC v5 协议支持
- ✅ QUIC 传输，低延迟
- ✅ 通过代理服务器转发请求
- ✅ 支持所有 HTTP 方法
- ✅ 完整的错误处理

## 使用场景

### 直接模式适用场景

- 本地开发测试
- 需要直接控制连接参数
- 需要查看连接池统计信息
- 性能测试和基准测试

### 代理模式适用场景

- 需要通过 VPS 代理访问
- 需要隐藏真实 IP 地址
- 需要绕过网络限制
- 分布式爬虫系统

## 注意事项

1. **TUIC Token**: 代理模式下必须提供正确的认证令牌
2. **网络连接**: 确保能够访问代理服务器
3. **TLS证书**: 代理服务器可能使用自签名证书，客户端会自动跳过验证
4. **超时设置**: 根据网络情况调整超时时间

## 故障排除

### 直接模式问题

**连接失败**:
- 检查目标 URL 是否正确
- 检查网络连接
- 查看日志输出

**性能问题**:
- 调整连接池配置
- 检查目标服务器响应时间

### 代理模式问题

**连接代理服务器失败**:
- 检查代理服务器地址和端口
- 检查防火墙设置
- 验证网络连接

**认证失败**:
- 验证 TUIC token 是否正确
- 检查服务器端 token 配置

**请求超时**:
- 增加 `-timeout` 参数值
- 检查网络延迟
- 检查代理服务器状态

## 开发说明

### 代码结构

- `main.go`: 主程序入口，参数解析和模式选择
- `tuic_client.go`: TUIC 客户端实现，包含 QUIC 连接和协议处理

### 扩展功能

- [ ] 支持 SOCKS5 代理
- [ ] 支持 HTTP 代理
- [ ] 支持连接池配置
- [ ] 支持请求/响应日志记录
- [ ] 支持批量请求
