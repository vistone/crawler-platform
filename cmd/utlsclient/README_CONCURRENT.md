# 高并发测试使用说明

## 概述

`utlsclient` 工具支持高并发测试模式，可以同时启动多个客户端连接，通过 TUIC 代理服务器发送大量请求，用于性能测试和压力测试。

## 使用方法

### 1. 启动代理服务器

首先需要启动 `utlsProxy` 服务器：

```bash
# 生成测试证书（如果还没有）
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes

# 启动服务器
./utlsProxy \
  -listen 0.0.0.0:443 \
  -token test-token \
  -cert server.crt \
  -key server.key \
  -log info
```

### 2. 运行并发测试

使用 `-concurrent-test` 参数启动并发测试：

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 50 \
  -requests 1000 \
  -timeout 30s
```

### 命令行参数

- `-concurrent-test`: 启用并发测试模式（必需）
- `-proxy`: 代理服务器地址（格式: `host:port`）
- `-token`: TUIC 认证令牌
- `-url`: 目标 URL
- `-concurrency`: 并发数（默认: 10）
- `-requests`: 总请求数（默认: 100，0 表示基于时间）
- `-duration`: 测试持续时间（如果 `requests` 为 0）
- `-timeout`: 单个请求超时时间（默认: 30s）
- `-server-port`: 服务器监听端口（默认: 443，仅在 `-start-server` 时使用）

### 示例

#### 示例 1: 发送 1000 个请求，并发数 50

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 50 \
  -requests 1000
```

#### 示例 2: 持续运行 60 秒，并发数 100

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 100 \
  -duration 60s \
  -requests 0
```

#### 示例 3: 高并发压力测试

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 200 \
  -requests 10000 \
  -timeout 60s
```

## 测试结果

测试完成后会显示以下统计信息：

- **总请求数**: 发送的总请求数
- **成功请求**: 成功完成的请求数和百分比
- **失败请求**: 失败的请求数和百分比
- **平均延迟**: 所有成功请求的平均延迟
- **最小延迟**: 最快请求的延迟
- **最大延迟**: 最慢请求的延迟
- **QPS**: 每秒请求数（Queries Per Second）
- **错误统计**: 各种错误的计数

### 示例输出

```
=== 并发测试结果 ===
总请求数: 1000
成功请求: 985 (98.50%)
失败请求: 15 (1.50%)
平均延迟: 234ms
最小延迟: 89ms
最大延迟: 1.2s
QPS: 42.55

错误统计:
  连接服务器失败: 10
  HTTP状态码: 500: 5

测试耗时: 23.5s
实际QPS: 42.55
```

## 注意事项

1. **资源消耗**: 高并发测试会消耗大量系统资源（CPU、内存、网络），请根据系统能力调整并发数
2. **网络限制**: 确保网络带宽足够支持高并发请求
3. **服务器性能**: 代理服务器的性能会影响测试结果
4. **超时设置**: 根据网络情况合理设置超时时间
5. **证书验证**: 测试环境可能使用自签名证书，客户端会自动跳过验证

## 性能优化建议

1. **调整并发数**: 根据服务器性能逐步增加并发数，找到最佳性能点
2. **连接复用**: 当前实现每个请求创建新连接，未来可以优化为连接池复用
3. **批量测试**: 可以同时运行多个测试进程，模拟更真实的负载
4. **监控资源**: 使用系统监控工具观察 CPU、内存、网络使用情况

## 故障排除

### 连接失败

- 检查代理服务器是否正在运行
- 验证代理服务器地址和端口
- 检查防火墙设置

### 大量请求失败

- 降低并发数
- 增加超时时间
- 检查服务器日志

### 性能不佳

- 检查网络延迟
- 调整服务器连接池配置
- 检查系统资源使用情况
