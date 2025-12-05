# UTLS Client 连接池管理系统

## 目录概述

`utlsclient` 是一个基于 uTLS 的高性能 HTTP/HTTPS 连接池管理系统，提供自动连接预热、健康检查、IP 黑白名单管理等功能。系统采用主动式连接池管理策略，确保连接始终处于可用状态。

## 核心组件

### 1. Client (`utlsclient.go`)
核心客户端，是整个系统的入口点，负责协调各个组件的工作。

**主要功能：**
- 初始化和管理所有子组件
- 提供连接获取和释放接口
- 执行定期健康检查
- 管理客户端生命周期

### 2. ConnectionManager (`connection_manager.go`)
连接管理器，扮演"白名单"角色，只存储健康、可用的连接。

**主要功能：**
- 管理连接的添加、移除和查询
- 维护域名到 IP 的映射关系
- 提供连接快照功能
- 清理空闲超时的连接

**数据结构：**
- `connections`: IP -> Connection 映射
- `hostMapping`: Host -> []IP 映射

### 3. PoolManager (`pool_manager.go`)
主动连接池管理器，负责连接的预热和维护。

**主要功能：**
- 从远程 IP 池获取域名和 IP 列表
- 主动建立连接并验证
- 将验证成功的连接加入白名单
- 将验证失败的 IP 加入黑名单
- 定期维护连接池

### 4. Blacklist (`blacklist.go`)
黑名单管理器，临时屏蔽被拒绝的 IP 地址。

**主要功能：**
- 添加 IP 到黑名单
- 检查 IP 是否被屏蔽
- 自动清理过期的黑名单条目
- 支持懒删除机制

### 5. Validator (`validator.go`)
连接验证器，验证新建立的连接是否可用。

**主要功能：**
- 执行 HTTP 请求验证连接
- 区分不同类型的验证失败（403 vs 其他错误）
- 提取 Session ID（如果存在）
- 返回验证结果

### 6. UTLSConnection (`utlshotconnpool.go`)
uTLS 连接包装器，封装了 TLS 连接和 HTTP 请求功能。

**主要功能：**
- 支持 HTTP/1.1 和 HTTP/2
- 管理连接状态（健康、使用中）
- 自动设置 User-Agent 和 Cookie
- 统计请求和错误计数

### 7. 指纹库 (`utlsfingerprint.go`)
TLS 指纹库，提供多种浏览器指纹配置。

**主要功能：**
- 管理多种浏览器指纹配置
- 随机选择指纹
- 生成随机 Accept-Language 头部
- 支持按浏览器、平台筛选指纹

### 8. Whitelist (`whitelist.go`)
白名单管理器（当前未在核心流程中使用，保留用于扩展）。

## 工作流程

### 1. 初始化阶段

```go
// 创建配置
config := &utlsclient.PoolConfig{
    MaxConnsPerHost:       10,
    PreWarmInterval:       5 * time.Minute,
    MaxConcurrentPreWarms: 20,
    ConnTimeout:           10 * time.Second,
    HealthCheckInterval:   5 * time.Minute,
    IPBlacklistTimeout:    15 * time.Minute,
    HotConnectPath:        "/",
    HotConnectMethod:      "GET",
}

// 创建客户端
client, err := utlsclient.NewClient(config, remotePool)
```

**初始化步骤：**
1. 创建 `Blacklist` 实例（黑名单管理器）
2. 创建 `ConnectionManager` 实例（连接管理器/白名单）
3. 创建 `ConfigurableValidator` 实例（验证器）
4. 创建 `PoolManager` 实例（连接池管理器）
5. 组装 `Client` 实例

### 2. 启动阶段

```go
client.Start()
```

**启动步骤：**
1. 启动 `PoolManager` 的后台维护循环
2. 启动 `Client` 的健康检查循环
3. 立即执行一次连接池维护

### 3. 连接预热流程

