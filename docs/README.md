# 文档中心

欢迎来到 crawler-platform 文档中心! 这里包含了项目的完整文档。

## 快速导航

### 🚀 新手入门

- [快速开始指南](../QUICKSTART.md) - 5分钟快速上手
- [系统架构](../ARCHITECTURE.md) - 了解整体设计
- [更新日志](../CHANGELOG.md) - 查看版本变更

### 📖 核心文档

- [模块文档](modules/) - 各模块详细说明
- [API参考](api/) - 完整的API文档
- [配置参考](configuration/config-reference.md) - 配置项详解

### 🔧 运维指南

- [部署指南](operations/deployment.md) - 生产环境部署
- [监控告警](operations/monitoring.md) - 系统监控配置
- [故障排查](operations/troubleshooting.md) - 常见问题解决

### 👨‍💻 开发指南

- [贡献指南](development/contributing.md) - 如何贡献代码
- [代码规范](development/coding-standards.md) - 编码标准
- [测试指南](development/testing-guide.md) - 测试方法
- [版本管理](development/version-management.md) - 版本发布流程

### 📐 设计文档

- [连接生命周期](design/connection-lifecycle.md) - 连接管理设计
- [并发控制](design/concurrency-control.md) - 并发安全设计
- [健康检查](design/health-check.md) - 健康检查机制
- [协议协商](design/protocol-negotiation.md) - HTTP/2协议协商

### 🔍 内部实现

- [锁策略分析](internals/lock-strategy.md) - 多级锁机制
- [性能优化技术](internals/performance-optimization.md) - 性能优化细节

## 文档结构

```
docs/
├── modules/              # 模块文档
│   ├── utlsclient.md
│   ├── hot-connection-pool.md
│   ├── tls-fingerprint.md
│   ├── ip-pool-management.md
│   └── googleearth.md
├── api/                  # API参考
│   ├── utlshotconnpool-api.md
│   ├── utlsclient-api.md
│   ├── connection-api.md
│   └── interfaces.md
├── configuration/        # 配置文档
│   ├── config-reference.md
│   ├── pool-config.md
│   └── environment.md
├── operations/           # 运维文档
│   ├── deployment.md
│   ├── monitoring.md
│   └── troubleshooting.md
├── development/          # 开发文档
│   ├── contributing.md
│   ├── coding-standards.md
│   ├── testing-guide.md
│   └── version-management.md
├── design/               # 设计文档
│   ├── connection-lifecycle.md
│   ├── concurrency-control.md
│   ├── health-check.md
│   └── protocol-negotiation.md
└── internals/            # 内部实现
    ├── lock-strategy.md
    └── performance-optimization.md
```

## 推荐阅读路径

### 初学者路径

1. [快速开始](../QUICKSTART.md) - 快速上手
2. [系统架构](../ARCHITECTURE.md) - 理解整体设计
3. [热连接池模块](modules/hot-connection-pool.md) - 核心功能
4. [配置参考](configuration/config-reference.md) - 配置优化

### 进阶路径

1. [API参考](api/) - 深入API细节
2. [设计文档](design/) - 理解设计思想
3. [内部实现](internals/) - 掌握实现细节
4. [性能优化](../test/reports/热连接池性能测试报告.md) - 性能调优

### 贡献者路径

1. [贡献指南](development/contributing.md) - 贡献流程
2. [代码规范](development/coding-standards.md) - 编码标准
3. [测试指南](development/testing-guide.md) - 测试要求
4. [版本管理](development/version-management.md) - 发布流程

## 文档维护

### 更新频率

- 📅 **主要版本发布**: 全面审查和更新
- 📅 **次要版本发布**: 更新相关文档
- 📅 **补丁版本**: 修复文档错误
- 📅 **定期审查**: 每月检查一次

### 反馈渠道

发现文档问题? 请通过以下方式反馈:

- 📝 [提交Issue](https://github.com/yourusername/crawler-platform/issues)
- 💬 [GitHub Discussions](https://github.com/yourusername/crawler-platform/discussions)
- 🔧 [提交PR](https://github.com/yourusername/crawler-platform/pulls)

### 贡献文档

欢迎贡献文档! 请参考:

1. [文档贡献指南](development/contributing.md#文档贡献)
2. [Markdown规范](development/coding-standards.md#文档规范)
3. [文档模板](development/doc-templates/)

## 许可证

文档内容遵循项目许可证 - 详见 [LICENSE](../LICENSE)

---

**最后更新**: 2025-11-20 | **文档版本**: v0.0.15
