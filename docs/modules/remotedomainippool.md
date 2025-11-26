# RemoteDomainIPPool 远程域名IP池模块

## 概述

`remotedomainippool.go` 实现了一个远程域名IP监控和池化管理系统，能够定期监控指定域名的IP地址变化，获取详细的IP信息，并提供多种格式的数据持久化功能。该模块是爬虫平台的重要组成部分，用于动态获取和管理目标域名的IP资源。

## 核心功能

### 1. 域名IP监控
- **定期DNS查询**：按配置间隔自动查询域名的A记录和AAAA记录
- **多DNS服务器支持**：支持配置多个DNS服务器进行并行查询
- **IP信息获取**：通过ipinfo.io API获取IP的详细地理位置和运营商信息

### 2. 数据持久化
- **多格式支持**：支持JSON、YAML、TOML三种存储格式
- **自动目录创建**：写入文件前自动创建必要的目录结构
- **深拷贝保护**：返回的数据为深拷贝，确保线程安全

### 3. 并发处理
- **并行DNS查询**：使用goroutine并行查询多个DNS服务器
- **并发IP信息获取**：并行获取多个IP的详细信息
- **线程安全缓存**：使用读写锁保护缓存数据

## 主要接口和结构体

### DomainMonitor 接口
定义了域名监控组件的标准行为：
```go
type DomainMonitor interface {
    Start()                                     // 启动监控
    Stop()                                      // 停止监控
    GetDomainPool(domain string) (map[string][]IPRecord, bool)  // 获取域名IP池
}
```

### 数据结构

#### IPRecord
存储IP地址及其详细信息：
```go
type IPRecord struct {
    IP     string          // IP地址
    IPInfo *IPInfoResponse // IP详细信息
}
```

#### IPInfoResponse
映射ipinfo.io API的完整响应，包含：
- **基础信息**：IP、主机名、组织信息
- **地理位置**：城市、地区、国家、经纬度、时区
- **网络信息**：ASN、Anycast、托管、移动、卫星等标记
- **地理对象**：结构化的地理位置信息

#### MonitorConfig
监控配置参数：
```go
type MonitorConfig struct {
    Domains        []string      // 监控的域名列表
    DNSServers     []string      // DNS服务器列表
    IPInfoToken    string        // ipinfo.io API Token
    UpdateInterval time.Duration // 更新间隔
    StorageDir     string        // 存储目录
    StorageFormat  string        // 存储格式
}
```

## 实现方法详解

### 1. 核心监控组件 remoteIPMonitor

#### Start 方法
**功能**：启动域名监控服务
**实现特点**：
- 立即执行一次完整的监控流程
- 启动后台goroutine按配置间隔重复执行
- 使用context和stopChan实现优雅停止

#### Stop 方法
**功能**：优雅停止监控服务
**实现特点**：
- 发送停止信号给后台goroutine
- 等待当前监控周期完成
- 清理资源

### 2. DNS解析实现

#### resolveDomain 方法
**功能**：解析域名的IPv4和IPv6地址
**实现策略**：
- 并行查询所有配置的DNS服务器
- 同时查询A记录（IPv4）和AAAA记录（IPv6）
- 使用sync.Map进行去重，避免重复IP
- 容错处理：单个DNS服务器失败不影响整体

**并发优化**：
```go
var wg sync.WaitGroup
var ipv4Map, ipv6Map sync.Map

for _, addr := range m.config.DNSServers {
    wg.Add(1)
    go func(addr string) {
        defer wg.Done()
        // 并行查询A记录和AAAA记录
    }(addr)
}
```

### 3. IP信息获取

#### fetchIPInfo 方法
**功能**：获取单个IP的详细信息
**实现特点**：
- 使用共享的HTTP客户端提高效率
- 调用ipinfo.io API获取结构化数据
- 错误处理和资源清理

#### fetchIPInfoBatch 方法
**功能**：批量获取多个IP的信息
**优化策略**：
- 并发获取，提高处理速度
- 控制并发数量，避免过度请求
- 错误隔离：单个IP失败不影响其他IP

