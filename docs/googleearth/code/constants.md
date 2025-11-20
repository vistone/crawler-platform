# constants.go - 常量定义

## 文件概述

定义了 Google Earth 数据处理所需的所有常量，包括数据库名称、魔法数字、URL 路径模板以及四叉树编号规则说明。

## 主要功能点

### 1. 数据库名称常量

```go
const (
    EARTH = "earth"  // 地球数据库
    MARS  = "mars"   // 火星数据库
    MOON  = "moon"   // 月球数据库
    SKY   = "sky"    // 天空数据库
    TM    = "tm"     // 历史卫星影像数据库
)
```

**用途**：用于标识不同的星球/数据源，在 API 请求中作为 `db` 参数。

### 2. 数据格式魔法数字

```go
const (
    CRYPTED_JPEG_MAGIC         = 0xA6EF9107  // 加密JPEG魔法数
    CRYPTED_MODEL_DATA_MAGIC   = 0x487B      // 加密模型数据魔法数
    CRYPTED_ZLIB_MAGIC         = 0x32789755  // 加密ZLIB魔法数
    DECRYPTED_MODEL_DATA_MAGIC = 0x0183      // 解密后模型数据魔法数
    DECRYPTED_ZLIB_MAGIC       = 0x7468DEAD  // 解密后ZLIB魔法数
)
```

**用途**：识别和验证不同类型的加密/解密数据格式。

### 3. API 端点常量

#### 主机名
```go
HOST_NAME    = "kh.google.com"      // 标准主机
TM_HOST_NAME = "khmdb.google.com"   // 历史数据主机
```

#### 路径模板

| 常量 | 格式 | 说明 |
|------|------|------|
| `DBROOT_PATH` | `/dbRoot.v5` | 获取数据库根信息 |
| `DBROOT_WITH_DB_PATH` | `/dbRoot.v5?db=%s` | 带数据库名称的dbRoot |
| `Q2_PATH` | `/flatfile?q2-%s-q.%d` | 四叉树数据（tilekey, epoch） |
| `QPQ2_PATH` | `/flatfile?db=%s&qp-%s-q.%d` | 带数据库的四叉树数据 |
| `IMAGERY_PATH` | `/flatfile?f1-%s-i.%d` | 影像数据（tilekey, epoch） |
| `IMAGERY_WITH_TM_PATH` | `/flatfile?db=tm&f1-%s-i.%d-%s` | 历史影像（tilekey, epoch, date） |

**使用示例**：
```go
// 获取地球的 dbRoot
url := fmt.Sprintf(DBROOT_WITH_DB_PATH, EARTH)

// 获取四叉树数据包
url := fmt.Sprintf(Q2_PATH, tilekey, epoch)

// 获取历史影像
url := fmt.Sprintf(IMAGERY_WITH_TM_PATH, tilekey, epoch, "2023-11-15")
```

### 4. 四叉树编号规则

#### 基本编号规则
```
       c0    c1
    |-----|-----|
 r1 |  3  |  2  |
    |-----|-----|
 r0 |  0  |  1  |
    |-----|-----|
```

- **0**: 左下象限 (c0, r0)
- **1**: 右下象限 (c1, r0)
- **2**: 右上象限 (c1, r1)
- **3**: 左上象限 (c0, r1)

#### Quadset 概念

> q2 是一个数据集合，只能是 tilekey 长度能被 4 整除的层级才可以当集合

#### Subindex（子索引）编号

两种编号方案：

**1. 标准树（非根节点）- 第二行特殊排序**：
```
                  0
               /     \
             1  86 171 256
          /     \
        2  3  4  5 ...
      /   \
     6  7  8  9  ...
```

**2. Keyhole 根节点 - 正常排序**：
```
                  0
               /     \
             1  2  3  4
          /     \
        5  6  7  8 ...
     /     \
   21 22 23 24  ...
```

**关键区别**：
- 根节点的第二行按顺序排列：1, 2, 3, 4
- 非根节点的第二行有特殊"错乱排序"：1, 86, 171, 256
- 这种差异由 `TreeNumbering` 的 `mangleSecondRow` 参数控制

## 使用场景

1. **API 请求构建**：使用路径模板和数据库名称常量构建请求 URL
2. **数据格式识别**：通过魔法数字判断数据是否加密及其类型
3. **数据解析**：理解四叉树编号规则，正确解析 quadset 和 subindex

## 依赖关系

- 被 `quadtree_numbering.go` 使用（Subindex 编号规则）
- 被 `gecrypt.go` 使用（魔法数字验证）
- 被所有需要构建 API 请求的模块使用

## 注意事项

1. **历史数据特殊性**：`TM` 数据库需要使用专用主机 `khmdb.google.com`
2. **Epoch 参数**：大多数请求需要 epoch（版本号）参数
3. **四叉树编号**：理解编号规则对于正确解析数据包至关重要
4. **Quadset 层级**：只有特定层级（能被4整除的长度）才能作为 quadset
