# Crawler Platform

**Version: v0.0.25**

基于uTLS的高性能爬虫平台，支持TLS指纹伪装、热连接池和IP池管理。

## 项目特性

### 核心功能

- **🔥 热连接池**: 预建立TLS连接并复用，性能提升3-6倍
- **🎭 TLS指纹伪装**: 支持33种真实浏览器指纹，模拟Chrome、Firefox、Safari、Edge等
- **🌍 多语言支持**: 随机生成Accept-Language头，从90种语言中组合，97.8%独特性
- **📡 双协议支持**: 自动检测HTTP/1.1和HTTP/2，完美支持h2连接复用
- **🌐 双栈网络**: 完整支持IPv4和IPv6地址
- **🔒 安全可靠**: 死锁预防、连接健康检查、自动重试机制

### 性能指标

| 指标 | 数值 | 说明 |
|------|------|------|
| **预热速度** | 75连接/秒 | 1611个连接在21.5秒内建立完成 |
| **成功率** | 98.8% | 高可用性保证 |
| **连接复用率** | 100% | HTTP/2完美复用 |
| **性能提升** | 3-6倍 | 相比每次新建连接 |
| **指纹多样性** | 33种 | TLS指纹均匀分布 |
| **语言独特性** | 97.8% | Accept-Language组合独特性 |

## 快速开始

### 安装依赖

```bash
go mod download
```

### 基本使用

```go
package main

import (
    "net/http"
    "crawler-platform/utlsclient"
)

func main() {
    // 创建热连接池
    pool := utlsclient.NewUTLSHotConnPool(nil)
    defer pool.Close()

    // 获取连接
    conn, err := pool.GetConnection("kh.google.com")
    if err != nil {
        panic(err)
    }

    // 使用连接发送请求
    client := utlsclient.NewUTLSClient(conn)
    req, _ := http.NewRequest("GET", "https://kh.google.com/rt/earth/PlanetoidMetadata", nil)
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    // 归还连接到池中
    pool.PutConnection(conn)
}
```

## 项目结构

```
.
├── cmd/                    # 命令行工具
│   └── utlsclient/        # uTLS客户端CLI
├── utlsclient/            # uTLS客户端核心库
│   ├── utlshotconnpool.go # 热连接池实现
│   ├── utlsclient.go      # HTTP客户端（HTTP/1.1 & HTTP/2）
│   ├── utlsfingerprint.go # TLS指纹库（33种+90种语言）
│   └── ...                # 其他模块
├── test/                  # 测试文件
│   ├── reports/           # 测试报告
│   ├── results/           # 测试结果
│   └── test_ip_pool_performance.go
├── dns/                   # DNS解析模块
├── logger/                # 日志模块
└── ...                    # 其他模块
```

## 核心模块

### 1. 热连接池 (UTLSHotConnPool)

热连接池通过预建立和复用TLS连接，大幅提升性能：

**工作流程**:
1. **预热阶段**: 为所有IP建立TLS连接
2. **获取连接**: 从池中获取可用连接
3. **使用连接**: 发送HTTP/HTTPS请求
4. **归还连接**: 请求完成后归还到池中
5. **连接复用**: 下次请求直接复用现有连接

**关键特性**:
- ✅ 自动协议检测（HTTP/1.1 / HTTP/2）
- ✅ 连接健康检查和超时管理
- ✅ 并发安全（多级锁机制）
- ✅ 死锁预防（双重检查模式）
- ✅ 白名单机制（IP成功验证后加入白名单）

### 2. TLS指纹伪装

支持33种真实浏览器TLS指纹配置：

**Chrome系列** (12种):
- Chrome 133, 131, 120, 115-PQ, 114, 112, 106, 102, 100, 96, 87, 83, Auto

**Firefox系列** (9种):
- Firefox 120, 105, 102, 99, 65, 63, 56, 55, Auto

**Safari系列** (4种):
- Safari 17 (macOS), iOS Safari 14/13/12 (iPhone)

**Edge系列** (3种):
- Edge 106, 85, Auto

每次建立连接时随机选择一个指纹，模拟真实浏览器行为。

### 3. Accept-Language随机化

从90种语言中随机组合2-5种语言，生成独特的Accept-Language头：

**示例**:
```
cs-CZ,uk-UA;q=0.9,bn-IN;q=0.8,no-NO;q=0.7,ro-RO;q=0.6
th-TH,sr-RS;q=0.9,da-DK;q=0.8,ta-LK;q=0.7,lt-LT;q=0.6
vi-VN,en-GB;q=0.9,cs-CZ;q=0.8,zh-HK;q=0.7,de-DE;q=0.6
```

在1611个连接的测试中，生成了1575种独特的语言组合（97.8%独特性）。

