# 热连接池测试文件说明

## 目录结构

```
test/
├── reports/                           # 测试报告
│   └── 热连接池性能测试报告.md         # 详细性能测试报告（2025-11-18）
├── results/                           # 测试结果数据
│   └── ip_pool_full_stats.txt        # 完整测试输出（410KB）
└── test_ip_pool_performance.go       # IP池性能测试程序
```

## 测试程序说明

### test_ip_pool_performance.go

**功能**: 测试热连接池的性能、TLS指纹多样性和Accept-Language随机化

**测试流程**:
1. 读取IP池文件（cmd/utlsclient/kh_google_com.json）
2. 预热阶段：为所有IP建立热连接，记录指纹和语言统计
3. 业务请求阶段：轮询所有IP访问多个URL，测试连接复用

**运行方法**:
```bash
# 基本运行
go run test/test_ip_pool_performance.go

# 保存输出到文件
go run test/test_ip_pool_performance.go > test/results/output.txt 2>&1

# 后台运行
nohup go run test/test_ip_pool_performance.go > test/results/output.txt 2>&1 &
```

**输出内容**:
- 每个IP的TLS指纹和Accept-Language
- 预热进度和统计
- TLS指纹分布（33种）
- Accept-Language分布（1575种，97.8%独特性）
- 各轮次性能数据

## 测试报告说明

### 热连接池性能测试报告.md

详细记录了2025-11-18进行的性能测试，包括：

**测试规模**:
- 1631个IP（840 IPv4 + 791 IPv6）
- 4个测试URL
- 6524次总请求

**核心发现**:
- ✅ 98.8%预热成功率
- ✅ 33种TLS指纹，均匀分布
- ✅ 1575种语言组合，97.8%独特性
- ✅ 热连接性能提升3-6倍
- ✅ 完美的HTTP/2连接复用

**技术改进**:
- HTTP/2协议支持
- IPv6地址支持
- Accept-Language随机化
- 死锁预防机制

## 快速查看测试结果

```bash
# 查看完整报告
cat test/reports/热连接池性能测试报告.md

# 查看TLS指纹统计
grep -A 40 "TLS指纹统计" test/results/ip_pool_full_stats.txt

# 查看Accept-Language统计
grep -A 50 "Accept-Language统计" test/results/ip_pool_full_stats.txt

# 查看语言多样性
grep "语言多样性" test/results/ip_pool_full_stats.txt
```

## 注意事项

1. 测试需要访问外网（kh.google.com）
2. 完整测试约需3-5分钟
3. 建议在网络环境良好时进行测试
4. 测试会生成大量并发连接，注意系统资源
