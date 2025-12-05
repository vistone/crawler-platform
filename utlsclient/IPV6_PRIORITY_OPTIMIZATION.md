# IPv6 地址池优先使用优化

## 问题描述

从日志观察到，很多请求显示 `本地IP=(系统默认)`，而不是使用配置的本地 IPv6 地址池。用户希望：
- 如果支持 IPv6 地址池，就尽可能使用 IPv6 地址去请求
- 避免使用 IPv4 地址去请求（当有 IPv6 地址池可用时）

## 优化方案

### 1. 重试机制

当目标是 IPv6 时，如果从本地 IP 池获取到的是 IPv4 或 `nil`，会重试最多 5 次，确保获取到 IPv6 地址：

```go
// 如果目标是 IPv6，优先获取 IPv6 本地地址（最多重试 5 次）
if targetIsIPv6 {
    maxRetries := 5
    for retry := 0; retry < maxRetries; retry++ {
        candidateIP := config.LocalIPPool.GetIP()
        if candidateIP == nil {
            // 地址池返回 nil，短暂等待后重试
            if retry < maxRetries-1 {
                time.Sleep(20 * time.Millisecond)
            }
            continue
        }
        // 检查是否是 IPv6 地址
        if candidateIP.To4() == nil {
            localIP = candidateIP
            break // 找到 IPv6 地址，停止重试
        }
        // 如果返回的是 IPv4，但目标是 IPv6，继续重试
        if retry < maxRetries-1 {
            time.Sleep(20 * time.Millisecond)
        }
    }
}
```

### 2. IPv4 目标处理

如果目标是 IPv4，但本地 IP 池返回的是 IPv6，则不能使用，会使用系统默认地址：

```go
// 如果目标是 IPv4，获取本地 IP
localIP = config.LocalIPPool.GetIP()
// 如果返回的是 IPv6，但目标是 IPv4，不能使用
if localIP != nil && localIP.To4() == nil {
    localIP = nil // IPv6 地址不能用于 IPv4 目标
}
```

## 配置检查

### 1. 确保 IPv6 子网配置正确

在 `config.toml` 中：

```toml
[LocalIPPool]
enable = true
# 从 ip addr show ipv6net 输出中提取子网前缀
ipv6_subnet_cidr = "2607:8700:5500:d197::/64"
target_ip_count = 100  # 确保有足够的 IPv6 地址
```

### 2. 检查地址池初始化

启动日志应该显示：

```
--- [IP 池初始化状态] ---
  [IPv4] 可用地址: ...
  [IPv6] 支持状态: 已启用
  [IPv6] 使用子网: 2607:8700:5500:d197::/64
  [IPv6] 绑定接口: ipv6net
--------------------------
```

### 3. 检查目标 IP 数量设置

确保 `target_ip_count` 已正确设置，并且地址池有足够的 IPv6 地址可用。

## 日志解读

### 成功使用本地 IPv6 地址

```
UTLS 连接获取成功: host=kh.google.com, 目标IP=2404:6800:4003:c01::5d, 本地IP=2607:8700:5500:d197::270, ...
```

### 未使用本地地址池（系统默认）

```
UTLS 连接获取成功: host=kh.google.com, 目标IP=2404:6800:4004:810::200e, 本地IP=(系统默认), ...
```

**可能原因**：
1. 地址池返回 `nil`（地址池未准备好或地址不足）
2. 地址池返回的是 IPv4，但目标是 IPv6（已优化，会重试）
3. 地址池配置不正确

## 故障排查

### 问题 1：地址池返回 nil

**检查**：
1. 查看启动日志，确认 IPv6 地址池已初始化
2. 检查 `target_ip_count` 是否已设置
3. 检查地址池是否有足够的活跃地址

**解决**：
- 增加 `target_ip_count` 的值
- 检查地址池的后台任务是否正常运行

### 问题 2：地址池返回 IPv4 但目标是 IPv6

**已优化**：代码会自动重试，直到获取到 IPv6 地址或达到最大重试次数。

### 问题 3：地址池初始化失败

**检查**：
- IPv6 子网 CIDR 是否正确
- 网络接口是否存在
- 是否有权限创建 IPv6 地址

## 性能影响

- **重试机制**：最多重试 5 次，每次等待 20ms，最多增加 100ms 延迟
- **影响范围**：仅影响 IPv6 目标的连接建立，IPv4 目标不受影响
- **优化效果**：确保 IPv6 目标优先使用本地 IPv6 地址池，提高 IP 轮换效果

## 相关代码

- `utlsclient/utlshotconnpool.go:245-300` - 本地 IP 获取和绑定逻辑
- `localippool/localippool.go:218-410` - IP 池 GetIP 实现
- `cmd/grpcserver/main.go:84-103` - 本地 IP 池初始化