### 4. HTTP/2支持

完整实现HTTP/2协议支持：

```go
// 自动检测协商的协议
negotiatedProto := conn.tlsConn.ConnectionState().NegotiatedProtocol

if negotiatedProto == "h2" {
    // 使用HTTP/2客户端
    return c.doHTTP2Request(ctx, req)
} else {
    // 使用HTTP/1.1
    return c.doHTTP1Request(ctx, req)
}
```

**关键改进**:
- ✅ HTTP/2 ClientConn缓存和复用
- ✅ 连接可用性检查（CanTakeNewRequest）
- ✅ 自动协议降级
- ✅ 连接错误自动重建

### 5. IPv6支持

完整支持IPv6地址连接：

```go
// 处理IPv6地址格式：需要用方括号包裹
var address string
if strings.Contains(ip, ":") {
    // IPv6地址
    address = fmt.Sprintf("[%s]:%d", ip, DefaultHTTPSPort)
} else {
    // IPv4地址
    address = fmt.Sprintf("%s:%d", ip, DefaultHTTPSPort)
}
```

在实际测试中，成功连接791个IPv6地址，成功率与IPv4相当。

## 存储与持久化架构（Store）

爬虫平台内置了一个多层存储子系统，用于持久化地理瓦片数据（影像 / 地形 / 矢量等）和元数据。整体结构如下：

- BBolt（嵌入式 KV）
  - 文件级别：按层级分片存储，采用 `getDBPath` 生成路径：
    - 0–8 层：`{dbdir}/{dataType}/base.g3db`
    - 9–12 层：`{dbdir}/{dataType}/8/{前4位前缀}.g3db`
    - 13–16 层：`{dbdir}/{dataType}/12/{前4位前缀}.g3db`
    - 17+ 层：`{dbdir}/{dataType}/{level}/{前4位前缀}.g3db`
  - 主键：使用压缩后的 tilekey（uint64）+ 大端编码为 8 字节 BLOB，包含路径位和层级信息，避免不同层级冲突。
  - 特性：
    - 连接池复用数据库文件句柄
    - 写入前健康检查 + 损坏自动修复（将旧文件重命名为 `.corrupt.<timestamp>` 后重建）
    - 支持批量事务写入，显著提升大批量导入性能

- SQLite（关系型）
  - 文件路径与 BBolt 完全一致，同样通过 `getDBPath` 生成，方便迁移与对照。
  - 主键：同样使用 8 字节 BLOB 主键（压缩后的 tilekey），避免 INTEGER 无法容纳完整 uint64 的问题。
  - 特性：
    - 初始化阶段启用 WAL 模式：`PRAGMA journal_mode=WAL;`
    - 为常用字段（如 epoch、provider_id、时间戳 t）建立索引，支持高效查询和统计
    - 适合做“查询 / 分析 / 统计”的主力数据库

- Redis（缓存 + 任务队列）
  - 作为写缓冲层使用，所有瓦片数据最终必须落盘到 BBolt 或 SQLite，Redis 不作为最终持久化存储。
  - 为不同数据类型（imagery / terrain / vector / q2 / qp 等）分配独立的 Redis DB，实现逻辑隔离；自动从 0–15 号库中选择“key 最少 / 空库”作为安全库，避免影响其他程序。

对应核心文件：
- `Store/bblotdb.go`：BBolt 连接池与读写实现
- `Store/sqlitedb.go`：SQLite 连接与表结构、WAL 初始化
- `Store/redisdb.go`：Redis 客户端管理、多 DB 分配、基础操作
- `Store/tilestorage.go`：统一存储入口，协调 Redis 和 BBolt/SQLite 的读写
- `Store/dbpath.go`：分层存储路径生成（与 tilekey 压缩规则配合）

## 瓦片元数据存储（epoch / provider / tm）

在持久化瓦片数据时，平台会同时管理一些元数据字段（如 epoch、provider），并兼容 KV 和关系型存储：

- KV 存储（BBolt / Redis）
  - 为避免修改、解析原始瓦片数据（BLOB），采用“值头部编码”的方式存储元数据：
    - Value 格式：`epoch(Uvarint) + provider_id(Uvarint) + data`
  - `epoch`：
    - 整数值，来自 dbroot 解析，例如 1029。
    - 使用 Uvarint 编码，通常占 1–3 字节。
  - `provider_id`：
    - 整数 provider ID，对应 dbroot 中的 `ProviderInfoProto.provider_id`。
    - 为节省空间和保持一致性，只存整数 ID，不存版权字符串；版权信息和 vertical_pixel_offset 在运行时通过 provider_id 查表获取。
  - `data`：
    - 原始瓦片二进制数据，不添加头部或版本号。

