# API参考

<cite>
**本文档中引用的文件**
- [utlshotconnpool.go](file://utlsclient/utlshotconnpool.go)
- [utlsclient.go](file://utlsclient/utlsclient.go)
- [interfaces.go](file://utlsclient/interfaces.go)
- [connection_manager.go](file://utlsclient/connection_manager.go)
- [health_checker.go](file://utlsclient/health_checker.go)
- [ip_access_controller.go](file://utlsclient/ip_access_controller.go)
- [example_hotconnpool_usage.go](file://examples/utlsclient/example_hotconnpool_usage.go)
- [example_utlsclient_usage.go](file://examples/utlsclient/example_utlsclient_usage.go)
- [main.go](file://cmd/utlsclient/main.go)
</cite>

## 目录
1. [简介](#简介)
2. [UTLSHotConnPool API](#utlshotconnpool-api)
3. [UTLSClient API](#utlsclient-api)
4. [接口定义](#接口定义)
5. [配置选项](#配置选项)
6. [使用示例](#使用示例)
7. [性能特征](#性能特征)
8. [错误处理](#错误处理)
9. [最佳实践](#最佳实践)

## 简介

crawler-platform 提供了一套强大的uTLS连接池和HTTP客户端API，专为爬虫和自动化工具设计。该系统支持TLS指纹模拟、连接复用、健康检查和智能IP管理等功能。

## UTLSHotConnPool API

### NewUTLSHotConnPool

创建新的热连接池实例。

```go
func NewUTLSHotConnPool(config *PoolConfig) *UTLSHotConnPool
```

**参数：**
- `config` (*PoolConfig): 连接池配置，传入nil时使用默认配置

**返回值：**
- *UTLSHotConnPool: 新创建的连接池实例

**异常情况：**
- 无异常抛出，但配置验证失败会返回错误

**最佳实践：**
- 建议使用配置文件加载而非硬编码配置
- 在应用启动时创建连接池实例

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 内存占用：根据配置的连接池大小而定

### GetConnection

获取指定域名的热连接。

```go
func (p *UTLSHotConnPool) GetConnection(targetHost string) (*UTLSConnection, error)
```

**参数：**
- `targetHost` (string): 目标主机域名

**返回值：**
- *UTLSConnection: 可用的热连接对象
- error: 错误信息

**异常情况：**
- `ErrNoAvailableConnection`: 没有可用连接且无法创建新连接
- `ErrInvalidTargetHost`: 无效的目标主机名
- `ErrConnectionCreationFailed`: 连接创建失败

**使用流程：**
1. 首先尝试获取现有的空闲热连接
2. 如果没有可用连接，创建新的热连接
3. 返回可用的连接对象

**性能特征：**
- 时间复杂度：平均O(1)，最坏O(n)（需要创建新连接）
- 并发安全性：线程安全
- 连接复用率：高

### GetConnectionWithValidation

获取并验证指定路径可用性的热连接。

```go
func (p *UTLSHotConnPool) GetConnectionWithValidation(fullURL string) (*UTLSConnection, error)
```

**参数：**
- `fullURL` (string): 完整的URL（如"https://example.com/path"）

**返回值：**
- *UTLSConnection: 已验证可用的连接
- error: 错误信息

**异常情况：**
- `ErrInvalidURL`: URL格式无效
- `ErrUnsupportedProtocol`: 不支持的协议（非HTTPS）
- `ErrConnectionValidationFailed`: 连接验证失败

**使用场景：**
- 需要确保连接能访问特定路径
- API调用前的预验证
- 敏感操作前的连接健康检查

**性能特征：**
- 时间复杂度：O(1)（已有连接）或O(k)（创建新连接，k为验证次数）
- 并发安全性：线程安全
- 验证开销：额外的HTTP请求验证

### PutConnection

归还连接到连接池。

```go
func (p *UTLSHotConnPool) PutConnection(conn *UTLSConnection)
```

**参数：**
- `conn` (*UTLSConnection): 要归还的连接

**异常情况：**
- 无异常抛出，但nil连接会被静默忽略

**操作流程：**
1. 更新连接状态为空闲
2. 更新最后使用时间
3. 检查连接健康状态
4. 唤醒等待的goroutine

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 连接回收效率：高

### GetStats

获取连接池统计信息。

```go
func (p *UTLSHotConnPool) GetStats() PoolStats
```

**返回值：**
- PoolStats: 连接池统计信息

**统计指标：**
- TotalConnections: 总连接数
- ActiveConnections: 活跃连接数
- IdleConnections: 空闲连接数
- HealthyConnections: 健康连接数
- SuccessRate: 成功率
- AverageResponseTime: 平均响应时间
- ConnReuseRate: 连接复用率

**性能特征：**
- 时间复杂度：O(n)，其中n为活跃连接数
- 并发安全性：线程安全
- 实时性：统计数据可能有轻微延迟

### IsHealthy

检查连接池是否健康。

```go
func (p *UTLSHotConnPool) IsHealthy() bool
```

**返回值：**
- bool: 连接池是否健康

**健康标准：**
- 至少有一个健康连接
- 连接池配置有效
- 所有组件正常运行

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 实时性：实时检查

### Close

关闭连接池并清理所有连接。

```go
func (p *UTLSHotConnPool) Close() error
```

**返回值：**
- error: 错误信息

**清理过程：**
1. 停止所有后台维护任务
2. 等待正在执行的任务完成
3. 关闭所有连接
4. 清理资源

**异常情况：**
- 连接关闭过程中出现错误

**性能特征：**
- 时间复杂度：O(n)，n为连接数
- 并发安全性：线程安全
- 资源释放：完全释放

## UTLSClient API

### NewUTLSClient

创建新的UTLS客户端实例。

```go
func NewUTLSClient(conn *UTLSConnection) *UTLSClient
```

**参数：**
- `conn` (*UTLSConnection): 热连接对象

**返回值：**
- *UTLSClient: 新创建的客户端实例

**默认配置：**
- 超时时间：30秒
- 用户代理：来自连接的TLS指纹用户代理
- 最大重试次数：3次

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 内存占用：小量固定内存

### SetTimeout

设置请求超时时间。

```go
func (c *UTLSClient) SetTimeout(timeout time.Duration)
```

**参数：**
- `timeout` (time.Duration): 超时时间

**约束条件：**
- 超时时间不能为负值
- 建议设置合理的超时时间（通常10-60秒）

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 生效时机：下次请求开始时生效

### SetUserAgent

设置自定义用户代理。

```go
func (c *UTLSClient) SetUserAgent(userAgent string)
```

**参数：**
- `userAgent` (string): 自定义用户代理字符串

**约束条件：**
- 用户代理不能为空字符串
- 建议使用真实的浏览器用户代理格式

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 生效时机：下次请求开始时生效

### SetMaxRetries

设置最大重试次数。

```go
func (c *UTLSClient) SetMaxRetries(maxRetries int)
```

**参数：**
- `maxRetries` (int): 最大重试次数

**约束条件：**
- 重试次数不能为负数
- 建议设置合理的重试次数（通常1-5次）

**性能特征：**
- 时间复杂度：O(1)
- 并发安全性：线程安全
- 生效时机：下次请求开始时生效

### Do

执行HTTP请求。

```go
func (c *UTLSClient) Do(req *http.Request) (*http.Response, error)
```

**参数：**
- `req` (*http.Request): HTTP请求对象

**返回值：**
- *http.Response: HTTP响应对象
- error: 错误信息

**自动处理：**
- 自动设置User-Agent（如果未设置）
- 自动设置Accept-Language（如果未设置）
- 自动设置Host头（如果未设置）
- 自动重试失败的请求

**性能特征：**
- 时间复杂度：O(1)（连接层面），O(n)（网络层面，n为数据量）
- 并发安全性：线程安全
- 协议支持：HTTP/1.1和HTTP/2

### DoWithContext

带上下文的HTTP请求。

```go
func (c *UTLSClient) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error)
```

**参数：**
- `ctx` (context.Context): 上下文对象
- `req` (*http.Request): HTTP请求对象

**返回值：**
- *http.Response: HTTP响应对象
- error: 错误信息

**特性：**
- 支持取消操作
- 支持超时控制
- 支持传递请求上下文

**性能特征：**
- 时间复杂度：同Do方法
- 并发安全性：线程安全
- 上下文支持：完整支持

### Get

快捷方法：执行GET请求。

```go
func (c *UTLSClient) Get(url string) (*http.Response, error)
```

**参数：**
- `url` (string): 请求URL

**返回值：**
- *http.Response: HTTP响应对象
- error: 错误信息

**使用场景：**
- 简单的GET请求
- 快速原型开发

**性能特征：**
- 时间复杂度：同Do方法
- 并发安全性：线程安全
- 简便性：最高

### Post

快捷方法：执行POST请求。

```go
func (c *UTLSClient) Post(url string, contentType string, body io.Reader) (*http.Response, error)
```

**参数：**
- `url` (string): 请求URL
- `contentType` (string): 内容类型
- `body` (io.Reader): 请求体

**返回值：**
- *http.Response: HTTP响应对象
- error: 错误信息

**使用场景：**
- 表单提交
- API数据上传
- JSON数据传输

**性能特征：**
- 时间复杂度：同Do方法
- 并发安全性：线程安全
- 数据处理：支持各种数据格式

### Head

快捷方法：执行HEAD请求。

```go
func (c *UTLSClient) Head(url string) (*http.Response, error)
```

**参数：**
- `url` (string): 请求URL

**返回值：**
- *http.Response: HTTP响应对象
- error: 错误信息

**使用场景：**
- 资源存在性检查
- 内容长度获取
- 缓存验证

**性能特征：**
- 时间复杂度：同Do方法
- 并发安全性：线程安全
- 效率：最高（无响应体）

## 接口定义

### HotConnPool 接口

```go
type HotConnPool interface {
    GetConnection(targetHost string) (*UTLSConnection, error)
    GetConnectionWithValidation(fullURL string) (*UTLSConnection, error)
    PutConnection(conn *UTLSConnection)
    GetStats() PoolStats
    IsHealthy() bool
    Close() error
}
```

**用途：**
- 提供统一的连接池操作接口
- 支持依赖注入和测试
- 允许替换不同的连接池实现

### HTTPClient 接口

```go
type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
    DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error)
    Get(url string) (*http.Response, error)
    Post(url string, contentType string, body io.Reader) (*http.Response, error)
    Head(url string) (*http.Response, error)
}
```

**用途：**
- 提供统一的HTTP客户端接口
- 支持模拟和测试
- 允许替换不同的HTTP实现

### IPPoolProvider 接口

```go
type IPPoolProvider interface {
    GetIP() (string, error)
    GetIPsForDomain(domain string) []string
}
```

**用途：**
- IP池管理抽象
- 支持不同的IP获取策略
- 便于扩展和测试

### AccessController 接口

```go
type AccessController interface {
    IsIPAllowed(ip string) bool
    AddIP(ip string, isWhite bool)
    GetAllowedIPs() []string
    GetBlockedIPs() []string
    RemoveFromBlacklist(ip string)
    AddToWhitelist(ip string)
}
```

**用途：**
- IP访问控制抽象
- 支持黑白名单管理
- 便于扩展访问控制策略

## 配置选项

### PoolConfig 结构

| 配置项 | 类型 | 默认值 | 描述 |
|--------|------|--------|------|
| MaxConnections | int | 100 | 最大连接数 |
| MaxConnsPerHost | int | 10 | 每个主机最大连接数 |
| MaxIdleConns | int | 20 | 最大空闲连接数 |
| ConnTimeout | time.Duration | 30s | 连接超时时间 |
| IdleTimeout | time.Duration | 60s | 空闲超时时间 |
| MaxLifetime | time.Duration | 300s | 连接最大生命周期 |
| TestTimeout | time.Duration | 10s | 测试请求超时 |
| HealthCheckInterval | time.Duration | 30s | 健康检查间隔 |
| CleanupInterval | time.Duration | 60s | 清理间隔 |
| BlacklistCheckInterval | time.Duration | 300s | 黑名单检查间隔 |
| DNSUpdateInterval | time.Duration | 1800s | DNS更新间隔 |
| MaxRetries | int | 3 | 最大重试次数 |

### 配置加载

```go
// 从TOML文件加载配置
config, whitelist, blacklist, err := LoadConfigFromTOML("config.toml")

// 从合并配置文件加载
config, whitelist, blacklist, err := LoadMergedPoolConfig()

// 使用默认配置
config := DefaultPoolConfig()
```

## 使用示例

### 基本连接池使用

```go
// 创建连接池
config := DefaultPoolConfig()
pool := NewUTLSHotConnPool(config)

// 获取连接
conn, err := pool.GetConnection("example.com")
if err != nil {
    log.Fatal(err)
}
defer pool.PutConnection(conn)

// 使用连接
client := NewUTLSClient(conn)
resp, err := client.Get("https://example.com")
```

### 带路径验证的连接获取

```go
// 获取并验证特定路径的连接
conn, err := pool.GetConnectionWithValidation("https://api.example.com/v1/data")
if err != nil {
    log.Fatal(err)
}
defer pool.PutConnection(conn)

// 使用验证过的连接
client := NewUTLSClient(conn)
resp, err := client.Get("https://api.example.com/v1/data")
```

### HTTP客户端使用

```go
// 创建客户端
client := NewUTLSClient(conn)
client.SetTimeout(30 * time.Second)
client.SetUserAgent("MyCrawler/1.0")

// 执行请求
resp, err := client.Get("https://example.com")
if err != nil {
    log.Fatal(err)
}
defer resp.Body.Close()

// 处理响应
body, err := io.ReadAll(resp.Body)
```

### 高级配置示例

```go
// 自定义配置
config := &PoolConfig{
    MaxConnections:         200,
    MaxConnsPerHost:        20,
    ConnTimeout:            60 * time.Second,
    IdleTimeout:            120 * time.Second,
    MaxLifetime:            600 * time.Second,
    HealthCheckInterval:    60 * time.Second,
}

pool := NewUTLSHotConnPool(config)

// 设置依赖
pool.SetDependencies(fingerprintLib, ipPool, accessController, logger)
```

**章节来源**
- [example_hotconnpool_usage.go](file://examples/utlsclient/example_hotconnpool_usage.go#L1-L277)
- [example_utlsclient_usage.go](file://examples/utlsclient/example_utlsclient_usage.go#L1-L117)
- [main.go](file://cmd/utlsclient/main.go#L1-L113)

## 性能特征

### 连接池性能

| 特征 | 数值 | 说明 |
|------|------|------|
| 连接复用率 | 高 | 通过连接池实现连接复用 |
| 并发处理能力 | 高 | 支持大量并发请求 |
| 内存占用 | 中等 | 根据连接池大小动态调整 |
| CPU开销 | 低 | 连接复用减少CPU消耗 |
| 网络延迟 | 低 | 减少连接建立时间 |

### HTTP客户端性能

| 特征 | 数值 | 说明 |
|------|------|------|
| 请求处理速度 | 高 | 直接使用底层连接 |
| 协议支持 | HTTP/1.1, HTTP/2 | 自动协商最优协议 |
| 重试机制 | 智能 | 基于错误类型决定是否重试 |
| 超时控制 | 精确 | 支持微秒级精度 |
| 内存管理 | 优化 | 自动垃圾回收 |

### 性能优化建议

1. **合理配置连接池大小**
   - 根据并发需求调整MaxConnections
   - 控制MaxConnsPerHost避免单点过载

2. **启用健康检查**
   - 定期检查连接质量
   - 及时移除不健康的连接

3. **使用连接复用**
   - 及时归还连接到池中
   - 避免长时间持有连接

4. **监控性能指标**
   - 关注连接池统计信息
   - 监控成功率和响应时间

## 错误处理

### 常见错误类型

| 错误类型 | 错误码 | 描述 | 处理建议 |
|----------|--------|------|----------|
| ErrNoAvailableConnection | - | 没有可用连接 | 增加连接池大小或等待 |
| ErrConnectionCreationFailed | - | 连接创建失败 | 检查网络和配置 |
| ErrConnectionValidationFailed | - | 连接验证失败 | 检查目标服务器状态 |
| ErrInvalidURL | - | URL格式无效 | 验证URL格式 |
| ErrMaxRetriesExceeded | - | 达到最大重试次数 | 检查网络和服务器状态 |

### 错误处理最佳实践

1. **重试策略**
```go
for i := 0; i < client.maxRetries; i++ {
    resp, err := client.Do(req)
    if err == nil {
        return resp, nil
    }
    // 根据错误类型决定是否重试
    if !shouldRetry(err) {
        break
    }
    time.Sleep(retryDelay(i))
}
```

2. **连接池健康检查**
```go
if !pool.IsHealthy() {
    // 重新初始化连接池
    pool = NewUTLSHotConnPool(config)
}
```

3. **优雅降级**
```go
conn, err := pool.GetConnectionWithValidation(url)
if err != nil {
    // 使用备用连接池或本地缓存
    conn = fallbackPool.GetConnection(host)
}
```

### 错误恢复机制

1. **自动重试**
   - 对于临时性错误自动重试
   - 指数退避算法减少服务器压力

2. **连接重建**
   - 检测到连接故障时自动重建
   - 使用备用IP地址

3. **降级处理**
   - 主连接池不可用时使用备用方案
   - 本地缓存作为最终降级手段

## 最佳实践

### 连接池管理

1. **生命周期管理**
   ```go
   // 应用启动时创建
   pool := NewUTLSHotConnPool(config)
   
   // 应用关闭时清理
   defer pool.Close()
   ```

2. **连接归还**
   ```go
   conn, err := pool.GetConnection(host)
   if err != nil {
       return err
   }
   defer pool.PutConnection(conn)
   
   // 使用连接...
   ```

3. **健康监控**
   ```go
   ticker := time.NewTicker(5 * time.Minute)
   go func() {
       for range ticker.C {
           if !pool.IsHealthy() {
               // 记录日志并采取措施
               log.Warn("连接池不健康")
           }
       }
   }()
   ```

### HTTP客户端使用

1. **超时设置**
   ```go
   client := NewUTLSClient(conn)
   client.SetTimeout(30 * time.Second) // 根据需求调整
   ```

2. **用户代理设置**
   ```go
   client.SetUserAgent("MyApp/1.0 (+https://myapp.com)")
   ```

3. **请求头管理**
   ```go
   req, _ := http.NewRequest("GET", url, nil)
   req.Header.Set("Accept", "application/json")
   req.Header.Set("Authorization", "Bearer "+token)
   ```

### 性能优化

1. **连接池调优**
   - 根据并发量调整MaxConnections
   - 监控连接池使用率
   - 定期清理空闲连接

2. **请求优化**
   - 使用HTTP/2提高性能
   - 启用压缩减少传输量
   - 合理设置超时时间

3. **监控和告警**
   - 监控连接池统计信息
   - 设置性能阈值告警
   - 记录错误和异常情况

### 安全考虑

1. **TLS配置**
   - 使用最新的TLS版本
   - 验证服务器证书
   - 避免使用弱加密算法

2. **IP访问控制**
   - 启用IP白名单/黑名单
   - 定期更新访问控制列表
   - 监控可疑IP活动

3. **请求限制**
   - 设置合理的请求频率
   - 实施反爬虫策略
   - 使用User-Agent伪装

### 测试和调试

1. **单元测试**
   ```go
   func TestUTLSClient(t *testing.T) {
       conn := NewTestConnection("127.0.0.1", "localhost")
       client := NewUTLSClient(conn)
       
       // 测试各种场景
       resp, err := client.Get("https://localhost/test")
       assert.NoError(t, err)
       assert.Equal(t, 200, resp.StatusCode)
   }
   ```

2. **集成测试**
   ```go
   func TestIntegration(t *testing.T) {
       pool := NewUTLSHotConnPool(DefaultPoolConfig())
       defer pool.Close()
       
       // 测试完整流程
       conn, err := pool.GetConnectionWithValidation(testURL)
       assert.NoError(t, err)
       // ...
   }
   ```

3. **调试工具**
   ```go
   client.SetDebug(true) // 启用详细日志
   stats := pool.GetStats()
   log.Info("连接池统计: %+v", stats)
   ```

**章节来源**
- [utlshotconnpool.go](file://utlsclient/utlshotconnpool.go#L291-L580)
- [utlsclient.go](file://utlsclient/utlsclient.go#L46-L392)
- [connection_manager.go](file://utlsclient/connection_manager.go#L1-L218)
- [health_checker.go](file://utlsclient/health_checker.go#L1-L165)
- [ip_access_controller.go](file://utlsclient/ip_access_controller.go#L1-L184)