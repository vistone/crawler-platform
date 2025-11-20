# RockTree 协议详细文档

## 协议概述

### 基本信息
- **协议名称**: RockTree Protocol
- **proto 文件**: `GoogleEarth/proto/RockTree.proto`
- **语法版本**: proto2
- **包名**: GoogleEarth.RockTree
- **Go 包**: GoogleEarth
- **生成文件**: `GoogleEarth/pb/RockTree.pb.go`

### 功能定位
RockTree 协议是 Google Earth 的核心空间索引和数据组织协议,使用"Rock"树结构组织地球表面的 3D 瓦片数据。它负责管理:
- 空间节点的键值和元数据
- 几何网格数据(顶点、索引、法线)
- 纹理贴图数据(多种格式和视角)
- 版权信息和采集日期
- LOD(细节层次)控制
- 定向包围盒和碰撞检测

### 与其他协议的关系
RockTree 在 Google Earth 数据架构中处于核心位置:
- **上游**: 接收 dbroot 配置的全局参数和服务器 URL
- **并行**: 与 QuadTreeSet 协作完成空间索引
- **下游**: 为 Diorama/Imagery/Terrain 提供空间分块基础

```
dbroot (配置层)
    ↓
RockTree (空间索引层) ←→ QuadTreeSet
    ↓
Diorama + Imagery + Terrain (数据内容层)
```

## 数据结构详解

### 核心消息类型

#### 1. NodeKey - 节点键值

**用途**: 唯一标识地球表面上的一个空间瓦片节点,类似文件系统路径。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| path | string | optional | "" | 节点路径,使用四叉树编码(如"0123") |
| epoch | int32 | optional | 0 | 时间戳/版本标识,用于缓存控制 |

**Go 类型定义**:
```go
type NodeKey struct {
    Path  *string
    Epoch *int32
}
```

#### 2. NodeMetadata - 节点元数据

**用途**: 存储空间节点的元信息,包含几何属性、时间戳、纹理格式等关键参数。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| path_and_flags | bytes | optional | - | 路径标识组合(高24位:路径长度,低8位:标志位) |
| epoch | int32 | optional | - | 元数据版本时间戳(秒,自Unix纪元) |
| bulk_metadata_epoch | int32 | optional | - | 批量处理时的元数据时间戳 |
| oriented_bounding_box | bytes | optional | - | 定向包围盒数据(9个64位浮点数) |
| meters_per_texel | float | optional | - | 空间分辨率(米/像素) |
| processing_oriented_bounding_box | bytes | optional | - | 处理专用包围盒(6个双精度坐标) |
| imagery_epoch | int32 | optional | - | 影像数据时间戳 |
| available_texture_formats | int32 | optional | - | 可用纹理格式掩码(按位表示) |
| available_view_dependent_textures | int32 | optional | - | 视角相关纹理数量 |
| available_view_dependent_texture_formats | int32 | optional | - | 视角相关纹理格式掩码 |
| dated_nodes | DatedNode[] | repeated | - | 时间关联子节点列表 |
| acquisition_date_range | AcquisitionDateRange | optional | - | 数据采集时间范围 |

**标志位枚举** (NodeMetadata.Flags):
```go
const (
    NodeMetadata_RICH3D_LEAF     = 1  // 富几何细节的叶子节点
    NodeMetadata_RICH3D_NODATA   = 2  // 无效节点(保留结构但无数据)
    NodeMetadata_LEAF            = 4  // 基础叶子节点标识
    NodeMetadata_NODATA          = 8  // 无有效数据节点
    NodeMetadata_USE_IMAGERY_EPOCH = 16 // 使用独立影像时间戳
)
```

#### 3. BulkMetadata - 批量元数据

**用途**: 批量传输多个节点的元数据,优化网络请求效率。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| node_metadata | NodeMetadata[] | repeated | - | 节点元数据列表 |
| head_node_key | NodeKey | optional | - | 头部节点键值 |
| head_node_center | double[] | repeated | - | 头部节点中心坐标(x,y,z) |
| meters_per_texel | float | optional | - | 每个纹理像素对应的米数 |
| default_imagery_epoch | int32 | optional | - | 默认图像纪元 |
| default_available_texture_formats | int32 | optional | - | 默认可用纹理格式 |
| default_available_view_dependent_textures | int32 | optional | - | 默认可用视图相关纹理 |
| default_available_view_dependent_texture_formats | int32 | optional | - | 默认可用视图相关纹理格式 |
| common_dated_nodes | DatedNode[] | repeated | - | 共享的日期节点列表 |
| default_acquisition_date_range | AcquisitionDateRange | optional | - | 默认采集日期范围 |

