# 真正的TUN层实现

## 概述

现在已经实现了真正的IP层隧道功能，支持：
- ✅ IP数据包解析（IPv4/IPv6）
- ✅ TCP连接管理（包含状态机）
- ✅ UDP数据包转发
- ✅ IP数据包封装和回传

## 架构

### 数据流

```
客户端应用
    ↓ 生成IP数据包
TUN设备（客户端）
    ↓ 封装到TUIC协议
QUIC传输
    ↓
utlsProxy服务器
    ↓ 解析TUIC协议
IPTunnelHandler
    ↓ 解析IP数据包
根据协议类型分发
    ↓
TCP/UDP处理
    ↓ 转发到目标服务器
目标服务器响应
    ↓ 封装回IP包
TUIC协议封装
    ↓ QUIC传输
客户端接收
```

## 实现细节

### IP数据包处理

1. **IPv4数据包解析**
   - 解析IP包头（20字节最小长度）
   - 提取源IP、目标IP、协议类型
   - 根据协议类型分发（TCP=6, UDP=17）

2. **IPv6数据包解析**
   - 解析IP包头（40字节最小长度）
   - 提取源IP、目标IP、下一个头类型

### TCP连接管理

1. **连接建立（SYN）**
   - 检测SYN标志
   - 建立到目标服务器的TCP连接
   - 维护连接映射表
   - 启动数据转发goroutine

2. **数据转发**
   - 从客户端接收TCP数据包
   - 提取负载并转发到目标服务器
   - 从目标服务器接收响应
   - 封装回IP包并发送给客户端

3. **连接关闭（FIN/RST）**
   - 检测FIN或RST标志
   - 关闭TCP连接
   - 清理连接映射表

### UDP数据包转发

1. **接收UDP数据包**
   - 解析UDP包头
   - 提取源/目标端口和负载

2. **转发UDP数据**
   - 建立UDP连接
   - 发送数据到目标服务器
   - 异步接收响应
   - 封装回IP包并发送

## 代码结构

### 主要文件

- `ip_tunnel_handler.go`: IP层隧道处理器主文件
  - `IPTunnelHandler`: 处理器结构
  - `IPTunnel`: IP隧道结构
  - `TCPConnection`: TCP连接状态管理
  - IP数据包处理逻辑

- `ip_tunnel_helper.go`: 辅助函数
  - `forwardTCPFromRemote`: TCP数据转发
  - `sendIPPacket`: 发送IP数据包
  - `buildIPv4Packet`: 构建IPv4数据包
  - `buildTCPPacket`: 构建TCP数据包
  - `sendTCPRST`: 发送TCP RST响应

### 关键函数

```go
// 处理IP数据包
handleIPPacket() -> handleIPv4Packet() / handleIPv6Packet()
    ↓
根据协议类型分发
    ↓
handleTCPPacket() 或 handleUDPPacket()
```

## 使用说明

服务器现在默认使用真正的TUN层实现：

```bash
./utlsProxy \
  -listen 0.0.0.0:443 \
  -token your-token \
  -cert server.crt \
  -key server.key
```

客户端需要发送真正的IP数据包（通过TUN设备），而不是HTTP请求。

## 与之前实现的区别

### 之前（TCP代理）
- 接收HTTP请求
- 直接转发TCP数据
- 不支持UDP

### 现在（真正的TUN层）
- 接收IP数据包
- 解析IP包头
- 支持TCP和UDP
- 完整的网络层隧道

## 限制和改进空间

### 当前限制

1. **TCP状态机简化**
   - 序列号和确认号未完整实现
   - 未处理TCP窗口和流量控制
   - 超时重传未实现

2. **UDP处理简化**
   - UDP响应超时固定为5秒
   - 未维护UDP连接状态

3. **IP校验和**
   - IP和TCP校验和未计算（设为0）
   - 实际使用时可能需要计算

### 未来改进

1. **完整的TCP状态机**
   - 实现序列号管理
   - 实现流量控制
   - 实现超时重传

2. **性能优化**
   - 连接池复用
   - 零拷贝数据转发
   - 批量处理

3. **协议支持扩展**
   - ICMP支持
   - 其他IP层协议
