# diorama_streaming 协议详细文档

## 协议概述

### 基本信息
- **协议名称**: Diorama Streaming Protocol
- **proto 文件**: `GoogleEarth/proto/diorama_streaming.proto`
- **语法版本**: proto2
- **包名**: GoogleEarth
- **Go 包**: GoogleEarth
- **生成文件**: `GoogleEarth/pb/diorama_streaming.pb.go`

### 功能定位
Diorama 协议定义 3D 建筑、模型等全景对象的流式传输格式。它负责:
- 3D 建筑模型的几何数据传输
- 纹理贴图的流式加载
- LOD(细节层次)分层管理
- 四叉树空间索引组织
- 高度模式和对象标志控制

### 与其他协议的关系
Diorama 属于数据内容层,配合空间索引使用:

```
dbroot (配置层)
    ↓
RockTree + QuadTreeSet (空间索引层)
    ↓
Diorama (3D模型数据) + Imagery (影像) + Terrain (地形)
```

## 数据结构详解

### 核心消息类型

#### 1. DioramaMetadata - 全景元数据

**用途**: 描述全景数据的元信息,包括对象列表、LOD级别、数据包信息等。

**关键字段**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Object (组) | - | 对象组(嵌套组语法) |
| ├─ object_index | int32 | 对象索引 |
| ├─ min_quadset_level | int32 | 最小四叉树集合级别 |
| ├─ max_quadset_level | int32 | 最大四叉树集合级别 |
| └─ Object (组) | - | 子对象组 |
| DataPacket (组) | - | 数据包组 |
| ├─ data_packet_index | int32 | 数据包索引 |
| ├─ data_type | string | 数据类型 |
| └─ epoch | int32 | 时间戳 |

**Go 类型定义**:
```go
type DioramaMetadata struct {
    Object []*DioramaMetadata_Object
}

type DioramaMetadata_Object struct {
    ObjectIndex      *int32
    MinQuadsetLevel  *int32
    MaxQuadsetLevel  *int32
    Object           []*DioramaMetadata_Object_Object
}

type DioramaMetadata_DataPacket struct {
    DataPacketIndex  *int32
    DataType         *string
    Epoch            *int32
}
```

#### 2. DioramaQuadset - 全景四叉树集合

**用途**: 组织全景数据的四叉树空间索引。

**关键字段**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| level | int32 | 四叉树级别 |
| quadtree_address | string | 四叉树地址(路径) |
| sparse_data | SparseData[] | 稀疏数据列表 |

#### 3. DioramaDataPacket - 全景数据包

**用途**: 承载实际的 3D 模型几何和纹理数据。

**关键字段**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| Objects (组) | - | 对象组 |
| ├─ object_index | int32 | 对象索引 |
| ├─ geometry_codec | Codec | 几何编解码器 |
| ├─ geometry_data | bytes | 几何数据 |
| ├─ texture_codec | Codec | 纹理编解码器 |
| ├─ texture_data | bytes | 纹理数据 |
| ├─ object_flags | int32 | 对象标志位 |
| ├─ texture_format | int32 | 纹理格式 |
| └─ altitude_mode | AltitudeMode | 高度模式 |
| building_has_info_bubble | bool | 建筑是否有信息气泡(默认true) |

**编解码器枚举**:
```go
const (
    DioramaDataPacket_JPEG         = 0   // JPEG纹理
    DioramaDataPacket_PNG          = 1   // PNG纹理
    DioramaDataPacket_DXT1         = 2   // DXT1压缩纹理
    DioramaDataPacket_DIO_GEOMETRY = 100 // Diorama几何格式
    DioramaDataPacket_BUILDING_Z   = 101 // 建筑Z格式
)
```

**高度模式枚举**:
```go
const (
    DioramaDataPacket_CLAMP_TO_GROUND      = 0 // 贴地
    DioramaDataPacket_RELATIVE_TO_GROUND   = 1 // 相对地面
    DioramaDataPacket_ABSOLUTE             = 2 // 绝对高度
)
```

#### 4. DioramaBlacklist - 全景黑名单