#### 4. NodeData - 节点数据

**用途**: 承载节点的实际几何、纹理和边界数据。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| matrix_globe_from_mesh | double[] | repeated | - | 从网格到地球的4x4矩阵变换(16个元素) |
| meshes | Mesh[] | repeated | - | 网格列表 |
| copyright_ids | int32[] | repeated | - | 版权ID列表 |
| node_key | NodeKey | optional | - | 节点键值 |
| kml_bounding_box | KmlBoundingBox | optional | - | KML边界框 |
| water_mesh | Mesh | optional | - | 水面网格 |
| overlay_surface_meshes | Mesh[] | repeated | - | 覆盖面网格列表 |
| normal_table | bytes | optional | - | 法线表(压缩数据) |

#### 5. Mesh - 网格

**用途**: 定义 3D 几何网格的顶点、索引、纹理和图层组织。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| vertices | bytes | optional | - | 顶点数据(压缩字节流) |
| vertex_alphas | bytes | optional | - | 顶点透明度 |
| texture_coords | bytes | optional | - | 纹理坐标 |
| indices | bytes | optional | - | 索引数据(三角形网格索引) |
| octant_ranges | bytes | optional | - | 八分体范围 |
| layer_counts | bytes | optional | - | 图层计数 |
| texture | Texture[] | repeated | - | 纹理列表 |
| texture_coordinates | bytes[] | repeated | - | 纹理坐标 |
| uv_offset_and_scale | float[] | repeated | - | UV偏移和缩放 |
| layer_and_octant_counts | bytes | optional | - | 图层和八分体计数 |
| normals | bytes | optional | - | 法线数据 |
| normals_dev | bytes | optional | - | 法线偏差 |
| mesh_id | int32 | optional | - | 网格ID |
| skirt_flags | int32 | optional | - | 裙边标志(用于瓦片边缘缝合) |

**图层枚举** (Mesh.Layer):
```go
const (
    Mesh_OVERGROUND              = 0  // 地上层
    Mesh_TERRAIN_BELOW_WATER     = 1  // 水下地形层
    Mesh_TERRAIN_ABOVE_WATER     = 2  // 水上地形层
    Mesh_TERRAIN_HIDDEN          = 3  // 隐藏地形层
    Mesh_WATER                   = 4  // 水层
    Mesh_WATER_SKIRTS            = 5  // 水裙边
    Mesh_WATER_SKIRTS_INVERTED   = 6  // 反向水裙边
    Mesh_OVERLAY_SURFACE         = 7  // 覆盖面
    Mesh_OVERLAY_SURFACE_SKIRTS  = 8  // 覆盖面裙边
    Mesh_NUM_LAYERS              = 9  // 图层数量
)
```

#### 6. Texture - 纹理

**用途**: 定义网格的纹理贴图数据,支持多种格式和视角。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| data | bytes[] | repeated | - | 纹理数据(可多个字节流,支持mipmap) |
| format | Format | optional | - | 纹理格式(JPG/DXT1/ETC1/PVRTC等) |
| width | int32 | optional | 256 | 纹理宽度(像素) |
| height | int32 | optional | 256 | 纹理高度(像素) |
| view_direction | ViewDirection | optional | - | 视图方向 |
| mesh_id | int32 | optional | - | 网格ID |
| measurement_data | MeasurementData | optional | - | 质量测量数据(PSNR) |

**纹理格式枚举** (Texture.Format):
```go
const (
    Texture_JPG       = 1   // JPG格式
    Texture_DXT1      = 2   // DXT1压缩格式
    Texture_ETC1      = 3   // ETC1压缩格式
    Texture_PVRTC2    = 4   // PVRTC2压缩格式
    Texture_PVRTC4    = 5   // PVRTC4压缩格式
    Texture_CRN_DXT1  = 6   // CRN_DXT1压缩格式
    Texture_HETC2     = 10  // HETC2压缩格式
)
```

**视图方向枚举** (Texture.ViewDirection):
```go
const (
    Texture_ANY       = 0  // 任意视角
    Texture_NADIR     = 1  // 垂直俯视
    Texture_NORTH_45  = 2  // 北向45度
    Texture_EAST_45   = 3  // 东向45度
    Texture_SOUTH_45  = 4  // 南向45度
    Texture_WEST_45   = 5  // 西向45度
)
```

