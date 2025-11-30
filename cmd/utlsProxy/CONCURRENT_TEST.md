# 高并发测试指南

## 概述

`utlsProxy` 现在支持两种模式，可以同时工作：
1. **CONNECT模式**（TCP代理）：向后兼容现有的 `utlsclient` 客户端
2. **PACKET模式**（TUN层）：真正的IP数据包隧道

服务器会根据收到的命令类型自动识别并选择对应的处理模式。

## 准备工作

### 1. 生成TLS证书

```bash
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes
```

### 2. 启动代理服务器

```bash
./utlsProxy \
  -listen 0.0.0.0:443 \
  -token test-token \
  -cert server.crt \
  -key server.key \
  -log debug
```

### 3. 运行高并发测试

#### 基本测试（100个请求，并发10）

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 10 \
  -requests 100 \
  -timeout 30s
```

#### 高并发测试（1000个请求，并发50）

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 50 \
  -requests 1000 \
  -timeout 60s
```

#### 持续压力测试（60秒，并发100）

```bash
./utlsclient-cli \
  -concurrent-test \
  -proxy 127.0.0.1:443 \
  -token test-token \
  -url https://www.google.com \
  -concurrency 100 \
  -requests 0 \
  -duration 60s \
  -timeout 60s
```

## 测试结果解读

测试完成后会显示：

```
=== 并发测试结果 ===
总请求数: 1000
成功请求: 985 (98.50%)
失败请求: 15 (1.50%)
平均延迟: 234ms
最小延迟: 89ms
最大延迟: 1.2s

错误统计:
  连接服务器失败: 10
  HTTP状态码: 500: 5
```

## 性能指标

- **成功率**: 应该保持在95%以上
- **平均延迟**: 取决于网络和服务器性能
- **QPS**: 每秒请求数，反映吞吐量
- **错误分布**: 帮助识别问题类型

## 故障排除

### 连接失败
- 检查服务器是否正在运行
- 验证token是否正确
- 检查防火墙设置

### 大量请求失败
- 降低并发数
- 增加超时时间
- 检查服务器日志

### 性能不佳
- 调整服务器连接池配置
- 检查网络带宽
- 监控服务器资源使用

## 架构说明

### 数据流（CONNECT模式）

```
utlsclient (并发客户端)
    ↓ CONNECT命令 + HTTP请求
utlsProxy (CONNECT处理器)
    ↓ 建立TCP连接
目标服务器
    ↓ HTTP响应
utlsProxy
    ↓ PACKET响应
utlsclient
```

### 当前实现

- ✅ 支持CONNECT命令（TCP代理模式）
- ✅ 支持PACKET命令（TUN模式）
- ✅ 自动模式识别
- ✅ 高并发支持
- ✅ 双向数据转发