**用途**: 标记需要排除或隐藏的全景对象。

**关键字段**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| object_index | int32 | 被屏蔽的对象索引 |
| epoch | int32 | 时间戳 |

## 使用场景说明

### 场景1: 分层加载3D建筑

根据视口距离动态加载不同细节层级的建筑模型:

```
远距离视图:
  → 加载 LOD 0 (最低细节)
  → 简化几何,小尺寸纹理

中距离视图:
  → 加载 LOD 1-2
  → 中等细节几何,标准纹理

近距离视图:
  → 加载 LOD 3+ (最高细节)
  → 完整几何,高分辨率纹理
```

### 场景2: 四叉树空间查询

使用四叉树地址快速定位区域内的建筑:

```
用户视口: 纬度40.7N, 经度-74.0W
    ↓
转换为四叉树地址: "0312"
    ↓
查询 DioramaQuadset: level=4, address="0312"
    ↓
获取该区域的建筑对象列表
    ↓
请求 DioramaDataPacket 加载具体数据
```

### 场景3: 高度模式应用

不同类型建筑使用不同的高度模式:

- **CLAMP_TO_GROUND**: 平面建筑、道路标记
- **RELATIVE_TO_GROUND**: 一般建筑(相对地形高度)
- **ABSOLUTE**: 高层建筑、地标(绝对海拔高度)

## Go 代码使用示例

### 1. 导入包

```go
import (
    pb "your-project/GoogleEarth"
    "google.golang.org/protobuf/proto"
)
```

### 2. 解析全景元数据

```go
// 从服务器接收的数据
metadataBytes := []byte{...}

// 反序列化
metadata := &pb.DioramaMetadata{}
err := proto.Unmarshal(metadataBytes, metadata)
if err != nil {
    log.Fatalf("解析元数据失败: %v", err)
}

// 遍历对象
for _, obj := range metadata.GetObject() {
    fmt.Printf("对象索引: %d\n", obj.GetObjectIndex())
    fmt.Printf("LOD范围: %d - %d\n", 
        obj.GetMinQuadsetLevel(), 
        obj.GetMaxQuadsetLevel())
    
    // 处理子对象
    for _, subObj := range obj.GetObject() {
        fmt.Printf("  子对象: %d\n", subObj.GetObjectIndex())
    }
}
```

### 3. 创建全景数据包

```go
// 读取几何数据和纹理
geometryData, _ := os.ReadFile("building.geometry")
textureData, _ := os.ReadFile("building.jpg")

// 创建数据包
dataPacket := &pb.DioramaDataPacket{
    BuildingHasInfoBubble: proto.Bool(true),
}

// 添加对象
obj := &pb.DioramaDataPacket_Objects{
    ObjectIndex:     proto.Int32(1001),
    GeometryCodec:   pb.DioramaDataPacket_BUILDING_Z.Enum(),
    GeometryData:    geometryData,
    TextureCodec:    pb.DioramaDataPacket_JPEG.Enum(),
    TextureData:     textureData,
    TextureFormat:   proto.Int32(1),  // RGB
    AltitudeMode:    pb.DioramaDataPacket_RELATIVE_TO_GROUND.Enum(),
    ObjectFlags:     proto.Int32(0x01), // 可见标志
}

dataPacket.Objects = append(dataPacket.Objects, obj)

// 序列化
data, err := proto.Marshal(dataPacket)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}
fmt.Printf("数据包大小: %d 字节\n", len(data))
```

### 4. 解析全景数据包