### 请求响应消息类型

#### ViewportMetadataRequest - 视口元数据请求
用于请求当前视图范围内的节点元数据。

#### NodeDataRequest - 节点数据请求
用于请求具体节点的几何网格数据。

#### TextureDataRequest - 纹理数据请求
用于请求节点的纹理贴图数据。

#### CopyrightRequest - 版权请求
用于请求版权信息。

### 辅助消息类型

#### KmlCoordinate - KML坐标
```go
type KmlCoordinate struct {
    Latitude  *float64  // 纬度
    Longitude *float64  // 经度
    Altitude  *float64  // 高度
}
```

#### DatedNode - 带日期的节点
表示带有采集日期的节点引用。

#### AcquisitionDateRange - 采集日期范围
表示数据的采集时间范围。

## 使用场景说明

### 场景1: 加载地球瓦片数据
客户端根据当前视口范围,分步加载所需的地球瓦片数据:

1. **请求视口元数据**: 发送 ViewportMetadataRequest
2. **接收批量元数据**: 获取 BulkMetadata,包含多个 NodeMetadata
3. **选择需要的节点**: 根据 meters_per_texel 计算 LOD,筛选节点
4. **请求节点数据**: 发送 NodeDataRequest 获取 NodeData
5. **请求纹理数据**: 发送 TextureDataRequest 获取 Texture
6. **渲染**: 使用网格和纹理进行 3D 渲染

### 场景2: LOD 管理
根据视口距离动态选择合适的细节层级:

```
视口距离 → meters_per_texel 值 → LOD 级别选择
  近距离 → 小值(如0.5) → 高细节(深层节点)
  远距离 → 大值(如50.0) → 低细节(浅层节点)
```

### 场景3: 缓存控制
使用 epoch 字段实现增量更新:

```
本地缓存 epoch: 12345
服务器返回 epoch: 12345 → 使用缓存,无需下载
服务器返回 epoch: 12400 → 数据已更新,重新下载
```

## Go 代码使用示例

### 1. 导入包

```go
import (
    pb "your-project/GoogleEarth"
    "google.golang.org/protobuf/proto"
    "google.golang.org/protobuf/encoding/protojson"
)
```

### 2. 创建节点键值

```go
// 创建节点键值
nodeKey := &pb.NodeKey{
    Path:  proto.String("0123"),  // 四叉树路径
    Epoch: proto.Int32(99999),    // 时间戳
}

fmt.Printf("节点路径: %s, 版本: %d\n", 
    nodeKey.GetPath(), nodeKey.GetEpoch())
```

### 3. 解析节点元数据

```go
// 假设从服务器接收到的二进制数据
metadataBytes := []byte{...} // 实际的 protobuf 数据

// 反序列化
metadata := &pb.NodeMetadata{}
err := proto.Unmarshal(metadataBytes, metadata)
if err != nil {
    log.Fatalf("解析元数据失败: %v", err)
}

// 访问字段
fmt.Printf("Epoch: %d\n", metadata.GetEpoch())
fmt.Printf("米/像素: %f\n", metadata.GetMetersPerTexel())

// 检查标志位
flags := metadata.GetPathAndFlags()
// 解析标志位(最后一个字节)
if len(flags) > 0 {
    flag := flags[len(flags)-1]
    if flag & pb.NodeMetadata_LEAF != 0 {
        fmt.Println("这是叶子节点")
    }
}
```

### 4. 创建批量元数据请求

```go
// 创建批量元数据
bulkMeta := &pb.BulkMetadata{
    HeadNodeKey: &pb.NodeKey{
        Path:  proto.String("01"),
        Epoch: proto.Int32(12345),
    },
    HeadNodeCenter: []float64{0.0, 0.0, 6371000.0}, // 地球半径
    MetersPerTexel: proto.Float32(10.0),
    DefaultImageryEpoch: proto.Int32(88888),
}

// 添加多个节点元数据
for i := 0; i < 4; i++ {
    meta := &pb.NodeMetadata{
        Epoch: proto.Int32(int32(12345 + i)),
        MetersPerTexel: proto.Float32(10.0 / float32(i+1)),
    }
    bulkMeta.NodeMetadata = append(bulkMeta.NodeMetadata, meta)
}

// 序列化
data, err := proto.Marshal(bulkMeta)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}
fmt.Printf("序列化后大小: %d 字节\n", len(data))
```

### 5. 处理节点数据