```
┌─────────────────┐
│  PoolManager    │
│  maintenanceLoop│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  maintainPool   │
│  获取所有域名IP  │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │ 遍历IP  │
    └───┬────┘
        │
        ├─→ 已在白名单？ ──→ 跳过
        │
        ├─→ 在黑名单？ ──→ 跳过
        │
        └─→ 需要预热 ──→ 并发预热（限制并发数）
                        │
                        ▼
                ┌──────────────┐
                │建立TCP连接    │
                │TLS握手       │
                │随机选择指纹  │
                └──────┬───────┘
                       │
                       ▼
                ┌──────────────┐
                │验证连接      │
                │发送HTTP请求  │
                └──────┬───────┘
                       │
        ┌──────────────┼──────────────┐
        │              │              │
    验证成功        403错误        其他错误
        │              │              │
        ▼              ▼              ▼
   加入白名单      加入黑名单      关闭连接
```

**预热详细步骤：**

1. **获取 IP 列表**
   - `PoolManager` 从 `RemoteIPPool` 获取所有域名和 IP 的映射

2. **筛选需要预热的 IP**
   - 跳过已在白名单（ConnectionManager）中的 IP
   - 跳过在黑名单中的 IP
   - 只处理既不在白名单也不在黑名单的 IP

3. **并发建立连接**
   - 使用信号量限制并发数（`MaxConcurrentPreWarms`）
   - 为每个 IP 启动 goroutine 进行预热

4. **建立连接**
   - 建立 TCP 连接
   - 随机选择 TLS 指纹
   - 执行 TLS 握手
   - 创建 `UTLSConnection` 实例

5. **验证连接**
   - 使用 `Validator` 发送 HTTP 请求验证连接
   - 根据响应状态码判断：
     - `200 OK`: 验证成功，提取 Session ID（如果有）
     - `403 Forbidden`: IP 被拒绝，加入黑名单
     - 其他状态码: 验证失败，关闭连接但不加入黑名单

6. **加入白名单**
   - 验证成功的连接加入 `ConnectionManager`
   - 建立域名到 IP 的映射关系

### 4. 连接使用流程

```
┌──────────────┐
│  用户请求     │
└──────┬───────┘
       │
       ▼
┌─────────────────┐
│GetConnectionFor │
│Host(host)       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ConnectionManager│
│GetConnections   │
│ForHost          │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │遍历连接 │
    └───┬────┘
        │
        ├─→ TryAcquire() ──→ 成功 ──→ 返回连接
        │
        └─→ 失败 ──→ 下一个连接
```

**使用详细步骤：**

1. **获取连接**
   ```go
   conn, err := client.GetConnectionForHost("example.com")
   ```

2. **查找可用连接**
   - `ConnectionManager` 根据域名查找所有相关连接
   - 遍历连接，尝试获取（`TryAcquire`）
   - 返回第一个成功获取的连接

3. **执行请求**
   ```go
   req, _ := http.NewRequest("GET", "https://example.com/api", nil)
   resp, err := conn.RoundTrip(req)
   ```

4. **释放连接**
   ```go
   client.ReleaseConnection(conn)
   ```

**连接释放逻辑：**
- 如果连接不健康：从白名单移除并关闭连接
- 如果连接健康：标记为未使用，放回连接池

### 5. 健康检查流程

```
┌─────────────────┐
│ maintenanceLoop │
│ 定时触发         │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  healthCheck    │
│  获取所有连接    │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │并发检查 │
    │(限制并发)│
    └───┬────┘
        │
        ├─→ 无法获取 ──→ 跳过（正在使用）
        │
        └─→ 获取成功 ──→ 发送HEAD请求
                        │
        ┌───────────────┼───────────────┐
        │               │               │
    网络错误         403错误         其他4xx/5xx
        │               │               │
        ▼               ▼               ▼
    标记不健康      加入黑名单      标记不健康
    移除连接        移除连接        移除连接
```

**健康检查详细步骤：**

1. **定时触发**
   - 根据 `HealthCheckInterval` 配置定期执行
   - 默认 5 分钟

2. **获取连接列表**
   - 从 `ConnectionManager` 获取所有连接的快照

3. **并发检查**
   - 使用信号量限制并发数（默认最大 10）
   - 为每个连接启动 goroutine

