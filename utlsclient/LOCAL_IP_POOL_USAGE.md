# 本地 IP 地址池使用说明

## 功能概述

`utlsclient` 支持使用本地 IP 地址池作为源 IP 地址，在向远程服务器发起连接时，会自动从本地地址池中选择一个 IP 地址并绑定到连接。

## 适用场景

- **VPS 多 IP 环境**：VPS 配置了多个 IPv4 或 IPv6 地址
- **IPv6 地址池**：VPS 有大量 IPv6 地址（如 `/64` 子网）
- **IP 轮换**：需要轮换使用不同的源 IP 地址，避免被限流

## 配置步骤

### 1. 在 `config.toml` 中启用本地 IP 池

```toml
[LocalIPPool]
Enable = true
# 静态 IPv4 地址列表（可选，如果 VPS 有多个 IPv4 地址）
StaticIPv4s = ["1.2.3.4", "1.2.3.5"]
# IPv6 子网 CIDR（可选，如果 VPS 有 IPv6 子网）
IPv6SubnetCIDR = "2607:8700:5500:d197::/64"
# 目标 IP 数量（用于 IPv6 动态地址池）
TargetIPCount = 100
```

### 2. 查看 VPS 的 IPv6 地址

使用以下命令查看 VPS 上的 IPv6 地址：

```bash
ip addr show ipv6net
```

输出示例：
```
4: ipv6net@NONE: <POINTOPOINT,NOARP,UP,LOWER_UP> mtu 1480 qdisc noqueue state UNKNOWN
    link/sit 45.78.5.252 peer 45.32.66.87
    inet6 2607:8700:5500:d197::c63e/128 scope global
    inet6 2607:8700:5500:d197::cf6e/128 scope global
    ...
    inet6 2607:8700:5500:d197::2/64 scope global
```

从输出中提取子网信息：
- 如果看到 `/64` 子网（如 `2607:8700:5500:d197::2/64`），使用该子网的前缀：`2607:8700:5500:d197::/64`
- 如果只有 `/128` 地址，说明是单播地址，不能用于动态地址池

### 3. 配置 IPv6 子网

在 `config.toml` 中设置 `IPv6SubnetCIDR`：

```toml
[LocalIPPool]
Enable = true
IPv6SubnetCIDR = "2607:8700:5500:d197::/64"
TargetIPCount = 100  # 目标 IP 数量，根据实际需求调整
```

### 4. 配置静态 IPv4 地址（可选）

如果 VPS 有多个静态 IPv4 地址：

```toml
[LocalIPPool]
Enable = true
StaticIPv4s = ["45.78.5.252", "45.78.5.253"]
```

## 工作原理

1. **地址获取**：`utlsclient` 在建立连接时，会从 `LocalIPPool` 中获取一个本地 IP 地址
2. **类型匹配**：系统会自动匹配 IP 类型：
   - 如果目标是 IPv6，使用 IPv6 本地地址
   - 如果目标是 IPv4，使用 IPv4 本地地址
3. **地址绑定**：使用 `net.Dialer.LocalAddr` 将本地 IP 绑定到 TCP 连接
4. **自动轮换**：每次建立连接时，会从地址池中获取不同的 IP（如果可用）

## 代码实现

### 连接建立逻辑

在 `utlsclient/utlshotconnpool.go` 的 `establishConnection` 函数中：

```go
// 创建 Dialer，支持绑定本地 IP 地址
dialer := &net.Dialer{
    Timeout: config.ConnTimeout,
}

// 如果配置了本地 IP 池，从池中获取一个本地 IP 并绑定
if config.LocalIPPool != nil {
    localIP := config.LocalIPPool.GetIP()
    if localIP != nil {
        // 根据目标 IP 的类型选择对应的本地 IP
        targetIsIPv6 := strings.Contains(ip, ":")
        localIsIPv6 := localIP.To4() == nil

        // 只有当本地 IP 类型与目标 IP 类型匹配时才绑定
        if targetIsIPv6 == localIsIPv6 {
            dialer.LocalAddr = &net.TCPAddr{
                IP:   localIP,
                Port: 0, // 0 表示让系统自动分配端口
            }
        }
    }
}

tcpConn, err := dialer.Dial("tcp", address)
```

## 验证方法

### 1. 查看日志

启动服务后，查看日志中是否有以下信息：

```
已设置本地 IP 池到 UTLS 客户端，将使用本地地址池作为源 IP
```

### 2. 检查连接

在服务器端或使用网络抓包工具（如 `tcpdump`）检查连接是否使用了配置的本地 IP 地址。

### 3. 测试连接

使用以下命令测试连接：

```bash
# 查看当前连接使用的源 IP
ss -tnp | grep :443
```

## 注意事项

1. **IP 类型匹配**：系统会自动匹配 IPv4/IPv6 类型，不会混用
2. **地址可用性**：确保配置的 IP 地址在系统上可用（已配置到网络接口）
3. **权限要求**：某些系统可能需要特殊权限才能绑定本地 IP
4. **性能影响**：使用本地 IP 池不会显著影响性能，但会增加少量内存使用

## 故障排查

### 问题：连接失败

**可能原因**：
- 本地 IP 地址未正确配置到网络接口
- IP 地址类型不匹配（IPv4 vs IPv6）
- 权限不足

**解决方法**：
1. 检查 IP 地址是否在系统上存在：`ip addr show`
2. 检查日志中的错误信息
3. 尝试不使用本地 IP 池，看是否能正常连接

### 问题：未使用本地 IP

**可能原因**：
- `LocalIPPool.Enable` 未设置为 `true`
- `LocalIPPool` 未正确初始化
- 地址池为空

**解决方法**：
1. 检查 `config.toml` 中的配置
2. 查看启动日志，确认本地 IP 池是否已初始化
3. 检查 `localIPPool.GetIP()` 是否返回非 nil 值

## 相关配置

- `config.toml` - 主配置文件
- `cmd/grpcserver/config.go` - 配置结构定义
- `localippool/localippool.go` - 本地 IP 池实现
- `utlsclient/utlshotconnpool.go` - 连接建立逻辑