- SQLite 存储
  - 将元数据拆成独立列，便于索引和查询：
    - imagery / terrain / vector：
      - `tile_id BLOB PRIMARY KEY`（8 字节）
      - `epoch INTEGER NOT NULL`
      - `provider_id INTEGER NULL`（为空时存 NULL）
      - `data BLOB NOT NULL`
    - tm（带时间的影像）：
      - `tile_id BLOB` + `t INTEGER`（毫秒时间戳） 组成主键
      - `epoch INTEGER NOT NULL`
      - `provider_id INTEGER NULL`
      - `data BLOB NOT NULL`
  - 对常用查询字段（如 t、epoch、provider_id）建立索引，支持按时间范围或 provider 过滤。

## q2 / qp 集合与任务状态

q2/qp 类型属于“集合类元数据”，主要用于表示某个区域下的一组瓦片任务。

- 存储规则
  - 仅在层级可被 4 整除的 tilekey 上存储集合信息（锚点层：level=0,4,8,12,...）。
  - 对任意 tilekey，会先映射到最近的锚点祖先（tilekey 前缀长度取 `level - (level % 4)`）。
  - 值存储为原始 q2/qp BLOB，不添加 epoch/provider 头部，epoch 始终来自 dbroot 解析。

- 表结构（锚点表，示意）
  - 推荐字段：
    - `tile_id BLOB PRIMARY KEY`（压缩后的 8 字节 tilekey）
    - `level INTEGER NOT NULL`
    - `data BLOB NOT NULL`
    - `status INTEGER NOT NULL DEFAULT 0`
      - 0 = PENDING（集合任务未完成）
      - 1 = IN_PROGRESS（处理中）
      - 2 = DONE（已完成）
      - 3 = ERROR（失败）
  - 可选字段：
    - `prefix4 INTEGER`：tilekey 前 4 级路径编码，用于区域范围查询。

- 状态管理
  - 初次解析并生成 q2/qp 集合时，将对应锚点记录的 status 置为 0（PENDING）。
  - 当任务系统开始处理该集合时，置为 1（IN_PROGRESS）。
  - 集合下所有下载任务完成后，置为 2（DONE）。
  - 出现不可恢复错误时，可置为 3（ERROR），后续重置为 0 再重试。

## Redis 使用规范（缓存 + q2 任务队列）

Redis 在本平台中主要扮演两种角色：

1. 瓦片数据写缓冲
   - 写入流程：
     - 请求到达后，先将瓦片写入 Redis（键为原始 tilekey，值为 KV 头部 + data）。
     - 后台异步协程从 Redis 读取数据，批量写入 BBolt/SQLite。
     - 持久化成功后，根据配置（`ClearRedisAfterPersist`）决定是否自动删除 Redis 中对应键。
   - 说明：Redis 不作为持久化存储，仅用于降低写入延迟、缓冲高并发写操作。

2. q2 任务队列与状态（整数状态码）
   - 将 q2 解析结果（例如 `q2_0_epoch_1029.json` 中的 `q2_list`）写入 Redis List 作为任务队列。
   - 任务结构：使用 JSON 形式存储任务项（包含 tilekey、version、url 等）。
   - 状态跟踪：
     - `status:{tilekey}`（Hash）：
       - `status_code`: 0=PENDING, 1=IN_PROGRESS, 2=DONE, 3=ERROR
       - `completed`: 0/1
     - 辅助集合（可选）：
       - `status:0`、`status:1`、`status:2`、`status:3`（Set），存储处于对应状态的 tilekey，便于统计和监控。
   - 流程示例：
     - 入队：解析 q2 结果，将任务项 LPUSH 到队列，`status_code=0`。
     - 取任务：Worker 从队列取出任务，将 `status_code` 置为 1（IN_PROGRESS）。
     - 持久化成功：写入 BBolt/SQLite 成功后，将 `status_code` 置为 2（DONE），`completed=1`，并按配置清理 Redis 中对应键。
     - 失败：将 `status_code` 置为 3（ERROR），记录错误信息，后续按需重试。

## 性能测试

### 测试规模

- **IP池大小**: 1631个（840 IPv4 + 791 IPv6）
- **测试URL**: 4个Google Earth API端点
- **总请求数**: 6524次（1631 IP × 4 URL）

### 测试结果

**预热阶段**:
- 成功建立: 1611个连接（98.8%成功率）
- 总耗时: 21.5秒
- 平均速度: 75连接/秒

**热连接阶段**:
- 第1轮: 1631次请求，100%成功，耗时~6秒
- 第2轮: 1631次请求，100%成功，耗时~6秒
- 第3轮: 1631次请求，100%成功，耗时~6秒