```go
// 创建节点数据
nodeData := &pb.NodeData{
    NodeKey: &pb.NodeKey{
        Path:  proto.String("0123"),
        Epoch: proto.Int32(99999),
    },
    // 4x4变换矩阵(行主序)
    MatrixGlobeFromMesh: []float64{
        1, 0, 0, 0,
        0, 1, 0, 0,
        0, 0, 1, 0,
        0, 0, 0, 1,
    },
    CopyrightIds: []int32{1, 2, 3},
}

// 添加网格
mesh := &pb.Mesh{
    MeshId: proto.Int32(1),
    Vertices: []byte{0x01, 0x02, 0x03}, // 实际的压缩顶点数据
    Indices:  []byte{0x00, 0x01, 0x02}, // 实际的索引数据
}
nodeData.Meshes = append(nodeData.Meshes, mesh)

// 序列化
data, err := proto.Marshal(nodeData)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}
```

### 6. 创建纹理数据

```go
// 读取 JPEG 纹理文件
jpegData, err := os.ReadFile("texture.jpg")
if err != nil {
    log.Fatalf("读取纹理失败: %v", err)
}

// 创建纹理对象
texture := &pb.Texture{
    Format:        pb.Texture_JPG.Enum(),
    Width:         proto.Int32(512),
    Height:        proto.Int32(512),
    ViewDirection: pb.Texture_NADIR.Enum(),
    MeshId:        proto.Int32(1),
}

// 添加纹理数据
texture.Data = append(texture.Data, jpegData)

// 可选:添加 mipmap 层级
// mipmap1Data := ... // 256x256 的纹理
// texture.Data = append(texture.Data, mipmap1Data)

fmt.Printf("纹理格式: %v, 尺寸: %dx%d\n",
    texture.GetFormat(), texture.GetWidth(), texture.GetHeight())
```

### 7. JSON 序列化(调试用)

```go
// 将消息转换为 JSON(便于调试和日志)
nodeKey := &pb.NodeKey{
    Path:  proto.String("0123"),
    Epoch: proto.Int32(99999),
}

jsonData, err := protojson.Marshal(nodeKey)
if err != nil {
    log.Fatalf("JSON序列化失败: %v", err)
}
fmt.Printf("JSON: %s\n", string(jsonData))

// 从 JSON 反序列化
nodeKey2 := &pb.NodeKey{}
err = protojson.Unmarshal(jsonData, nodeKey2)
if err != nil {
    log.Fatalf("JSON反序列化失败: %v", err)
}
```

### 8. 计算 LOD 级别

```go
// LOD 计算辅助函数
func calculateLOD(metersPerTexel float32, viewDistance float64) int {
    // 根据视口距离和分辨率计算合适的 LOD
    pixelSize := viewDistance * 0.001 // 简化的像素大小估算
    
    if float64(metersPerTexel) < pixelSize/4 {
        return 0 // 最高细节
    } else if float64(metersPerTexel) < pixelSize/2 {
        return 1
    } else if float64(metersPerTexel) < pixelSize {
        return 2
    } else {
        return 3 // 较低细节
    }
}

// 使用示例
metadata := &pb.NodeMetadata{
    MetersPerTexel: proto.Float32(5.0),
}

viewDist := 10000.0 // 10公里
lod := calculateLOD(metadata.GetMetersPerTexel(), viewDist)
fmt.Printf("建议 LOD 级别: %d\n", lod)
```

### 9. 解析包围盒数据

```go
// 解析定向包围盒(OBB)
func parseOBB(obbBytes []byte) (center [3]float64, extent [3]float64, rotation [9]float64, err error) {
    if len(obbBytes) != 9*8 { // 9个float64,每个8字节
        return center, extent, rotation, fmt.Errorf("OBB数据长度错误: %d", len(obbBytes))
    }
    
    buf := bytes.NewReader(obbBytes)
    
    // 读取中心点(x, y, z)
    for i := 0; i < 3; i++ {
        binary.Read(buf, binary.LittleEndian, &center[i])
    }
    
    // 读取范围(x, y, z)
    for i := 0; i < 3; i++ {
        binary.Read(buf, binary.LittleEndian, &extent[i])
    }
    
    // 读取旋转矩阵(3x3)
    for i := 0; i < 9; i++ {
        binary.Read(buf, binary.LittleEndian, &rotation[i])
    }
    
    return
}

// 使用示例
metadata := &pb.NodeMetadata{
    OrientedBoundingBox: obbData, // 实际的 OBB 数据
}

center, extent, rotation, err := parseOBB(metadata.GetOrientedBoundingBox())
if err != nil {
    log.Fatalf("解析OBB失败: %v", err)
}
fmt.Printf("包围盒中心: %v\n", center)
fmt.Printf("包围盒范围: %v\n", extent)
```