### 4. 数据持久化

#### saveDomainData 方法
**功能**：保存域名数据到文件
**实现特点**：
- 自动创建目录结构
- 支持多种序列化格式
- 原子写入，避免数据损坏

**格式支持**：
- **JSON**：结构化，易读，支持注释
- **YAML**：人类友好，层次清晰
- **TOML**：配置文件友好，语法简洁

#### loadDomainData 方法
**功能**：从文件加载域名数据
**容错设计**：
- 文件不存在时返回空数据
- 解析失败时静默处理
- 支持所有存储格式

### 5. 缓存管理

#### GetDomainPool 方法
**功能**：获取域名的最新IP池数据
**安全特性**：
- 返回深拷贝数据
- 调用方可以安全修改返回数据
- 读写锁保护并发访问

#### setLatestDomainData 方法
**功能**：更新缓存数据
**实现细节**：
- 使用深拷贝避免数据竞争
- 读写锁保护更新操作

### 6. 辅助功能

#### uniqueStrings 函数
**功能**：字符串切片去重
**实现**：使用map进行去重，保持顺序

#### cloneDomainPool 函数
**功能**：域名数据深拷贝
**重要性**：确保缓存数据的线程安全

## 并发安全设计

### 1. 读写锁保护
- **主缓存锁**：保护latestData映射
- **深拷贝策略**：避免共享可变数据

### 2. 并发控制
- **goroutine池**：控制并发数量
- **sync.Map**：无锁并发数据结构
- **context**：优雅的生命周期管理

### 3. 资源管理
- **HTTP客户端复用**：避免连接泄漏
- **文件句柄管理**：defer确保资源释放
- **goroutine管理**：正确的启动和停止机制

## 性能优化

### 1. 并行处理
- **DNS查询并行化**：同时查询多个DNS服务器
- **IP信息获取并行化**：并发获取多个IP信息
- **A/AAAA记录并行**：同时查询IPv4和IPv6

### 2. 缓存策略
- **内存缓存**：避免重复计算
- **文件缓存**：持久化存储，支持重启恢复

### 3. 批量操作
- **批量API调用**：减少网络开销
- **批量文件操作**：提高I/O效率

## 错误处理策略

### 1. 容错设计
- **DNS服务器故障**：单个服务器失败不影响整体
- **API调用失败**：记录错误但继续处理其他IP
- **文件操作失败**：提供详细错误信息

### 2. 日志记录
- **详细的错误信息**：便于问题诊断
- **操作状态记录**：监控执行情况

## 使用场景

### 1. 目标网站监控
- 监控目标网站的IP地址变化
- 及时发现IP变更，调整爬虫策略

### 2. IP资源收集
- 收集高质量IP地址资源
- 获取IP的地理位置和运营商信息

### 3. 负载均衡
- 基于IP信息进行智能负载分配
- 避免同一运营商或地区的IP过度使用

## 配置示例

```go
config := MonitorConfig{
    Domains:        []string{"example.com", "target.com"},
    DNSServers:     []string{"8.8.8.8:53", "1.1.1.1:53"},
    IPInfoToken:    "your-api-token",
    UpdateInterval: 30 * time.Minute,
    StorageDir:     "/data/domain-pools",
    StorageFormat:  "json",
}
```

## 扩展性设计

### 1. 接口抽象
- DomainMonitor接口支持多种实现
- 便于单元测试和功能扩展

### 2. 配置灵活
- 支持多种存储格式
- 可配置的监控间隔和DNS服务器

### 3. 数据格式扩展
- 易于添加新的存储格式
- 支持自定义数据字段

## 总结

RemoteDomainIPPool模块是一个功能完善的域名IP监控系统，通过并发DNS查询、IP信息获取、数据持久化等功能，为爬虫平台提供了动态IP资源管理能力。该模块注重并发安全、性能优化和容错处理，是一个生产级别的可靠组件。
