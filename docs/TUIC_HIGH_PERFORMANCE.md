# TUIC 高速传输优化指南

## 概述

本文档说明如何优化 TUIC 客户端和服务器端连接，实现高速传输和直接连接，绕过中间路由器。

## 已实现的优化

### 1. QUIC 协议优化

#### 客户端优化 (`cmd/utlsclient/tuic_client.go`)

- **KeepAlive 优化**: 从 10 秒减少到 5 秒，保持连接更活跃
- **空闲超时**: 从 30 秒增加到 60 秒，支持连接复用
- **接收窗口**: 
  - 流接收窗口: 8MB
  - 连接接收窗口: 16MB
- **并发流数**: 增加到 1000，支持高并发
- **0-RTT 支持**: 启用 0-RTT，减少握手延迟

#### 服务器端优化 (`cmd/utlsProxy/main.go`)

- 与客户端相同的 QUIC 配置优化
- 支持长连接复用

### 2. UDP 缓冲区优化

- **接收缓冲区**: 8MB
- **发送缓冲区**: 8MB
- 提高 UDP 传输效率，减少丢包

### 3. 连接复用机制

- **全局连接池**: 相同服务器地址的请求复用同一个 QUIC 连接
- **连接验证**: 自动检测连接有效性
- **自动清理**: 连接在空闲超时后自动关闭

## 网络层面优化建议

### 1. 服务器部署位置

**目标**: 减少网络跳数和延迟

- **靠近目标区域**: 将 TUIC 服务器部署在靠近主要访问目标的地理位置
- **优质网络**: 选择具有优质网络带宽和低延迟的服务器提供商
- **BGP 优化**: 使用支持 BGP 优化的服务器，自动选择最佳路由

### 2. 网络路径优化

#### 使用专用网络

```bash
# 如果可能，使用专用网络连接
# 例如：AWS Direct Connect, Azure ExpressRoute, GCP Interconnect
```

#### 路由优化

- **静态路由**: 配置静态路由，避免通过不必要的路由器
- **BGP 配置**: 如果服务器支持，配置 BGP 以选择最优路径
- **多路径**: 使用多路径传输（如果网络支持）

### 3. 系统层面优化

#### Linux 系统优化

```bash
# 增加 UDP 缓冲区大小（需要 root 权限）
echo 'net.core.rmem_max = 16777216' >> /etc/sysctl.conf
echo 'net.core.wmem_max = 16777216' >> /etc/sysctl.conf
echo 'net.core.rmem_default = 16777216' >> /etc/sysctl.conf
echo 'net.core.wmem_default = 16777216' >> /etc/sysctl.conf

# 应用配置
sysctl -p
```

#### 拥塞控制算法

```bash
# 使用 BBR 拥塞控制算法（推荐）
echo 'net.core.default_qdisc=fq' >> /etc/sysctl.conf
echo 'net.ipv4.tcp_congestion_control=bbr' >> /etc/sysctl.conf

# 注意：QUIC 使用自己的拥塞控制，但系统层面的优化仍然有帮助
sysctl -p
```

### 4. 防火墙和 NAT 优化

```bash
# 优化 UDP 连接跟踪超时
# 对于 QUIC/UDP，需要较长的超时时间
iptables -t raw -A PREROUTING -p udp --dport <TUIC_PORT> -j NOTRACK
```

## 性能测试

### 测试连接速度

```bash
# 使用客户端测试
./utlsclient-cli \
  -proxy your-server:443 \
  -token your-token \
  -url https://www.google.com \
  -timeout 30s
```

### 监控指标

- **连接建立时间**: 应该 < 100ms（使用连接复用后）
- **首字节时间 (TTFB)**: 应该 < 200ms
- **传输速度**: 应该接近网络带宽上限

## 高级优化

### 1. 多服务器负载均衡

如果可能，部署多个 TUIC 服务器，使用 DNS 轮询或智能路由：

```go
// 示例：支持多个服务器地址
servers := []string{
    "server1.example.com:443",
    "server2.example.com:443",
    "server3.example.com:443",
}
```

### 2. 地理位置感知

根据客户端地理位置，自动选择最近的服务器：

```go
// 根据客户端 IP 选择最优服务器
func selectOptimalServer(clientIP string) string {
    // 实现地理位置检测和服务器选择逻辑
    return optimalServer
}
```

### 3. 网络质量检测

实现网络质量检测，自动切换到最优路径：

```go
// 检测服务器延迟和带宽
func measureServerQuality(serverAddr string) QualityMetrics {
    // 实现网络质量检测
    return metrics
}
```

## 故障排查

### 连接速度慢

1. **检查网络延迟**: 使用 `ping` 和 `traceroute` 检查网络路径
2. **检查 UDP 缓冲区**: 确认系统 UDP 缓冲区设置正确
3. **检查防火墙**: 确认 UDP 流量未被限制
4. **检查服务器负载**: 确认服务器有足够的 CPU 和内存

### 连接频繁断开

1. **检查 KeepAlive 设置**: 确认 KeepAlive 间隔合理
2. **检查空闲超时**: 确认 MaxIdleTimeout 足够长
3. **检查网络稳定性**: 使用网络监控工具检查丢包率

## 配置示例

### 客户端配置

```go
// 已优化的默认配置
quicConfig := &quic.Config{
    KeepAlivePeriod: 5 * time.Second,
    MaxIdleTimeout:  60 * time.Second,
    MaxIncomingStreams: 1000,
    InitialStreamReceiveWindow: 8 * 1024 * 1024,
    InitialConnectionReceiveWindow: 16 * 1024 * 1024,
    Allow0RTT: true,
}
```

### 服务器配置

```go
// 与客户端相同的优化配置
quicConfig := &quic.Config{
    KeepAlivePeriod: 5 * time.Second,
    MaxIdleTimeout:  60 * time.Second,
    MaxIncomingStreams: 1000,
    InitialStreamReceiveWindow: 8 * 1024 * 1024,
    InitialConnectionReceiveWindow: 16 * 1024 * 1024,
    Allow0RTT: true,
}
```

## 总结

通过以上优化，TUIC 客户端和服务器可以实现：

1. **快速连接**: 使用连接复用和 0-RTT，减少连接建立时间
2. **高速传输**: 大接收窗口和优化的 UDP 缓冲区，提高吞吐量
3. **稳定连接**: 优化的 KeepAlive 和空闲超时，保持连接稳定
4. **高并发**: 支持大量并发流，满足高并发需求

结合网络层面的优化（服务器位置、路由优化等），可以实现接近直连的高速传输效果。

