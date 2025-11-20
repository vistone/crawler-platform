# GoogleEarth 代码模块文档

## 模块概览

GoogleEarth 包是一个完整的 Google Earth 数据解析库，从 C++ libge 迁移而来，提供四叉树数据解析、坐标转换、加解密等核心功能。

## 文件列表

| 文件名 | 代码行数 | 主要功能 | 复杂度 |
|--------|---------|----------|--------|
| `constants.go` | 66 | 常量定义 | ⭐ |
| `gecrypt.go` | 175 | 加解密 | ⭐⭐⭐ |
| `gedbroot.go` | 35 | dbRoot解析 | ⭐⭐ |
| `jpeg_comment_date.go` | 229 | JPEG日期处理 | ⭐⭐ |
| `qtutils.go` | 764 | 坐标转换工具集 | ⭐⭐⭐⭐ |
| `quadtree_numbering.go` | 204 | 四叉树编号 | ⭐⭐⭐⭐ |
| `quadtree_packet.go` | 655 | 数据包解码 | ⭐⭐⭐⭐⭐ |
| `quadtree_path.go` | 265 | 四叉树路径 | ⭐⭐⭐ |
| `terrain.go` | 307 | 地形数据解码 | ⭐⭐⭐⭐ |
| `tree_numbering.go` | 298 | 通用树编号 | ⭐⭐⭐⭐ |

## 模块分类

### 基础模块

#### 1. constants.go - 常量定义
- 数据库名称（earth, mars, moon, sky, tm）
- API 端点路径模板
- 魔法数字常量
- 四叉树编号规则说明

#### 2. gecrypt.go - 加解密
- XOR 异或解密算法
- ZLIB 解包
- 1024字节密钥管理

#### 3. gedbroot.go - dbRoot 解析
- 更新全局解密密钥
- 解析 dbRoot 版本号

### 数据类型模块

#### 4. jpeg_comment_date.go - JPEG 日期处理
- 历史影像日期表示
- 日期格式转换（YYYYMMDD ↔ time.Time）
- 日期比较和验证

### 坐标系统模块

#### 5. qtutils.go - 坐标转换工具集（762行）

**核心转换链**：
```
经纬度 ↔ 墨卡托坐标 ↔ 瓦片坐标 ↔ 四叉树地址
```

**主要函数**：
- `LatLonToMercator` / `MercatorToLatLon`: 墨卡托投影转换
- `LatLonToTile` / `TileBounds`: 瓦片坐标转换
- `ConvertToQtNode` / `ConvertFromQtNode`: QtNode 编解码
- `LatLon2GoogleTile`: Google Tile 坐标
- `MercatorLatToY` / `MercatorYToLat`: 墨卡托纬度转换
- `ConvertFlatToMercatorQtAddresses`: 平面投影转墨卡托

#### 6. quadtree_path.go - 四叉树路径（265行）

**数据结构**：
- 使用 64 位整数压缩存储路径
- 高 48 位存路径（每层 2 bit）
- 低 16 位存层级

**主要操作**：
- `NewQuadtreePath`: 从 level/row/col 构造
- `NewQuadtreePathFromString`: 从字符串构造（如 "0123"）
- `Parent` / `Child`: 路径导航
- `Concatenate`: 路径拼接
- `Advance`: 前序遍历

### 编号系统模块

#### 7. tree_numbering.go - 通用树编号（298行）

**功能**：
- Subindex ↔ Inorder 转换
- 支持任意分支因子的树
- 预计算转换表（性能优化）

**关键概念**：
- `Subindex`: 从树顶部按层级编号（1, 2, 3, 4, ...）
- `Inorder`: 中序遍历编号
- `mangleSecondRow`: Keyhole 特有的第二行排序

#### 8. quadtree_numbering.go - 四叉树编号（204行）

**Keyhole 特有逻辑**：
- Quadset 分割规则
- 根 quadset（深度4） vs 默认 quadset（深度5）
- 全局节点号计算
- Maps 格式路径转换

**主要函数**：
- `TraversalPathToQuadsetAndSubindex`: 路径拆分
- `QuadsetAndSubindexToTraversalPath`: 路径组合
- `IsQuadsetRootLevel`: 判断 quadset 根层

