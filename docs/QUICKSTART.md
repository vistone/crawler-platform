# 快速开始指南

> **文档版本**: v0.0.15  
> **最后更新**: 2025-11-20  
> **适用人群**: 新用户、快速集成

## 目录

- [5分钟快速开始](#5分钟快速开始)
- [安装与配置](#安装与配置)
- [基础使用](#基础使用)
- [进阶使用](#进阶使用)
- [常见问题](#常见问题)

## 5分钟快速开始

### 前置要求

- Go 1.25+
- Linux/macOS/Windows系统

### 快速安装

```bash
# 克隆项目
git clone https://github.com/yourusername/crawler-platform.git
cd crawler-platform

# 安装依赖
go mod download

# 验证安装
go build ./...
```

### 第一个程序

创建 `main.go`:

```go
package main

import (
    "fmt"
    "net/http"
    "crawler-platform/utlsclient"
)

func main() {
    // 1. 创建热连接池
    pool := utlsclient.NewUTLSHotConnPool(nil)
    defer pool.Close()

    // 2. 获取连接
    conn, err := pool.GetConnection("kh.google.com")
    if err != nil {
        panic(err)
    }

    // 3. 创建HTTP客户端
    client := utlsclient.NewUTLSClient(conn)

    // 4. 发送请求
    req, _ := http.NewRequest("GET", "https://kh.google.com/rt/earth/PlanetoidMetadata", nil)
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    // 5. 归还连接
    pool.PutConnection(conn)

    fmt.Printf("状态码: %d\n", resp.StatusCode)
}
```

运行:

```bash
go run main.go
```

**预期输出**:
```
状态码: 200
```

🎉 恭喜! 你已经成功发送了第一个使用TLS指纹伪装的请求!

## 安装与配置

### 方式一: 作为Go模块引入

```bash
# 在你的项目中引入
go get crawler-platform
```

```go
import (
    "crawler-platform/utlsclient"
    "crawler-platform/config"
)
```

### 方式二: 直接使用源码

```bash
# 克隆到GOPATH或使用go mod
git clone https://github.com/yourusername/crawler-platform.git
cd crawler-platform
go mod download
```

### 配置文件 (可选)

创建 `config.toml`:

```toml
[pool]
max_connections = 100
max_conns_per_host = 10
max_idle_conns = 20
conn_timeout = 30
idle_timeout = 60
max_lifetime = 300
test_timeout = 10
health_check_interval = 30
cleanup_interval = 60
blacklist_check_interval = 300
dns_update_interval = 1800
max_retries = 3

[whitelist]
ips = []

[blacklist]
ips = []

[GoogleEarth]
host_name = "kh.google.com"
tm_host_name = "khms.google.com"
base_url = "https://kh.google.com"
tm_base_url = "https://khms.google.com"
```

**配置说明**:
- `max_connections`: 连接池最大连接数
- `max_conns_per_host`: 每个主机最大连接数
- `conn_timeout`: 连接超时时间(秒)
- 更多配置项见 [配置参考](docs/configuration/config-reference.md)

### 加载配置

```go
// 方式一: 使用默认配置
pool := utlsclient.NewUTLSHotConnPool(nil)

// 方式二: 自定义配置
config := &utlsclient.PoolConfig{
    MaxConnections:      200,
    MaxConnsPerHost:     20,
    ConnTimeout:         30 * time.Second,
    IdleTimeout:         60 * time.Second,
    HealthCheckInterval: 30 * time.Second,
}
pool := utlsclient.NewUTLSHotConnPool(config)

// 方式三: 从配置文件加载
poolConfig, _, _, err := utlsclient.LoadMergedPoolConfig()
if err != nil {
    panic(err)
}
pool := utlsclient.NewUTLSHotConnPool(poolConfig)
```

## 基础使用

### 模式一: 热连接池 (推荐)

适用于需要频繁访问同一域名的场景，性能提升3-6倍。

```go
package main

import (
    "fmt"
    "net/http"
    "crawler-platform/utlsclient"
)

func main() {
    // 创建连接池
    pool := utlsclient.NewUTLSHotConnPool(nil)
    defer pool.Close()

    // 获取连接
    conn, err := pool.GetConnection("example.com")
    if err != nil {
        panic(err)
    }

    // 创建客户端
    client := utlsclient.NewUTLSClient(conn)

    // 发送请求
    resp, err := client.Get("https://example.com/api/data")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    // 归还连接 (重要!)
    pool.PutConnection(conn)

    fmt.Printf("状态码: %d\n", resp.StatusCode)
}
```

**关键点**:
- ✅ 使用 `pool.GetConnection()` 获取连接
- ✅ 使用完毕后调用 `pool.PutConnection()` 归还
- ✅ 连接可以被复用，避免重复TLS握手

### 模式二: 批量请求

```go
func batchRequests(pool *utlsclient.UTLSHotConnPool, urls []string) {
    for _, url := range urls {
        // 获取连接
        conn, err := pool.GetConnection("example.com")
        if err != nil {
            fmt.Printf("获取连接失败: %v\n", err)
            continue
        }

        // 发送请求
        client := utlsclient.NewUTLSClient(conn)
        resp, err := client.Get(url)
        if err != nil {
            fmt.Printf("请求失败 %s: %v\n", url, err)
        } else {
            fmt.Printf("成功: %s - %d\n", url, resp.StatusCode)
            resp.Body.Close()
        }

        // 归还连接
        pool.PutConnection(conn)
    }
}
```

### 模式三: 并发请求

```go
func concurrentRequests(pool *utlsclient.UTLSHotConnPool, urls []string) {
    var wg sync.WaitGroup
    semaphore := make(chan struct{}, 10) // 限制并发数为10

    for _, url := range urls {
        wg.Add(1)
        go func(u string) {
            defer wg.Done()
            
            semaphore <- struct{}{}        // 获取信号量
            defer func() { <-semaphore }() // 释放信号量

            // 获取连接
            conn, err := pool.GetConnection("example.com")
            if err != nil {
                return
            }
            defer pool.PutConnection(conn)

            // 发送请求
            client := utlsclient.NewUTLSClient(conn)
            resp, err := client.Get(u)
            if err != nil {
                return
            }
            defer resp.Body.Close()

            fmt.Printf("完成: %s - %d\n", u, resp.StatusCode)
        }(url)
    }

    wg.Wait()
}
```

### HTTP方法示例

#### GET请求

```go
client := utlsclient.NewUTLSClient(conn)
resp, err := client.Get("https://example.com/api/data")
```

#### POST请求

```go
import "bytes"

data := []byte(`{"key": "value"}`)
resp, err := client.Post(
    "https://example.com/api/submit",
    "application/json",
    bytes.NewReader(data),
)
```

#### 自定义请求

```go
req, _ := http.NewRequest("PUT", "https://example.com/api/update", body)
req.Header.Set("Authorization", "Bearer token")
req.Header.Set("Content-Type", "application/json")

resp, err := client.Do(req)
```

## 进阶使用

### 自定义客户端配置

```go
client := utlsclient.NewUTLSClient(conn)

// 设置超时时间
client.SetTimeout(30 * time.Second)

// 设置User-Agent (可选，默认使用TLS指纹对应的UA)
client.SetUserAgent("MyApp/1.0")

// 设置最大重试次数
client.SetMaxRetries(3)

// 开启调试模式
client.SetDebug(true)
```

### 带验证的连接获取

适用于需要验证特定路径可达的场景：

```go
// 验证路径可用性
conn, err := pool.GetConnectionWithValidation("https://example.com/api/health")
if err != nil {
    panic(err)
}

// 此连接已经验证过 /api/health 路径可用
client := utlsclient.NewUTLSClient(conn)
resp, _ := client.Get("https://example.com/api/data")
```

### 使用Context控制超时

```go
import "context"

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

req, _ := http.NewRequest("GET", "https://example.com/api/data", nil)
req = req.WithContext(ctx)

resp, err := client.DoWithContext(ctx, req)
```

### 连接池统计信息

```go
stats := pool.GetStats()

fmt.Printf("总连接数: %d\n", stats.TotalConnections)
fmt.Printf("活跃连接: %d\n", stats.ActiveConnections)
fmt.Printf("空闲连接: %d\n", stats.IdleConnections)
fmt.Printf("健康连接: %d\n", stats.HealthyConnections)
fmt.Printf("白名单IP: %d\n", stats.WhitelistIPs)
fmt.Printf("黑名单IP: %d\n", stats.BlacklistIPs)
fmt.Printf("成功率: %.2f%%\n", stats.SuccessRate*100)
```

### IP池管理

```go
// 获取白名单IP
whitelist := pool.GetWhitelist()
fmt.Printf("白名单: %v\n", whitelist)

// 获取黑名单IP
blacklist := pool.GetBlacklist()
fmt.Printf("黑名单: %v\n", blacklist)

// 手动添加IP到白名单
pool.GetAccessController().AddToWhitelist("1.2.3.4")

// 从黑名单移除IP
pool.GetAccessController().RemoveFromBlacklist("5.6.7.8")
```

### Google Earth数据处理

```go
import (
    "crawler-platform/GoogleEarth"
    pb "crawler-platform/GoogleEarth/pb"
    "google.golang.org/protobuf/proto"
)

// 解析四叉树路径
path := "0123"
qtPath, _ := GoogleEarth.NewQuadtreePathFromString(path)

// 转换为四叉树编号
qtNum := qtPath.AsNumber()
fmt.Printf("四叉树编号: %d\n", qtNum)

// 解析Protobuf数据
nodeData := []byte{...} // 从网络获取的数据
qtNode := &pb.QuadtreeNode{}
if err := proto.Unmarshal(nodeData, qtNode); err != nil {
    panic(err)
}

fmt.Printf("节点Flags: %d\n", qtNode.GetFlags())
```

## 常见问题

### Q1: 如何查看使用了哪个TLS指纹?

```go
conn, _ := pool.GetConnection("example.com")
fingerprint := conn.Fingerprint()

fmt.Printf("指纹名称: %s\n", fingerprint.Name)
fmt.Printf("User-Agent: %s\n", fingerprint.UserAgent)
fmt.Printf("Accept-Language: %s\n", conn.AcceptLanguage())
```

### Q2: 连接池何时需要预热?

对于大规模请求场景，建议先预热连接池:

```go
// 预热阶段
ips := []string{"1.2.3.4", "5.6.7.8", ...}
for _, ip := range ips {
    conn, err := pool.GetConnectionToIP("https://example.com", ip)
    if err == nil {
        // 发送一个简单的健康检查请求
        client := utlsclient.NewUTLSClient(conn)
        client.Head("https://example.com/")
        pool.PutConnection(conn)
    }
}

// 预热后，连接已建立并验证，后续请求直接复用
```

### Q3: 如何处理请求失败?

```go
conn, _ := pool.GetConnection("example.com")
client := utlsclient.NewUTLSClient(conn)

// 设置重试次数
client.SetMaxRetries(3)

// 发送请求，自动重试
resp, err := client.Get("https://example.com/api/data")
if err != nil {
    // 请求失败，可能需要切换IP
    fmt.Printf("请求失败: %v\n", err)
    
    // 归还连接 (即使失败也要归还)
    pool.PutConnection(conn)
    
    // 尝试使用其他IP
    conn2, _ := pool.GetConnection("example.com")
    client2 := utlsclient.NewUTLSClient(conn2)
    resp, err = client2.Get("https://example.com/api/data")
}
```

### Q4: 为什么有些请求返回403?

可能原因:
1. **TLS指纹被识别**: 尝试更换不同的指纹
2. **IP被封禁**: 检查黑名单，尝试使用新IP
3. **请求头缺失**: 确保设置了必要的请求头

```go
// 检查黑名单
blacklist := pool.GetBlacklist()
if len(blacklist) > 0 {
    fmt.Printf("黑名单IP: %v\n", blacklist)
}

// 手动设置请求头
req, _ := http.NewRequest("GET", url, nil)
req.Header.Set("Referer", "https://example.com/")
req.Header.Set("Accept", "text/html,application/xhtml+xml")
```

### Q5: 连接池何时需要关闭?

应用程序退出前关闭连接池:

```go
func main() {
    pool := utlsclient.NewUTLSHotConnPool(nil)
    defer pool.Close() // 确保关闭

    // 应用程序逻辑
    runApp(pool)
}
```

### Q6: 如何优化性能?

**关键优化点**:

1. **使用连接池**: 比每次新建连接快3-6倍
2. **HTTP/2优先**: 单个连接可处理多个并发请求
3. **预热连接**: 提前建立连接，避免冷启动
4. **合理配置**: 根据负载调整连接池大小
5. **并发控制**: 使用信号量限制并发数

```go
// 高性能配置示例
config := &utlsclient.PoolConfig{
    MaxConnections:      500,
    MaxConnsPerHost:     30,
    MaxIdleConns:        100,
    ConnTimeout:         20 * time.Second,
    IdleTimeout:         600 * time.Second,
    HealthCheckInterval: 120 * time.Second,
}
```

### Q7: 如何调试问题?

```go
// 开启调试模式
client.SetDebug(true)

// 检查连接健康状态
if !pool.IsHealthy() {
    fmt.Println("连接池不健康")
}

// 查看详细统计
stats := pool.GetStats()
fmt.Printf("失败请求: %d\n", stats.FailedRequests)
fmt.Printf("成功率: %.2f%%\n", stats.SuccessRate*100)
```

## 下一步

- 📖 阅读 [系统架构文档](ARCHITECTURE.md) 了解内部实现
- 📖 查看 [API参考](docs/api/) 了解所有可用接口
- 📖 参考 [配置文档](docs/configuration/config-reference.md) 优化配置
- 📖 学习 [最佳实践](docs/development/best-practices.md) 提升代码质量
- 📊 查看 [性能测试报告](test/reports/热连接池性能测试报告.md) 了解性能指标

## 获取帮助

- 📝 查看 [FAQ](docs/operations/troubleshooting.md)
- 🐛 提交 [Issue](https://github.com/yourusername/crawler-platform/issues)
- 💬 加入讨论 [Discussions](https://github.com/yourusername/crawler-platform/discussions)

## 许可证

本项目采用 [许可证名称] 许可证。详见 [LICENSE](LICENSE) 文件。
