# 配置参考文档

> **文档版本**: v0.0.15  
> **最后更新**: 2025-11-20  
> **相关代码**: `config/config.go`, `utlsclient/utlshotconnpool.go`

## 目录

- [配置文件组织](#配置文件组织)
- [连接池配置](#连接池配置)
- [黑白名单配置](#黑白名单配置)
- [Google Earth配置](#google-earth配置)
- [配置场景示例](#配置场景示例)
- [配置最佳实践](#配置最佳实践)

## 配置文件组织

### 配置文件位置

系统支持两个位置的配置文件,按优先级从高到低:

1. **项目根目录**: `config.toml` (优先级高)
2. **config目录**: `config/config.toml` (优先级低,默认配置)

**配置合并策略**:
- 优先加载 `config/config.toml` 作为默认值
- 再加载根目录 `config.toml` 覆盖默认配置
- 任一文件不存在时跳过

### 配置文件格式

使用 [TOML](https://toml.io/cn/) 格式:

```toml
# 这是注释
[section]
key = "value"
number = 123
boolean = true
array = ["item1", "item2"]
```

### 加载配置

#### 方式一: 使用默认配置

```go
pool := utlsclient.NewUTLSHotConnPool(nil)
```

#### 方式二: 加载配置文件

```go
// 加载合并后的配置
poolConfig, whitelist, blacklist, err := utlsclient.LoadMergedPoolConfig()
if err != nil {
    log.Fatal(err)
}

pool := utlsclient.NewUTLSHotConnPool(poolConfig)

// 应用黑白名单
for _, ip := range whitelist {
    pool.GetAccessController().AddToWhitelist(ip)
}
for _, ip := range blacklist {
    pool.GetAccessController().AddIP(ip, false)
}
```

#### 方式三: 自定义配置

```go
config := &utlsclient.PoolConfig{
    MaxConnections:      200,
    MaxConnsPerHost:     20,
    ConnTimeout:         30 * time.Second,
    HealthCheckInterval: 60 * time.Second,
}

pool := utlsclient.NewUTLSHotConnPool(config)
```

## 连接池配置

### [pool] 配置段

连接池核心配置,控制连接池行为和性能。

#### max_connections

**最大连接数**

- **类型**: `int`
- **默认值**: `100`
- **取值范围**: `1-10000`
- **说明**: 连接池允许的最大并发连接数,超过此数量的连接请求将等待
- **影响**: 
  - 内存占用: 每个连接约占用100KB-1MB内存
  - 系统资源: 操作系统文件描述符限制
  - 并发能力: 值越大,并发处理能力越强
- **调优建议**:
  - 开发环境: 20-50
  - 测试环境: 50-100
  - 生产环境: 200-500
  - 高性能场景: 500-1000

```toml
max_connections = 100
```

#### max_conns_per_host

**每主机最大连接数**

- **类型**: `int`
- **默认值**: `10`
- **取值范围**: `1-100`
- **说明**: 单个目标主机允许的最大连接数
- **影响**:
  - 服务器压力: 避免对单个服务器造成过大压力
  - 连接复用: HTTP/2下,少量连接即可高效复用
  - 负载均衡: 限制单主机连接,促进多主机负载均衡
- **调优建议**:
  - HTTP/1.1: 5-10 (每连接只能串行处理请求)
  - HTTP/2: 2-5 (单连接可并发处理多个请求)
  - 高负载: 15-30

```toml
max_conns_per_host = 10
```

#### max_idle_conns

**最大空闲连接数**

- **类型**: `int`
- **默认值**: `20`
- **取值范围**: `0-max_connections`
- **说明**: 池中保持的空闲连接数量上限
- **影响**:
  - 连接复用: 值越大,连接复用机会越多
  - 内存占用: 空闲连接也占用内存
  - 响应速度: 更多空闲连接意味着更快的响应
- **调优建议**:
  - 通常设置为 `max_connections` 的 20%-30%
  - 高频访问场景: 30%-50%
  - 低频访问场景: 10%-20%

```toml
max_idle_conns = 20
```

#### conn_timeout

**连接超时**

- **类型**: `int` (秒)
- **默认值**: `30`
- **取值范围**: `5-300`
- **说明**: 建立TLS连接的最大等待时间
- **影响**:
  - 故障检测: 超时越短,故障检测越快
  - 网络适应: 网络差时需要更长超时
  - 用户体验: 超时太短可能导致误判
- **调优建议**:
  - 内网/高速网络: 10-15秒
  - 外网/正常网络: 20-30秒
  - 跨国/慢速网络: 60-120秒

```toml
conn_timeout = 30
```

#### idle_timeout

**空闲超时**

- **类型**: `int` (秒)
- **默认值**: `60`
- **取值范围**: `10-600`
- **说明**: 连接空闲多久后被视为过期并关闭
- **影响**:
  - 资源释放: 超时越短,资源释放越快
  - 连接复用: 超时越长,连接复用机会越多
  - 服务器策略: 需匹配服务器的keepalive时间
- **调优建议**:
  - 短期任务: 30-60秒
  - 长期运行: 300-600秒
  - 与服务器keepalive配合: 略小于服务器超时

```toml
idle_timeout = 60
```

#### max_lifetime

**最大生命周期**

- **类型**: `int` (秒)
- **默认值**: `300` (5分钟)
- **取值范围**: `60-3600`
- **说明**: 连接从创建到强制关闭的最长时间
- **影响**:
  - 连接泄漏: 防止长时间存在的异常连接
  - 资源刷新: 定期刷新连接,获取新IP
  - 服务器策略: 适应服务器的连接限制
- **调优建议**:
  - 开发/测试: 120-300秒 (快速刷新)
  - 生产环境: 600-1800秒 (30分钟)
  - 稳定场景: 1800-3600秒 (1小时)

```toml
max_lifetime = 300
```

#### test_timeout

**测试超时**

- **类型**: `int` (秒)
- **默认值**: `10`
- **取值范围**: `3-60`
- **说明**: 连接健康检查请求的超时时间
- **影响**:
  - 检测速度: 超时越短,检测越快
  - 准确性: 超时太短可能误判健康连接
  - 系统开销: 影响健康检查的资源消耗
- **调优建议**:
  - 快速检查: 5-10秒
  - 严格检查: 15-30秒
  - 网络差: 30-60秒

```toml
test_timeout = 10
```

#### health_check_interval

**健康检查间隔**

- **类型**: `int` (秒)
- **默认值**: `30`
- **取值范围**: `10-600`
- **说明**: 两次主动健康检查之间的间隔
- **影响**:
  - 故障检测: 间隔越短,故障发现越快
  - 系统开销: 频繁检查增加系统负担
  - 网络开销: 每次检查都会发送请求
- **调优建议**:
  - 高可用场景: 15-30秒
  - 正常场景: 30-60秒
  - 低负载场景: 60-300秒

```toml
health_check_interval = 30
```

#### cleanup_interval

**清理间隔**

- **类型**: `int` (秒)
- **默认值**: `60`
- **取值范围**: `30-600`
- **说明**: 清理过期和不健康连接的周期
- **影响**:
  - 资源回收: 影响资源释放的及时性
  - 系统开销: 频繁清理增加CPU占用
  - 内存占用: 清理不及时会增加内存占用
- **调优建议**:
  - 通常设置为 `idle_timeout` 的 50%-100%
  - 资源紧张: 30-45秒
  - 资源充足: 60-120秒

```toml
cleanup_interval = 60
```

#### blacklist_check_interval

**黑名单检查间隔**

- **类型**: `int` (秒)
- **默认值**: `300` (5分钟)
- **取值范围**: `60-3600`
- **说明**: 重试黑名单IP的时间间隔
- **影响**:
  - IP恢复: 影响被封IP的恢复速度
  - 网络开销: 频繁重试增加无效请求
  - 封禁风险: 过于频繁可能加重封禁
- **调优建议**:
  - 临时问题(网络抖动): 180-300秒
  - 一般问题: 300-600秒
  - 持久问题(被封): 1800-3600秒

```toml
blacklist_check_interval = 300
```

#### dns_update_interval

**DNS更新间隔**

- **类型**: `int` (秒)
- **默认值**: `1800` (30分钟)
- **取值范围**: `300-7200`
- **说明**: 重新解析域名获取新IP的周期
- **影响**:
  - IP池刷新: 影响IP池的更新频率
  - DNS负载: 频繁查询增加DNS服务器压力
  - IP多样性: 更新越频繁,IP越多样
- **调优建议**:
  - 根据DNS TTL调整
  - IP快速变化: 600-1800秒
  - IP稳定: 1800-3600秒
  - CDN场景: 300-900秒

```toml
dns_update_interval = 1800
```

#### max_retries

**最大重试次数**

- **类型**: `int`
- **默认值**: `3`
- **取值范围**: `0-10`
- **说明**: 连接失败时的最大重试次数
- **影响**:
  - 容错能力: 重试次数越多,容错能力越强
  - 响应时间: 重试会增加总体响应时间
  - 资源消耗: 重试会消耗额外资源
- **调优建议**:
  - 网络稳定: 2-3次
  - 网络不稳定: 5-8次
  - 高可用场景: 3-5次
  - 快速失败场景: 0-1次

```toml
max_retries = 3
```

## 黑白名单配置

### [whitelist] 配置段

IP白名单配置,包含已验证可用的IP地址。

```toml
[whitelist]
ips = ["1.1.1.1", "8.8.8.8", "2001:4860:4860::8888"]
```

**字段说明**:

- **ips**: `string[]` - IP地址列表,支持IPv4和IPv6

**工作机制**:
1. 白名单中的IP优先使用
2. 连接验证成功后自动加入白名单
3. 白名单IP不受黑名单检查影响

**使用场景**:
- 预先配置已知可用IP
- 固定使用特定IP池
- 绕过黑名单检查机制

### [blacklist] 配置段

IP黑名单配置,包含已知不可用或被封禁的IP地址。

```toml
[blacklist]
ips = ["192.168.1.1", "10.0.0.1"]
```

**字段说明**:

- **ips**: `string[]` - IP地址列表,支持IPv4和IPv6

**工作机制**:
1. 黑名单中的IP不会被使用
2. 连接验证失败后自动加入黑名单
3. 定期重试黑名单IP,验证恢复

**使用场景**:
- 预先排除已知问题IP
- 避免使用内网IP
- 手动屏蔽特定IP

**注意事项**:
- 黑名单会定期重试(由 `blacklist_check_interval` 控制)
- 如果IP恢复,会自动从黑名单移除并加入白名单
- 白名单优先级高于黑名单

## Google Earth配置

### [GoogleEarth] 配置段

Google Earth服务端点配置。

```toml
[GoogleEarth]
host_name = "kh.google.com"
tm_host_name = "khms.google.com"
base_url = "https://kh.google.com"
tm_base_url = "https://khms.google.com"
auth_endpoint = "/rt/earth/PlanetoidMetadata"
dbroot_endpoint = "/dbRoot.v5"
```

**字段说明**:

- **host_name**: 主服务器域名
- **tm_host_name**: 瓦片地图服务器域名
- **base_url**: 主服务器基础URL
- **tm_base_url**: 瓦片地图服务器基础URL
- **auth_endpoint**: 认证端点路径
- **dbroot_endpoint**: 数据库根端点路径

**使用示例**:

```go
import "crawler-platform/config"

var cfg config.Config
err := config.LoadMergedInto(&cfg)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Google Earth主机: %s\n", cfg.GoogleEarth.HostName)
fmt.Printf("基础URL: %s\n", cfg.GoogleEarth.BaseURL)
```

## 配置场景示例

### 开发环境配置

适用于本地开发和调试,优先考虑调试便利性。

```toml
[pool]
max_connections = 20
max_conns_per_host = 5
max_idle_conns = 5
conn_timeout = 15
idle_timeout = 30
max_lifetime = 120
test_timeout = 5
health_check_interval = 60
cleanup_interval = 60
blacklist_check_interval = 300
dns_update_interval = 1800
max_retries = 2

[whitelist]
ips = []

[blacklist]
ips = ["192.168.0.0", "10.0.0.0"] # 排除内网IP
```

**特点**:
- 连接数少,减少资源占用
- 超时时间短,快速失败
- 重试次数少,快速发现问题

### 测试环境配置

适用于功能测试和集成测试,平衡性能和稳定性。

```toml
[pool]
max_connections = 50
max_conns_per_host = 10
max_idle_conns = 15
conn_timeout = 30
idle_timeout = 60
max_lifetime = 300
test_timeout = 10
health_check_interval = 30
cleanup_interval = 60
blacklist_check_interval = 300
dns_update_interval = 1800
max_retries = 3

[GoogleEarth]
host_name = "kh.google.com"
base_url = "https://kh.google.com"
```

**特点**:
- 中等连接数,模拟真实负载
- 标准超时配置
- 完整的健康检查机制

### 生产环境配置

适用于生产部署,优先考虑性能和可靠性。

```toml
[pool]
max_connections = 200
max_conns_per_host = 20
max_idle_conns = 50
conn_timeout = 30
idle_timeout = 300
max_lifetime = 1800
test_timeout = 15
health_check_interval = 60
cleanup_interval = 120
blacklist_check_interval = 600
dns_update_interval = 3600
max_retries = 5

[whitelist]
ips = [
    # 预先配置已验证的高质量IP
    "142.250.185.78",
    "142.251.42.174"
]
```

**特点**:
- 大连接池,支持高并发
- 较长超时,保证稳定性
- 频繁健康检查,快速故障检测
- 预配置白名单,减少冷启动

### 高性能配置

适用于大规模爬取场景,最大化吞吐量。

```toml
[pool]
max_connections = 500
max_conns_per_host = 30
max_idle_conns = 100
conn_timeout = 20
idle_timeout = 600
max_lifetime = 3600
test_timeout = 10
health_check_interval = 120
cleanup_interval = 300
blacklist_check_interval = 1800
dns_update_interval = 7200
max_retries = 3
```

**特点**:
- 超大连接池
- 较短连接超时,快速失败
- 较长空闲超时,最大化复用
- 降低健康检查频率,减少开销

### 高可靠配置

适用于对稳定性要求极高的场景。

```toml
[pool]
max_connections = 100
max_conns_per_host = 10
max_idle_conns = 30
conn_timeout = 60
idle_timeout = 120
max_lifetime = 600
test_timeout = 20
health_check_interval = 15
cleanup_interval = 60
blacklist_check_interval = 300
dns_update_interval = 1800
max_retries = 8
```

**特点**:
- 中等连接数,避免过载
- 较长超时,容忍网络波动
- 高频健康检查,及时发现问题
- 高重试次数,最大化成功率

## 配置最佳实践

### 1. 根据场景选择配置

| 场景 | 推荐配置 | 关键参数 |
|------|----------|----------|
| 开发调试 | 开发环境配置 | 小连接池 + 短超时 |
| 功能测试 | 测试环境配置 | 中连接池 + 标准超时 |
| 生产部署 | 生产环境配置 | 大连接池 + 长超时 |
| 高并发爬取 | 高性能配置 | 超大连接池 + 优化超时 |
| 关键业务 | 高可靠配置 | 高频检查 + 高重试 |

### 2. 性能调优建议

#### 连接池大小

```go
// 根据CPU核心数和预期并发调整
cpuCores := runtime.NumCPU()
optimalConnections := cpuCores * 20 // 经验值

config := &PoolConfig{
    MaxConnections:  optimalConnections,
    MaxConnsPerHost: optimalConnections / 10,
}
```

#### 超时时间

```go
// 根据网络延迟调整
networkLatency := measureNetworkLatency() // 测量网络延迟
config.ConnTimeout = networkLatency * 3    // 超时设为延迟的3倍
config.TestTimeout = networkLatency * 2    // 测试超时稍短
```

#### 健康检查

```go
// 根据连接数调整检查频率
if config.MaxConnections > 500 {
    config.HealthCheckInterval = 120 * time.Second // 大池降低频率
} else {
    config.HealthCheckInterval = 30 * time.Second  // 小池保持频率
}
```

### 3. 安全考虑

#### 配置文件权限

```bash
# 设置配置文件只有所有者可读写
chmod 600 config.toml
chmod 600 config/config.toml
```

#### 敏感信息处理

```go
// 使用环境变量替代敏感配置
googleEarthConfig := config.GoogleEarthConfig{
    BaseURL:      os.Getenv("GOOGLE_EARTH_BASE_URL"),
    AuthEndpoint: os.Getenv("GOOGLE_EARTH_AUTH_ENDPOINT"),
}
```

### 4. 配置验证

```go
func validateConfig(cfg *PoolConfig) error {
    if cfg.MaxConnections <= 0 {
        return errors.New("max_connections必须大于0")
    }
    if cfg.MaxConnsPerHost > cfg.MaxConnections {
        return errors.New("max_conns_per_host不能大于max_connections")
    }
    if cfg.MaxIdleConns > cfg.MaxConnections {
        return errors.New("max_idle_conns不能大于max_connections")
    }
    if cfg.ConnTimeout <= 0 {
        return errors.New("conn_timeout必须大于0")
    }
    return nil
}
```

### 5. 动态配置(未来扩展)

当前系统使用静态配置,未来可考虑支持动态配置:

- **配置中心**: 集成Consul、etcd等配置中心
- **热更新**: 实现配置变更的热更新机制
- **配置版本**: 支持配置版本管理和回滚
- **A/B测试**: 支持不同配置的A/B测试

**当前替代方案**:
1. 修改配置文件
2. 重启应用程序
3. 新配置生效

优点: 简单可靠,状态一致
缺点: 需要短暂服务中断

## 相关文档

- [快速开始](../../QUICKSTART.md) - 配置使用示例
- [系统架构](../../ARCHITECTURE.md) - 配置在架构中的作用
- [API参考](../api/) - 配置API详细说明
- [故障排查](../operations/troubleshooting.md) - 配置相关问题排查

## 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| v0.0.15 | 2025-11-20 | 初始配置文档创建 |