```go
// 反序列化
dataPacket := &pb.DioramaDataPacket{}
err := proto.Unmarshal(packetBytes, dataPacket)
if err != nil {
    log.Fatalf("解析失败: %v", err)
}

// 处理每个对象
for _, obj := range dataPacket.GetObjects() {
    fmt.Printf("对象 %d:\n", obj.GetObjectIndex())
    
    // 检查几何编解码器
    switch obj.GetGeometryCodec() {
    case pb.DioramaDataPacket_BUILDING_Z:
        fmt.Println("  几何格式: BUILDING_Z")
        // 解码 BUILDING_Z 格式
    case pb.DioramaDataPacket_DIO_GEOMETRY:
        fmt.Println("  几何格式: DIO_GEOMETRY")
        // 解码 DIO_GEOMETRY 格式
    }
    
    // 检查纹理编解码器
    switch obj.GetTextureCodec() {
    case pb.DioramaDataPacket_JPEG:
        fmt.Println("  纹理格式: JPEG")
        // 解码 JPEG 纹理
    case pb.DioramaDataPacket_PNG:
        fmt.Println("  纹理格式: PNG")
    case pb.DioramaDataPacket_DXT1:
        fmt.Println("  纹理格式: DXT1")
    }
    
    // 检查高度模式
    switch obj.GetAltitudeMode() {
    case pb.DioramaDataPacket_CLAMP_TO_GROUND:
        fmt.Println("  高度模式: 贴地")
    case pb.DioramaDataPacket_RELATIVE_TO_GROUND:
        fmt.Println("  高度模式: 相对地面")
    case pb.DioramaDataPacket_ABSOLUTE:
        fmt.Println("  高度模式: 绝对高度")
    }
    
    // 检查对象标志
    flags := obj.GetObjectFlags()
    if flags & 0x01 != 0 {
        fmt.Println("  对象可见")
    }
    if flags & 0x02 != 0 {
        fmt.Println("  对象可点击")
    }
}
```

### 5. 创建四叉树集合

```go
// 创建四叉树集合
quadset := &pb.DioramaQuadset{
    Level:           proto.Int32(5),
    QuadtreeAddress: proto.String("01230"),
}

// 添加稀疏数据
sparseData := &pb.DioramaQuadset_SparseData{
    Index: proto.Int32(0),
    ObjectIndices: []int32{1001, 1002, 1003},
}
quadset.SparseData = append(quadset.SparseData, sparseData)

// 序列化
data, err := proto.Marshal(quadset)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}
```

### 6. 处理对象标志位

```go
// 定义标志位常量
const (
    FLAG_VISIBLE     = 1 << 0  // 0x01
    FLAG_CLICKABLE   = 1 << 1  // 0x02
    FLAG_SHADOW      = 1 << 2  // 0x04
    FLAG_TRANSPARENT = 1 << 3  // 0x08
)

// 设置标志位
func setObjectFlags(visible, clickable, shadow, transparent bool) int32 {
    var flags int32 = 0
    
    if visible {
        flags |= FLAG_VISIBLE
    }
    if clickable {
        flags |= FLAG_CLICKABLE
    }
    if shadow {
        flags |= FLAG_SHADOW
    }
    if transparent {
        flags |= FLAG_TRANSPARENT
    }
    
    return flags
}

// 检查标志位
func checkObjectFlags(flags int32) {
    if flags & FLAG_VISIBLE != 0 {
        fmt.Println("对象可见")
    }
    if flags & FLAG_CLICKABLE != 0 {
        fmt.Println("对象可点击")
    }
    if flags & FLAG_SHADOW != 0 {
        fmt.Println("对象投射阴影")
    }
    if flags & FLAG_TRANSPARENT != 0 {
        fmt.Println("对象有透明度")
    }
}

// 使用示例
flags := setObjectFlags(true, true, true, false)
obj := &pb.DioramaDataPacket_Objects{
    ObjectIndex: proto.Int32(1001),
    ObjectFlags: proto.Int32(flags),
}
```

### 7. LOD 级别管理