4. **执行检查**
   - 尝试获取连接（`TryAcquire`）
   - 发送 HEAD 请求到根路径
   - 根据响应判断连接状态

5. **处理结果**
   - **网络错误**: 标记不健康，移除连接
   - **403 Forbidden**: 加入黑名单，标记不健康，移除连接
   - **其他 4xx/5xx**: 标记不健康，移除连接（不加入黑名单）
   - **200 OK**: 连接健康，释放回连接池

6. **清理黑名单**
   - 定期清理过期的黑名单条目

### 6. 连接生命周期

```
创建 → 验证 → 加入白名单 → 使用 → 健康检查 → 移除/继续使用
  │      │        │         │        │
  │      │        │         │        ├─→ 健康 ──→ 继续使用
  │      │        │         │        │
  │      │        │         │        └─→ 不健康 ──→ 移除
  │      │        │         │
  │      │        │         └─→ 使用中出错 ──→ 标记不健康
  │      │        │
  │      │        └─→ 空闲超时 ──→ 清理
  │      │
  │      └─→ 验证失败 ──→ 关闭连接
  │          │
  │          ├─→ 403 ──→ 加入黑名单
  │          │
  │          └─→ 其他错误 ──→ 不加入黑名单
  │
  └─→ 建立失败 ──→ 返回错误
```

## 架构设计

### 组件关系图

```
                    ┌─────────────┐
                    │   Client    │
                    │  (核心入口)  │
                    └──────┬──────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│Connection    │   │ PoolManager  │   │  Blacklist   │
│Manager       │◄──┤              │───┤              │
│(白名单)       │   │  (连接预热)   │   │  (黑名单)     │
└──────┬───────┘   └──────┬───────┘   └──────────────┘
       │                  │
       │                  │
       ▼                  ▼
┌──────────────┐   ┌──────────────┐
│UTLSConnection│   │  Validator   │
│(连接包装)     │   │  (验证器)     │
└──────────────┘   └──────────────┘
```

### 数据流

```
RemoteIPPool
    │
    │ GetAllDomainIPs()
    ▼
PoolManager
    │
    │ 筛选IP（排除白名单和黑名单）
    ▼
establishConnection()
    │
    │ 建立TCP + TLS连接
    │ 随机选择指纹
    ▼
Validator.Validate()
    │
    │ 发送HTTP请求验证
    ▼
    ├─→ 成功 ──→ ConnectionManager.AddConnection()
    │
    └─→ 失败 ──→ Blacklist.Add() (如果是403)
```

### 并发安全

所有组件都采用适当的锁机制保证并发安全：

- **ConnectionManager**: 使用 `sync.RWMutex` 保护连接映射
- **Blacklist**: 使用 `sync.RWMutex` 保护黑名单映射
- **UTLSConnection**: 使用 `sync.Mutex` 保护连接状态
- **PoolManager**: 使用 `sync.Once` 确保 Stop 只执行一次
- **Client**: 使用 `sync.Mutex` 保护运行状态

## 配置说明

### PoolConfig 配置项

```go
type PoolConfig struct {
    // 每个主机最大连接数
    MaxConnsPerHost int
    
    // 连接池预热间隔
    PreWarmInterval time.Duration
    
    // 最大并发预热数
    MaxConcurrentPreWarms int
    
    // 连接超时时间
    ConnTimeout time.Duration
    
    // 空闲连接超时时间
    IdleTimeout time.Duration
    
    // 连接最大生存时间
    MaxConnLifetime time.Duration
    
    // 健康检查间隔
    HealthCheckInterval time.Duration
    
    // IP黑名单超时时间
    IPBlacklistTimeout time.Duration
    
    // 验证请求路径
    HotConnectPath string
    
    // 验证请求方法
    HotConnectMethod string
    
    // 验证请求体
    HotConnectBody string
}
```

## 使用示例

### 基本使用