## 最佳实践建议

### 1. 字段填充建议

**必须填充的字段**:
- NodeKey: path 和 epoch 都应填充
- NodeMetadata: epoch 和 meters_per_texel 是核心字段
- NodeData: node_key 和 meshes 不可缺少
- Mesh: vertices 和 indices 是最小要求
- Texture: format 和 data 必须提供

**可选但推荐的字段**:
- NodeMetadata.oriented_bounding_box: 用于视锥裁剪优化
- Mesh.mesh_id: 便于跟踪和调试
- Texture.view_direction: 多视角纹理必需

### 2. 性能优化要点

**批量请求**:
- 使用 BulkMetadata 批量获取元数据,减少网络往返
- 一次请求可包含数十到数百个节点的元数据

**缓存策略**:
- 根据 epoch 字段判断缓存是否有效
- 元数据和实际数据分别缓存,元数据体积小可长期保留

**按需加载**:
- 根据视锥裁剪结果只请求可见节点
- 使用 meters_per_texel 计算 LOD,避免加载过细或过粗的数据

**数据压缩**:
- 网格数据(vertices, indices)已经是压缩格式,不要重复压缩
- 纹理优先使用 DXT1/ETC1 等 GPU 原生格式

### 3. 常见错误和解决方法

**错误1: 解析二进制数据失败**
```go
// ❌ 错误做法
var metadata pb.NodeMetadata
proto.Unmarshal(data, metadata) // 传递值类型

// ✅ 正确做法
metadata := &pb.NodeMetadata{}
err := proto.Unmarshal(data, metadata) // 传递指针
```

**错误2: 访问未设置的字段导致 panic**
```go
// ❌ 错误做法
path := *nodeKey.Path // 如果 Path 为 nil 会 panic

// ✅ 正确做法
path := nodeKey.GetPath() // 如果为 nil 返回空字符串
```

**错误3: 忘记设置枚举类型**
```go
// ❌ 错误做法
texture.Format = pb.Texture_JPG // 枚举值,不是指针

// ✅ 正确做法
texture.Format = pb.Texture_JPG.Enum() // 使用 Enum() 方法获取指针
```

**错误4: 矩阵顺序错误**
```go
// matrix_globe_from_mesh 是行主序(row-major)的 4x4 矩阵
// [0-3]   第一行: [m00, m01, m02, m03]
// [4-7]   第二行: [m10, m11, m12, m13]
// [8-11]  第三行: [m20, m21, m22, m23]
// [12-15] 第四行: [m30, m31, m32, m33]

// 正确的单位矩阵
identity := []float64{
    1, 0, 0, 0,
    0, 1, 0, 0,
    0, 0, 1, 0,
    0, 0, 0, 1,
}
```

### 4. 版本兼容性注意事项

- **proto2 语法**: 所有字段都是 optional,未设置的字段返回默认值
- **向后兼容**: 添加新字段不会破坏旧代码,旧代码会忽略未知字段
- **向前兼容**: 旧数据可以被新代码解析,新增字段使用默认值
- **不要修改字段编号**: 字段编号(如 `= 1`)是协议的核心标识,修改会破坏兼容性

## 参考资料

### Protocol Buffers 官方文档
- [Protocol Buffers Language Guide (proto2)](https://protobuf.dev/programming-guides/proto2/)
- [Go Generated Code Guide](https://protobuf.dev/reference/go/go-generated/)
- [Encoding Specification](https://protobuf.dev/programming-guides/encoding/)

### Google Earth 相关技术
- [KML Specification](https://developers.google.com/kml/)
- [Quadtree Spatial Indexing](https://en.wikipedia.org/wiki/Quadtree)
- [Level of Detail (LOD)](https://en.wikipedia.org/wiki/Level_of_detail_(computer_graphics))

### 3D 图形学基础
- [Oriented Bounding Box (OBB)](https://en.wikipedia.org/wiki/Minimum_bounding_box)
- [Texture Compression Formats](https://en.wikipedia.org/wiki/Texture_compression)
- [Triangle Strips and Meshes](https://en.wikipedia.org/wiki/Triangle_strip)
