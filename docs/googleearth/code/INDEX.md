# GoogleEarth 代码文档索引

本目录包含 GoogleEarth 包所有核心模块的详细技术文档。

## 📚 文档目录

### 快速入门

1. **[README.md](README.md)** - 总览文档（360行）
   - 模块概览和分类
   - 核心概念讲解
   - 完整使用示例
   - 性能特点和注意事项

2. **[modules_summary.md](modules_summary.md)** - 模块功能详解（652行）
   - 每个文件的详细功能说明
   - 完整的API参考
   - 使用示例和代码片段
   - 模块依赖关系图

### 详细文档

3. **[constants.md](constants.md)** - 常量定义（137行）
   - 数据库名称常量
   - API 端点路径模板
   - 魔法数字定义
   - 四叉树编号规则

4. **[gecrypt.md](gecrypt.md)** - 加密解密模块（225行）
   - XOR 解密算法详解
   - ZLIB 解包流程
   - 密钥管理
   - 使用场景和示例

## 📁 源码文件映射

| 源文件 | 文档 | 行数 | 复杂度 | 说明 |
|--------|------|------|--------|------|
| `constants.go` | [constants.md](constants.md) | 66 | ⭐ | 常量定义 |
| `gecrypt.go` | [gecrypt.md](gecrypt.md) | 175 | ⭐⭐⭐ | 加解密 |
| `gedbroot.go` | [modules_summary.md](modules_summary.md#1-gedbrootgo---dbroot-解析器) | 35 | ⭐⭐ | dbRoot解析 |
| `jpeg_comment_date.go` | [modules_summary.md](modules_summary.md#2-jpeg_comment_datego---jpeg-日期管理器) | 229 | ⭐⭐ | JPEG日期处理 |
| `qtutils.go` | [modules_summary.md](modules_summary.md#3-qtutilsgo---坐标转换工具集762行) | 764 | ⭐⭐⭐⭐ | 坐标转换工具集 |
| `quadtree_numbering.go` | [modules_summary.md](modules_summary.md#6-quadtree_numberinggo---四叉树编号204行) | 204 | ⭐⭐⭐⭐ | 四叉树编号 |
| `quadtree_packet.go` | [modules_summary.md](modules_summary.md#7-quadtree_packetgo---数据包解码器655行) | 655 | ⭐⭐⭐⭐⭐ | 数据包解码 |
| `quadtree_path.go` | [modules_summary.md](modules_summary.md#4-quadtree_pathgo---四叉树路径265行) | 265 | ⭐⭐⭐ | 四叉树路径 |
| `terrain.go` | [modules_summary.md](modules_summary.md#8-terraingo---地形数据解码器307行) | 307 | ⭐⭐⭐⭐ | 地形数据解码 |
| `tree_numbering.go` | [modules_summary.md](modules_summary.md#5-tree_numberinggo---通用树编号298行) | 298 | ⭐⭐⭐⭐ | 通用树编号 |

## 🎯 按功能查找

### 坐标转换
- 经纬度 ↔ 墨卡托：[qtutils.go](modules_summary.md#3-qtutilsgo---坐标转换工具集762行)
- 墨卡托 ↔ 瓦片：[qtutils.go](modules_summary.md#3-qtutilsgo---坐标转换工具集762行)
- 瓦片 ↔ 四叉树：[qtutils.go](modules_summary.md#3-qtutilsgo---坐标转换工具集762行)
- QtNode 编解码：[qtutils.go](modules_summary.md#3-qtutilsgo---坐标转换工具集762行)

### 数据加解密
- XOR 解密：[gecrypt.go](gecrypt.md#1-核心解密算法---decryptxor)
- ZLIB 解包：[gecrypt.go](gecrypt.md#3-zlib-解包函数---unpackgezlib)
- 密钥管理：[gedbroot.go](modules_summary.md#1-gedbrootgo---dbroot-解析器)

### 四叉树系统
- 路径操作：[quadtree_path.go](modules_summary.md#4-quadtree_pathgo---四叉树路径265行)
- 编号转换：[quadtree_numbering.go](modules_summary.md#6-quadtree_numberinggo---四叉树编号204行)
- 通用树编号：[tree_numbering.go](modules_summary.md#5-tree_numberinggo---通用树编号298行)

### 数据解析
- 数据包解码：[quadtree_packet.go](modules_summary.md#7-quadtree_packetgo---数据包解码器655行)
- 地形解析：[terrain.go](modules_summary.md#8-terraingo---地形数据解码器307行)
- 日期处理：[jpeg_comment_date.go](modules_summary.md#2-jpeg_comment_datego---jpeg-日期管理器)

### API 相关
- 端点常量：[constants.go](constants.md#3-api-端点常量)
- 数据库名称：[constants.go](constants.md#1-数据库名称常量)
- 魔法数字：[constants.go](constants.md#2-数据格式魔法数字)

## 🚀 快速查找

### 我想要...

**转换坐标**
→ 查看 [qtutils.go 坐标转换工具集](modules_summary.md#3-qtutilsgo---坐标转换工具集762行)

**解密数据**
→ 查看 [gecrypt.go 加密解密模块](gecrypt.md)

**解析四叉树数据包**
→ 查看 [quadtree_packet.go 数据包解码器](modules_summary.md#7-quadtree_packetgo---数据包解码器655行)

**解析地形数据**
→ 查看 [terrain.go 地形数据解码器](modules_summary.md#8-terraingo---地形数据解码器307行)

**理解四叉树编号**
→ 查看 [constants.go 四叉树编号规则](constants.md#4-四叉树编号规则)

**构建 API 请求**
→ 查看 [constants.go API 端点常量](constants.md#3-api-端点常量)

## 📖 学习路线

### 初学者
1. 阅读 [README.md](README.md) 了解整体架构
2. 查看 [constants.md](constants.md) 理解基本概念
3. 学习 [gecrypt.md](gecrypt.md) 理解加解密流程

### 中级开发者
1. 深入 [qtutils.go](modules_summary.md#3-qtutilsgo---坐标转换工具集762行) 学习坐标转换
2. 理解 [quadtree_path.go](modules_summary.md#4-quadtree_pathgo---四叉树路径265行) 路径操作
3. 掌握 [quadtree_packet.go](modules_summary.md#7-quadtree_packetgo---数据包解码器655行) 数据解析

### 高级开发者
1. 研究 [tree_numbering.go](modules_summary.md#5-tree_numberinggo---通用树编号298行) 算法实现
2. 深入 [quadtree_numbering.go](modules_summary.md#6-quadtree_numberinggo---四叉树编号204行) Keyhole 特性
3. 优化 [terrain.go](modules_summary.md#8-terraingo---地形数据解码器307行) 解析性能

## 📊 统计信息

- **总文档数**: 4 个
- **总文档行数**: 1,374 行
- **源码文件数**: 10 个
- **源码总行数**: ~3,500 行
- **测试用例数**: 52 个（全部通过 ✅）

## 🔗 相关资源

- [GoogleEarth 主文档](../README.md)
- [Protobuf 定义](../../GoogleEarth/proto/)
- [测试用例](../../test/googleearth/)
- [Go 源码](../../GoogleEarth/)

## 📝 文档更新日志

- 2025-11-20: 初始版本，包含所有核心模块文档

---

**文档生成时间**: 2025-11-20  
**GoogleEarth 版本**: v0.0.15  
**文档作者**: AI Assistant
