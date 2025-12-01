# Crawler Platform

这是一个高性能的爬虫平台，支持多种协议和高级功能。

## 项目结构

```
crawler-platform/
├── cmd/                 # 主要命令行程序
│   ├── UtlsProxy/       # UTLS代理服务器
│   ├── utlsclient/      # UTLS客户端示例
│   ├── TaskCreator/     # 任务创建器
│   └── web-server/      # Web服务端
├── utlsclient/          # UTLS客户端核心库
├── Store/               # 数据存储模块
├── logger/              # 日志模块
├── scheduler/           # 任务调度器
├── localippool/         # 本地IP池
├── remotedomainippool/  # 远程域IP池
├── tools/               # 工具集
├── test/                # 测试文件
└── web/                 # Web界面
```

## 快速开始

### 1. 生成TLS证书

项目包含一个用于生成自签名证书的工具：

```bash
# 生成证书
go run tools/generate_cert.go localhost,127.0.0.1 certs/cert.pem certs/key.pem
```

这会在 `certs/` 目录下生成证书和私钥文件。

### 2. 配置和运行UTLS代理

```bash
cd cmd/UtlsProxy
go run .
```

### 3. 运行UTLS客户端示例

```bash
cd cmd/utlsclient
go run .
```

## TUIC协议支持

本项目支持TUIC v5协议，具有以下特点：
- 基于QUIC的高性能传输
- UDP转发优化
- 0-RTT握手支持
- 多种拥塞控制算法

## 许可证

MIT License

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