**性能对比**:
- 预热阶段: 平均13ms/连接（需要TLS握手）
- 热连接阶段: 平均4ms/请求（复用连接）
- **性能提升: 3倍以上**

### 指纹和语言统计

**TLS指纹分布**（33种）:
- Chrome 100 - Windows: 73次（4.53%）
- Firefox 63 - Windows: 64次（3.97%）
- Chrome 115 PQ - Windows: 59次（3.66%）
- ... 其他30种指纹均匀分布

**Accept-Language多样性**:
- 总组合数: 1575种
- 独特组合: 1541种（97.8%独特性）
- 多次使用: 仅34种组合被使用2-3次

详细测试报告见: `test/reports/热连接池性能测试报告.md`

## 技术改进记录

### 已修复的问题

| # | 问题 | 解决方案 | 状态 |
|---|------|----------|------|
| 1 | cleanupTicker未使用警告 | 删除未使用字段 | ✅ |
| 2 | HTTP/2协议不支持 | 添加HTTP/2检测和处理 | ✅ |
| 3 | TLS握手empty PSK错误 | 添加OmitEmptyPsk配置 | ✅ |
| 4 | HTTP/2连接无法复用 | 缓存HTTP/2 ClientConn | ✅ |
| 5 | HTTP/2验证失败 | 针对h2连接使用状态检查 | ✅ |
| 6 | getExistingConnection死锁 | 解锁后再调用健康检查 | ✅ |
| 7 | IPv6地址连接失败 | 使用方括号包裹IPv6地址 | ✅ |
| 8 | Accept-Language缺失 | 添加随机语言生成 | ✅ |

### 新增功能

- ✅ HTTP/2协议自动检测和切换
- ✅ IPv6地址完整支持
- ✅ Accept-Language随机化（90种语言）
- ✅ 连接池统计信息（指纹分布、语言分布）
- ✅ 性能测试框架
- ✅ 死锁预防机制
- ✅ 双重检查模式

## 使用建议

### 最佳实践

1. **预热连接**: 在开始大规模请求前，先预热所有IP的连接
   ```go
   // 预热阶段
   for _, ip := range ipPool {
       conn, _ := pool.GetConnectionToIP(url, ip)
       // 发送一个简单请求
       client.Do(req)
       pool.PutConnection(conn)
   }
   ```

2. **轮询使用**: 采用【获取-使用-归还】模式
   ```go
   conn, _ := pool.GetConnection(host)
   client := utlsclient.NewUTLSClient(conn)
   resp, _ := client.Do(req)
   pool.PutConnection(conn)  // 一定要归还！
   ```

3. **并发控制**: 建议每50-100个IP加一个小延迟
   ```go
   if (idx+1)%100 == 0 {
       time.Sleep(100 * time.Millisecond)
   }
   ```

4. **超时设置**: 建议设置10-15秒的请求超时
   ```go
   client.SetTimeout(15 * time.Second)
   ```

5. **错误重试**: 对于失败的请求，可以重试或切换IP

### 性能优化建议

1. 使用连接池而非每次创建新连接，性能提升3-6倍
2. 预热阶段可以异步进行，不阻塞主业务
3. 对于长时间运行的任务，定期检查和清理过期连接
4. 根据实际网络情况调整并发数和超时时间
5. 使用HTTP/2时，一个连接可以处理多个并发请求

## 测试

### 运行性能测试

```bash
# 运行IP池性能测试
go run test/test_ip_pool_performance.go

# 保存结果
go run test/test_ip_pool_performance.go > test/results/output.txt 2>&1
```

### 运行单元测试

```bash
# 测试热连接池
go test ./utlsclient -v -run TestUTLSHotConnPool

# 测试TLS指纹
go test ./utlsclient -v -run TestFingerprint

# 运行所有测试
go test ./... -v
```

### 查看测试报告

```bash
# 完整测试报告
cat test/reports/热连接池性能测试报告.md

# 查看TLS指纹统计
grep -A 40 "TLS指纹统计" test/results/ip_pool_full_stats.txt

# 查看语言多样性统计
grep -A 50 "Accept-Language统计" test/results/ip_pool_full_stats.txt
```

## 依赖库

- **refraction-networking/utls**: uTLS库，用于TLS指纹伪装
- **golang.org/x/net/http2**: HTTP/2协议支持
- 其他标准库

## 许可证

[添加许可证信息]

## 贡献

欢迎提交Issue和Pull Request！

## 更新日志

### 2025-11-18

- ✅ 完成热连接池性能优化
- ✅ 添加HTTP/2完整支持
- ✅ 实现IPv6地址支持
- ✅ 添加Accept-Language随机化
- ✅ 修复死锁问题
- ✅ 完成大规模性能测试（1631 IP × 4 URL）
- ✅ 生成详细测试报告