```go
package main

import (
    "fmt"
    "net/http"
    "time"
    
    "crawler-platform/utlsclient"
)

func main() {
    // 1. 创建配置
    config := &utlsclient.PoolConfig{
        MaxConnsPerHost:       10,
        PreWarmInterval:       5 * time.Minute,
        MaxConcurrentPreWarms: 20,
        ConnTimeout:           10 * time.Second,
        HealthCheckInterval:   5 * time.Minute,
        IPBlacklistTimeout:    15 * time.Minute,
        HotConnectPath:        "/",
        HotConnectMethod:      "GET",
    }
    
    // 2. 实现 RemoteIPPool 接口
    remotePool := &MyRemoteIPPool{}
    
    // 3. 创建客户端
    client, err := utlsclient.NewClient(config, remotePool)
    if err != nil {
        panic(err)
    }
    
    // 4. 启动客户端
    client.Start()
    defer client.Stop()
    
    // 5. 等待连接预热（可选）
    time.Sleep(10 * time.Second)
    
    // 6. 使用连接
    conn, err := client.GetConnectionForHost("example.com")
    if err != nil {
        panic(err)
    }
    defer client.ReleaseConnection(conn)
    
    // 7. 发送请求
    req, _ := http.NewRequest("GET", "https://example.com/api", nil)
    resp, err := conn.RoundTrip(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    
    fmt.Printf("Status: %d\n", resp.StatusCode)
}

// 实现 RemoteIPPool 接口
type MyRemoteIPPool struct{}

// GetAllDomainIPs 从 ./data/domain_ips/ 目录加载域名-IP 池，并按域名聚合返回。
// 实际项目中推荐直接复用 DomainMonitor / DomainIPPoolAdapter 提供的内存结构，
// 这里给出一个简化示例，说明期望的返回格式。
func (p *MyRemoteIPPool) GetAllDomainIPs() map[string][]string {
    // 伪代码示例：
    // 1. 遍历 ./data/domain_ips/ 目录下的文件（例如 kh.google.com.json）
    // 2. 解析 JSON，提取该域名对应的所有 IP
    // 3. 填充到 result[domain] = []string{ip1, ip2, ...}
    //
    // 真实实现中应避免在每次调用时都读文件，
    // 而是由 DomainMonitor 周期性更新内存中的域名-IP 映射，
    // RemoteIPPool 只需返回这份内存快照即可。

    return map[string][]string{
        // 示例：kh.google.com 在 ./data/domain_ips/ 中的 IP 池
        "kh.google.com": {"1.2.3.4", "5.6.7.8"},
    }
}
```

## 关键特性

1. **主动连接预热**: 系统主动建立和验证连接，确保连接池始终有可用连接
2. **智能 IP 管理**: 自动区分可用和不可用 IP，使用黑白名单机制
3. **健康检查**: 定期检查连接健康状态，自动移除不健康连接
4. **TLS 指纹伪装**: 使用 uTLS 库模拟真实浏览器指纹
5. **并发安全**: 所有组件都经过并发安全设计
6. **资源管理**: 确保所有连接和资源正确释放
7. **错误处理**: 完善的错误处理和日志记录

## 注意事项

1. **连接释放**: 使用完连接后必须调用 `ReleaseConnection()` 释放
2. **响应体关闭**: 使用 `RoundTrip()` 返回的响应后，必须关闭响应体
3. **配置调优**: 根据实际场景调整并发数和超时时间
4. **资源清理**: 程序退出前调用 `Stop()` 确保资源正确释放

## 文件说明

- `utlsclient.go`: 核心客户端，系统入口
- `connection_manager.go`: 连接管理器（白名单）
- `pool_manager.go`: 连接池管理器（主动预热）
- `blacklist.go`: 黑名单管理器
- `validator.go`: 连接验证器
- `utlshotconnpool.go`: TLS 连接包装和连接建立
- `utlsfingerprint.go`: TLS 指纹库
- `whitelist.go`: 白名单管理器（保留用于扩展）

## 扩展点

1. **自定义验证器**: 实现 `Validator` 接口，自定义验证逻辑
2. **自定义 IP 池**: 实现 `RemoteIPPool` 接口，提供 IP 来源
3. **指纹扩展**: 在 `utlsfingerprint.go` 中添加新的指纹配置
4. **监控集成**: 在关键点添加监控指标收集