### 数据解析模块

#### 9. quadtree_packet.go - 数据包解码（655行）

**支持两种格式**：

**格式1：二进制格式（QuadTreePacket16）**
- Keyhole 旧格式
- MagicID: 0x7E2D
- 固定大小的 Quantum 结构

**格式2：Protobuf 格式（QuadtreePacketProtoBuf）**
- 新格式，使用 protobuf
- 更灵活的数据结构

**核心数据结构**：
```go
type QuadTreeQuantum16 struct {
    Children            uint8    // 子节点标志（低4位）+ 其他标志（高4位）
    CNodeVersion        uint16   // 缓存节点版本
    ImageVersion        uint16   // 影像版本
    TerrainVersion      uint16   // 地形版本
    ImageNeighbors      [8]int8  // 影像邻居
    ChannelType         []uint16 // 通道类型
    ChannelVersion      []uint16 // 通道版本
}
```

**标志位解析**：
- Bit 0-3: 子节点标志
- Bit 4: 缓存节点
- Bit 5: 矢量数据（Drawable）
- Bit 6: 影像数据
- Bit 7: 地形数据

#### 10. terrain.go - 地形数据解码（307行）

**数据结构**：
```go
type Mesh struct {
    OriginX, OriginY  float64      // 原点坐标（归一化）
    DeltaX, DeltaY    float64      // 步长（归一化）
    NumPoints         int          // 顶点数量
    NumFaces          int          // 面数量
    Level             int          // 层级
    Vertices          []MeshVertex // 顶点列表
    Faces             []MeshFace   // 面列表（三角形）
}
```

**解码流程**：
1. 读取网格头部（source_size, origin, delta）
2. 解码压缩顶点（8位 x/y + 32位 z）
3. 坐标归一化（归一化值 × 180° + 原点）
4. 高程转换（地球半径单位 → 米）
5. 解码三角形面

## 核心概念

### 1. 四叉树编号系统

```
       c0    c1
    |-----|-----|
 r1 |  3  |  2  |
    |-----|-----|
 r0 |  0  |  1  |
    |-----|-----|
```

**路径表示**：
- "0" = 根节点
- "01" = 根节点的右下子节点
- "0123" = 完整路径

### 2. Quadset 分割

```
Level 0-3:  根 quadset（深度4，不特殊排序）
Level 4-7:  第1个 quadset（深度5，特殊排序第二行）
Level 8-11: 第2个 quadset（深度5，特殊排序第二行）
...
```

### 3. 坐标系统

**经纬度 → 墨卡托**：
```
X = R × lon（弧度）
Y = R × ln(tan(π/4 + lat（弧度）/2))
```

**墨卡托 → 瓦片**：
```
col = (X + worldSize/2) / tileSize
row = (worldSize/2 - Y) / tileSize
```

### 4. 数据加密

**密钥来源**：
1. 默认密钥：固定的 1024 字节
2. dbRoot 密钥：从服务器下发（前8字节为0，后1016字节从 dbRoot 提取）

**解密流程**：
```
1. 检查魔法数（0x32789755 = 加密）
2. XOR 解密（特殊密钥跳转逻辑）
3. 检查解密后魔法数（0x7468DEAD = 成功）
4. ZLIB 解压
```

## 使用示例

### 示例1：获取四叉树数据

```go
package main

import (
    "fmt"
    "crawler-platform/GoogleEarth"
)

func main() {
    // 1. 获取 dbRoot 并更新密钥
    dbRootData := fetchDBRoot("https://kh.google.com/dbRoot.v5")
    version, _ := GoogleEarth.UpdateCryptKeyFromDBRoot(dbRootData)
    fmt.Printf("DBRoot version: %d\n", version)
    
    // 2. 构造四叉树数据请求
    tilekey := "0123"  // 四叉树地址
    epoch := 42        // 版本号
    url := fmt.Sprintf("https://kh.google.com/flatfile?q2-%s-q.%d", tilekey, epoch)
    
    // 3. 获取并解密数据
    encryptedData := fetchData(url)
    unpackedData, _ := GoogleEarth.UnpackGEZlib(encryptedData)
    
    // 4. 解析数据包
    packet := GoogleEarth.NewQuadTreePacket16()
    packet.Decode(unpackedData)
    
    // 5. 提取数据引用
    refs := GoogleEarth.QuadtreeDataReferenceGroup{}
    path := GoogleEarth.NewQuadtreePathFromString(tilekey)
    for _, quantum := range packet.DataInstances {
        quantum.GetDataReferences(&refs, path)
    }
    
    fmt.Printf("Found %d imagery refs\n", len(refs.ImgRefs))
}
```