```go
// LOD 管理器
type LODManager struct {
    objects map[int32]*LODObject
}

type LODObject struct {
    Index       int32
    MinLevel    int32
    MaxLevel    int32
    DataPackets map[int32][]byte  // level -> data
}

// 根据距离选择合适的 LOD
func (m *LODManager) selectLOD(objectIndex int32, viewDistance float64) int32 {
    obj, ok := m.objects[objectIndex]
    if !ok {
        return 0
    }
    
    // 简单的距离到LOD映射
    var level int32
    if viewDistance < 100 {
        level = obj.MaxLevel  // 最高细节
    } else if viewDistance < 500 {
        level = (obj.MinLevel + obj.MaxLevel) / 2
    } else {
        level = obj.MinLevel  // 最低细节
    }
    
    // 确保在有效范围内
    if level < obj.MinLevel {
        level = obj.MinLevel
    }
    if level > obj.MaxLevel {
        level = obj.MaxLevel
    }
    
    return level
}

// 使用示例
manager := &LODManager{
    objects: make(map[int32]*LODObject),
}

// 从元数据添加对象
for _, obj := range metadata.GetObject() {
    lodObj := &LODObject{
        Index:       obj.GetObjectIndex(),
        MinLevel:    obj.GetMinQuadsetLevel(),
        MaxLevel:    obj.GetMaxQuadsetLevel(),
        DataPackets: make(map[int32][]byte),
    }
    manager.objects[obj.GetObjectIndex()] = lodObj
}

// 选择 LOD
viewDist := 250.0  // 米
lod := manager.selectLOD(1001, viewDist)
fmt.Printf("选择的 LOD 级别: %d\n", lod)
```

## 最佳实践建议

### 1. 字段填充建议

**必填字段**:
- DioramaDataPacket.Objects.object_index: 对象标识
- DioramaDataPacket.Objects.geometry_codec: 几何编解码器
- DioramaDataPacket.Objects.geometry_data: 几何数据
- DioramaDataPacket.Objects.altitude_mode: 高度模式

**推荐字段**:
- texture_codec 和 texture_data: 提升视觉效果
- object_flags: 控制对象行为
- building_has_info_bubble: 支持交互

### 2. 性能优化要点

**LOD 策略**:
- 根据视口距离动态切换 LOD 级别
- 预加载相邻级别的数据,减少切换延迟
- 使用四叉树索引快速裁剪不可见对象

**数据压缩**:
- 优先使用 DXT1 纹理格式(GPU原生,无需解压)
- 几何数据使用 BUILDING_Z 格式(专为建筑优化)
- 批量传输同一区域的多个对象

**内存管理**:
- 及时释放远距离对象的高细节数据
- 缓存常用的纹理和几何数据
- 使用对象池减少 GC 压力

### 3. 常见错误和解决方法

**错误1: 高度模式使用不当**
```go
// ❌ 所有建筑都使用贴地模式
obj.AltitudeMode = pb.DioramaDataPacket_CLAMP_TO_GROUND.Enum()

// ✅ 根据建筑类型选择
if isGround {
    obj.AltitudeMode = pb.DioramaDataPacket_CLAMP_TO_GROUND.Enum()
} else {
    obj.AltitudeMode = pb.DioramaDataPacket_RELATIVE_TO_GROUND.Enum()
}
```

**错误2: 嵌套组访问错误**
```go
// proto2 的 group 语法生成嵌套结构体

// ❌ 错误的访问方式
objects := dataPacket.Objects  // 错误,这是切片

// ✅ 正确的访问方式
objects := dataPacket.GetObjects()  // 返回 []*DioramaDataPacket_Objects
```

### 4. 版本兼容性注意事项

- proto2 语法,所有字段都是 optional
- 添加新的编解码器类型时向后兼容
- object_flags 扩展新标志位不影响旧代码

## 参考资料

### Protocol Buffers
- [Protocol Buffers Language Guide (proto2)](https://protobuf.dev/programming-guides/proto2/)
- [Group Syntax](https://protobuf.dev/programming-guides/proto2/#groups)

### 3D 图形学
- [Level of Detail (LOD)](https://en.wikipedia.org/wiki/Level_of_detail_(computer_graphics))
- [DXT Texture Compression](https://en.wikipedia.org/wiki/S3_Texture_Compression)
- [Quadtree Spatial Indexing](https://en.wikipedia.org/wiki/Quadtree)

### Google Earth
- [Google Earth 3D Buildings](https://earth.google.com/web/)
- [KML Altitude Modes](https://developers.google.com/kml/documentation/altitudemode)
