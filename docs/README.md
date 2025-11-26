# 文档中心

欢迎来到 crawler-platform 文档中心! 这里包含了项目的完整文档。

## 快速导航

### 🚀 新手入门

- [快速开始指南](../QUICKSTART.md) - 5分钟快速上手
- [系统架构](../ARCHITECTURE.md) - 了解整体设计
- [更新日志](../CHANGELOG.md) - 查看版本变更

### 📖 核心文档

- [模块文档](modules/) - 各模块详细说明
- [UTLS 客户端文档集](utlsclient/README.md) - HTTP 客户端、连接管理、日志
- [Google Earth 文档集](googleearth/README.md) - 四叉树、地形、加密解密
- [配置参考](configuration/config-reference.md) - 配置项详解
- [逐文件对齐状态](files/status.md) - 文档与源码差异盘点

### 🔧 运维指南

- 待补充（operations/ 目录当前为空）

### 👨‍💻 开发指南

- 待补充（development/ 目录当前为空）

### 📐 设计文档

- [存储架构与键值规范](design/storage-spec.md)
- [任务管理设计](design/task_manager/DESIGN.md)
- [任务管理手册](design/task_manager/README.md)

### 🔍 内部实现

- 待补充（internals/ 目录当前为空）

## 文档结构

```
docs/
├── README.md
├── modules/
│   ├── localippool.md
│   ├── remotedomainippool.md
│   ├── whiteblackippool.md
│   ├── utlsfingerprint.md
│   ├── utlshotconnpool.md
│   └── README.md
├── configuration/
│   └── config-reference.md
├── googleearth/
│   ├── README.md
│   ├── terrain.md
│   ├── quadtreeset.md
│   ├── streaming_imagery.md
│   ├── diorama_streaming.md
│   ├── dbroot.md
│   └── code/
│       ├── constants.md
│       ├── gecrypt.md
│       ├── INDEX.md
│       └── modules_summary.md
├── utlsclient/
│   ├── README.md
│   ├── FILE_STRUCTURE.md
│   ├── LOGGING.md
│   ├── DESIGN_ISSUES.md
│   ├── README_TEST.md
│   └── TEST_SUMMARY.md
├── files/
│   ├── status.md
│   └── Store/
│       ├── bblotdb.md
│       ├── dbpath.md
│       ├── redisdb.md
│       ├── sqlitedb.md
│       └── tilestorage.md
└── reports/
    └── 热连接池性能测试报告.md（位于 test/reports/）
```

## 推荐阅读路径

### 初学者路径

1. [快速开始](../QUICKSTART.md) - 快速上手
2. [系统架构](../ARCHITECTURE.md) - 理解整体设计
3. [热连接池模块](modules/hot-connection-pool.md) - 核心功能
4. [配置参考](configuration/config-reference.md) - 配置优化

### 进阶路径

1. [模块文档](modules/) - 深入各模块细节
2. [Google Earth 文档集](googleearth/) - 重点理解四叉树与地形
3. [存储架构](design/storage-spec.md) - Redis/BBolt/SQLite 键值与结构
4. [性能测试报告](../test/reports/热连接池性能测试报告.md)

### 贡献者路径

- 待补充（development/ 目录当前为空）
- [逐文件文档索引](files/status.md) - 快速了解代码与文档对齐情况

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