### 示例2：坐标转换

```go
package main

import (
    "fmt"
    "crawler-platform/GoogleEarth"
)

func main() {
    // 经纬度
    lat, lon := 39.9, 116.4  // 北京
    level := 10
    
    // 转换为四叉树地址
    qtAddr := GoogleEarth.LatLonToQuadtreeAddress(lat, lon, level)
    fmt.Printf("Quadtree address: %s\n", qtAddr)
    
    // 转换为 Google Tile 坐标
    tileX, tileY := GoogleEarth.LatLon2GoogleTile(lat, lon, level)
    fmt.Printf("Google Tile: (%d, %d)\n", tileX, tileY)
    
    // 获取瓦片边界
    minLat, minLon, maxLat, maxLon := GoogleEarth.QuadtreeAddressToBounds(qtAddr)
    fmt.Printf("Bounds: [%.6f, %.6f] - [%.6f, %.6f]\n", 
        minLat, minLon, maxLat, maxLon)
}
```

### 示例3：解析地形数据

```go
package main

import (
    "fmt"
    "crawler-platform/GoogleEarth"
)

func main() {
    // 获取地形数据
    terrainData := fetchTerrainData("https://kh.google.com/...")
    
    // 解包和解密
    unpackedData, _ := GoogleEarth.UnpackGEZlib(terrainData)
    
    // 解析地形
    terrain := GoogleEarth.NewTerrain("0123")
    terrain.Decode(unpackedData)
    
    // 遍历网格
    for qtNode, meshes := range terrain.MeshGroups {
        fmt.Printf("QtNode: %s, Meshes: %d\n", qtNode, len(meshes))
        for i, mesh := range meshes {
            fmt.Printf("  Mesh %d: %d vertices, %d faces\n", 
                i, mesh.NumPoints, mesh.NumFaces)
            
            // 访问顶点
            if len(mesh.Vertices) > 0 {
                v := mesh.Vertices[0]
                fmt.Printf("    First vertex: (%.6f, %.6f, %.2f)\n", 
                    v.X, v.Y, v.Z)
            }
        }
    }
}
```

## 测试覆盖

所有模块都有对应的测试文件：

- `test/googleearth/qtutils_test.go` - 坐标转换测试（32个测试）
- `test/googleearth/quadtree_path_test.go` - 路径操作测试（8个测试）
- `test/googleearth/quadtree_numbering_test.go` - 编号转换测试（5个测试）
- `test/googleearth/tree_numbering_test.go` - 树编号测试（7个测试）
- `test/googleearth/packet_terrain_test.go` - 数据包测试

**测试结果**：52 个测试全部通过 ✅

## 性能特点

1. **预计算优化**：`TreeNumbering` 预计算所有转换表
2. **零拷贝解密**：`geDecrypt` 原地修改数据
3. **压缩存储**：`QuadtreePath` 使用 64 位整数压缩路径
4. **延迟解析**：仅在需要时解析数据

## 注意事项

1. **密钥管理**：必须先调用 `UpdateCryptKeyFromDBRoot` 更新密钥
2. **坐标系统**：理解不同坐标系的转换关系
3. **数据格式**：支持两种数据包格式（二进制 + Protobuf）
4. **层级限制**：最大层级为 24（QuadtreePath）或 32（MAX_LEVEL）
5. **Quadset 分割**：理解根 quadset 和普通 quadset 的区别

## 扩展阅读

- [Google Earth 协议文档](../README.md)
- [Protobuf 定义](../../proto/)
- [测试用例](../../../test/googleearth/)
