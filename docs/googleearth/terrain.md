# terrain 协议详细文档

## 协议概述

### 基本信息
- **协议名称**: Terrain Protocol
- **proto 文件**: `GoogleEarth/proto/terrain.proto`
- **语法版本**: proto2
- **包名**: GoogleEarth.Terrain
- **Go 包**: GoogleEarth
- **生成文件**: `GoogleEarth/pb/terrain.pb.go`

### 功能定位
Terrain 协议定义地形高程数据和水面信息的网格结构。它负责:
- 地形高程网格数据传输
- 水面/陆地/海岸线分类
- 水域高度和透明度信息
- 三角形网格优化(条带、列表)
- 瓦片边界无缝拼接

### 与其他协议的关系
Terrain 属于数据内容层,提供地形基础:

```
dbroot (配置层)
    ↓
RockTree + QuadTreeSet (空间索引层)
    ↓
Terrain (地形高程) + Imagery (影像) + Diorama (3D模型)
```

## 数据结构详解

### 核心消息类型

#### 1. WaterSurfaceTileProto - 水面瓦片协议

**用途**: 描述包含水体的地形瓦片,区分陆地、水域和海岸线,是 Terrain 协议的核心。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| tile_type | TileType | optional | ALL_LAND | 瓦片类型(陆地/水域/海岸) |
| Mesh (组) | - | optional | - | 水面网格数据(嵌套组) |
| ├─ altitude_cm | sint32 | optional | - | 水面高度(厘米,支持负值) |
| ├─ x | bytes | optional | - | 网格顶点X坐标(压缩) |
| ├─ y | bytes | optional | - | 网格顶点Y坐标(压缩) |
| ├─ alpha | bytes | optional | - | 透明度数据 |
| ├─ triangle_vertices | sint32[] | repeated | - | 三角形网格索引列表 |
| ├─ Strips (组) | - | repeated | - | 三角形条带 |
| │   └─ strip_vertices | sint32[] | repeated | - | 条带顶点索引 |
| └─ AdditionalEdgePoints (组) | - | repeated | - | 边缘额外控制点 |
|     ├─ x | sint32 | optional | - | X坐标 |
|     └─ y | sint32 | optional | - | Y坐标 |
| terrain_vertex_is_underwater | bytes | optional | - | 地形顶点水下标记位图 |

**Go 类型定义**:
```go
type WaterSurfaceTileProto struct {
    TileType                   *WaterSurfaceTileProto_TileType
    Mesh                       []*WaterSurfaceTileProto_Mesh
    TerrainVertexIsUnderwater  []byte
}

type WaterSurfaceTileProto_Mesh struct {
    AltitudeCm        *int32
    X                 []byte
    Y                 []byte
    Alpha             []byte
    TriangleVertices  []int32
    Strips            []*WaterSurfaceTileProto_Mesh_Strips
    AdditionalEdgePoints []*WaterSurfaceTileProto_Mesh_AdditionalEdgePoints
}
```

#### 2. TileType - 瓦片类型枚举

**用途**: 标识瓦片的水陆属性。

**枚举值**:
```go
const (
    WaterSurfaceTileProto_ALL_LAND  = 1  // 完全陆地瓦片
    WaterSurfaceTileProto_ALL_WATER = 2  // 完全水域瓦片
    WaterSurfaceTileProto_COAST     = 3  // 海岸线瓦片(水陆混合)
)
```

**类型说明**:
- **ALL_LAND**: 无水面网格,仅地形高程
- **ALL_WATER**: 整个瓦片被水覆盖,单一水面高度
- **COAST**: 包含复杂的水陆边界,需要详细网格和 Alpha 数据

#### 3. TerrainPacketExtraDataProto - 地形额外数据协议

**用途**: 扩展地形数据包,支持水面信息和原始数据保留。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| water_tile_quads | WaterSurfaceTileProto[] | 水面瓦片四边形列表 |
| original_terrain_packet | bytes | 原始未处理的地形数据 |

## 使用场景说明

### 场景1: 纯陆地瓦片

内陆地区,无水体:

```
tile_type = ALL_LAND
  ↓
不需要 Mesh 数据
  ↓
仅使用基础地形高程数据
  ↓
渲染为普通地形表面
```

### 场景2: 海洋瓦片

远离陆地的海洋区域:

```
tile_type = ALL_WATER
  ↓
Mesh.altitude_cm = 0 (海平面)
  ↓
无需复杂网格数据
  ↓
渲染为平坦水面
```

### 场景3: 海岸线瓦片

水陆交界的复杂区域:

```
tile_type = COAST
  ↓
Mesh 包含详细数据:
  - X/Y: 水陆边界顶点
  - alpha: 水陆混合透明度
  - triangle_vertices: 三角形网格
  - strips: 优化的条带
  ↓
terrain_vertex_is_underwater: 标记水下顶点
  ↓
渲染为半透明混合效果
```

### 场景4: 河流和湖泊

内陆水体:

```
tile_type = COAST (水陆混合)
  ↓
Mesh.altitude_cm = 河流/湖泊海拔高度
  ↓
X/Y 定义水域边界
  ↓
alpha 处理水陆过渡
  ↓
渲染为内陆水面
```

## Go 代码使用示例

### 1. 导入包

```go
import (
    pb "your-project/GoogleEarth"
    "google.golang.org/protobuf/proto"
    "encoding/binary"
)
```

### 2. 创建陆地瓦片

```go
// 创建纯陆地瓦片
landTile := &pb.WaterSurfaceTileProto{
    TileType: pb.WaterSurfaceTileProto_ALL_LAND.Enum(),
}

// 序列化
data, err := proto.Marshal(landTile)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

fmt.Printf("陆地瓦片大小: %d 字节\n", len(data))
```

### 3. 创建海洋瓦片

```go
// 创建海洋瓦片
waterTile := &pb.WaterSurfaceTileProto{
    TileType: pb.WaterSurfaceTileProto_ALL_WATER.Enum(),
}

// 添加水面网格
mesh := &pb.WaterSurfaceTileProto_Mesh{
    AltitudeCm: proto.Int32(0),  // 海平面
}

waterTile.Mesh = append(waterTile.Mesh, mesh)

// 序列化
data, err := proto.Marshal(waterTile)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

fmt.Printf("海洋瓦片大小: %d 字节\n", len(data))
```

### 4. 创建海岸线瓦片

```go
// 创建海岸线瓦片(水陆混合)
coastTile := &pb.WaterSurfaceTileProto{
    TileType: pb.WaterSurfaceTileProto_COAST.Enum(),
}

// 创建水面网格
mesh := &pb.WaterSurfaceTileProto_Mesh{
    AltitudeCm: proto.Int32(0),  // 海平面
}

// 添加顶点坐标(压缩格式,实际应用需要压缩算法)
// 这里简化为示例数据
mesh.X = []byte{0x01, 0x02, 0x03, 0x04}
mesh.Y = []byte{0x01, 0x02, 0x03, 0x04}

// 添加透明度数据(0=透明, 255=不透明)
mesh.Alpha = []byte{0, 128, 255, 200}

// 添加三角形索引(定义两个三角形)
mesh.TriangleVertices = []int32{0, 1, 2, 1, 2, 3}

coastTile.Mesh = append(coastTile.Mesh, mesh)

// 标记水下顶点(位图:每个顶点1位)
// 假设4个顶点,前2个水下,后2个陆地: 0011 = 0x03
coastTile.TerrainVertexIsUnderwater = []byte{0x03}

// 序列化
data, err := proto.Marshal(coastTile)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

fmt.Printf("海岸线瓦片大小: %d 字节\n", len(data))
```

### 5. 使用三角形条带优化

```go
// 创建优化的网格(使用条带)
mesh := &pb.WaterSurfaceTileProto_Mesh{
    AltitudeCm: proto.Int32(0),
}

// 方式1: 三角形列表(6个索引定义2个三角形)
mesh.TriangleVertices = []int32{0, 1, 2, 1, 2, 3}

// 方式2: 三角形条带(4个索引定义2个三角形,节省空间)
strip := &pb.WaterSurfaceTileProto_Mesh_Strips{
    StripVertices: []int32{0, 1, 2, 3},
}
mesh.Strips = append(mesh.Strips, strip)

fmt.Println("三角形列表: 6个索引")
fmt.Println("三角形条带: 4个索引(节省33%)")
```

### 6. 解析瓦片数据

```go
// 从服务器接收的数据
tileBytes := []byte{...}

// 反序列化
tile := &pb.WaterSurfaceTileProto{}
err := proto.Unmarshal(tileBytes, tile)
if err != nil {
    log.Fatalf("解析失败: %v", err)
}

// 检查瓦片类型
switch tile.GetTileType() {
case pb.WaterSurfaceTileProto_ALL_LAND:
    fmt.Println("纯陆地瓦片")
    // 仅渲染地形
    
case pb.WaterSurfaceTileProto_ALL_WATER:
    fmt.Println("纯水域瓦片")
    // 渲染平坦水面
    for _, mesh := range tile.GetMesh() {
        altitudeM := float64(mesh.GetAltitudeCm()) / 100.0
        fmt.Printf("  水面高度: %.2f 米\n", altitudeM)
    }
    
case pb.WaterSurfaceTileProto_COAST:
    fmt.Println("海岸线瓦片")
    // 渲染复杂水陆混合
    for i, mesh := range tile.GetMesh() {
        fmt.Printf("  网格 %d:\n", i)
        fmt.Printf("    高度: %d cm\n", mesh.GetAltitudeCm())
        fmt.Printf("    顶点数据: %d 字节\n", len(mesh.GetX()))
        fmt.Printf("    透明度数据: %d 字节\n", len(mesh.GetAlpha()))
        fmt.Printf("    三角形索引: %d 个\n", len(mesh.GetTriangleVertices()))
        fmt.Printf("    条带数: %d 个\n", len(mesh.GetStrips()))
    }
}
```

### 7. 处理水下顶点标记

```go
// 解析水下顶点位图
func isVertexUnderwater(bitmap []byte, vertexIndex int) bool {
    byteIndex := vertexIndex / 8
    bitIndex := vertexIndex % 8
    
    if byteIndex >= len(bitmap) {
        return false
    }
    
    return (bitmap[byteIndex] & (1 << bitIndex)) != 0
}

// 使用示例
tile := &pb.WaterSurfaceTileProto{}
proto.Unmarshal(tileBytes, tile)

underwaterBitmap := tile.GetTerrainVertexIsUnderwater()

for i := 0; i < 100; i++ {  // 假设100个顶点
    if isVertexUnderwater(underwaterBitmap, i) {
        fmt.Printf("顶点 %d: 水下\n", i)
    }
}
```

### 8. 添加边缘控制点

```go
// 创建带边缘控制点的网格(用于无缝拼接相邻瓦片)
mesh := &pb.WaterSurfaceTileProto_Mesh{
    AltitudeCm: proto.Int32(0),
}

// 添加主网格数据
mesh.X = []byte{...}
mesh.Y = []byte{...}

// 添加边缘控制点(确保瓦片边界连续)
for i := 0; i < 4; i++ {  // 瓦片4条边
    edgePoint := &pb.WaterSurfaceTileProto_Mesh_AdditionalEdgePoints{
        X: proto.Int32(int32(i * 10)),
        Y: proto.Int32(int32(i * 10)),
    }
    mesh.AdditionalEdgePoints = append(mesh.AdditionalEdgePoints, edgePoint)
}

fmt.Printf("主网格顶点: %d 字节\n", len(mesh.GetX()))
fmt.Printf("边缘控制点: %d 个\n", len(mesh.GetAdditionalEdgePoints()))
```

### 9. 解码压缩的坐标数据

```go
// 简化的坐标解码示例(实际格式可能更复杂)
func decodeCoordinates(compressedData []byte) []int32 {
    coords := make([]int32, 0)
    
    // 假设使用 delta 编码
    var current int32 = 0
    
    for i := 0; i < len(compressedData); i++ {
        delta := int32(int8(compressedData[i]))  // 有符号增量
        current += delta
        coords = append(coords, current)
    }
    
    return coords
}

// 使用示例
mesh := tile.GetMesh()[0]
xCoords := decodeCoordinates(mesh.GetX())
yCoords := decodeCoordinates(mesh.GetY())

fmt.Printf("解码顶点数: %d\n", len(xCoords))
for i := 0; i < len(xCoords); i++ {
    fmt.Printf("顶点 %d: (%d, %d)\n", i, xCoords[i], yCoords[i])
}
```

### 10. 构建完整的地形瓦片包

```go
// 构建包含多个水面网格的地形数据
func buildTerrainPacket(waterTiles []*pb.WaterSurfaceTileProto) *pb.TerrainPacketExtraDataProto {
    packet := &pb.TerrainPacketExtraDataProto{}
    
    // 添加所有水面瓦片
    packet.WaterTileQuads = waterTiles
    
    // 可选:保留原始地形数据
    // packet.OriginalTerrainPacket = originalData
    
    return packet
}

// 使用示例
waterTiles := []*pb.WaterSurfaceTileProto{
    {TileType: pb.WaterSurfaceTileProto_ALL_LAND.Enum()},
    {TileType: pb.WaterSurfaceTileProto_COAST.Enum()},
    {TileType: pb.WaterSurfaceTileProto_ALL_WATER.Enum()},
}

packet := buildTerrainPacket(waterTiles)
data, _ := proto.Marshal(packet)

fmt.Printf("地形包大小: %d 字节\n", len(data))
fmt.Printf("包含瓦片数: %d\n", len(packet.GetWaterTileQuads()))
```

## 最佳实践建议

### 1. 字段填充建议

**必填字段**:
- tile_type: 必须明确指定瓦片类型
- Mesh.altitude_cm: 水面瓦片必需(陆地瓦片可省略)

**推荐字段**:
- X/Y 坐标: 海岸线瓦片必需
- alpha 透明度: 水陆过渡必需
- triangle_vertices 或 Strips: 至少提供一种网格定义
- terrain_vertex_is_underwater: 水陆混合时推荐

### 2. 性能优化要点

**网格优化**:
- 优先使用三角形条带(Strips)减少索引数量
- 对于规则网格,条带可节省30-50%的索引数据
- 边缘控制点仅在必要时添加

**数据压缩**:
- 坐标使用 delta 编码或 zigzag 编码
- Alpha 数据可用 RLE 压缩(水陆边界明确时)
- 水下标记使用位图(1 bit/顶点)

**类型判断优化**:
```go
// 快速路径:纯陆地/纯水域
if tile.GetTileType() != pb.WaterSurfaceTileProto_COAST {
    // 简单处理
    return
}

// 复杂路径:仅海岸线需要详细处理
processCo astMesh(tile.GetMesh())
```

### 3. 常见错误和解决方法

**错误1: 坐标数据长度不一致**
```go
// ❌ 错误:X和Y长度不同
mesh.X = []byte{0x01, 0x02, 0x03}
mesh.Y = []byte{0x01, 0x02}  // 长度不匹配!

// ✅ 正确:确保长度一致
if len(mesh.GetX()) != len(mesh.GetY()) {
    return fmt.Errorf("坐标数据长度不一致")
}
```

**错误2: 高度单位混淆**
```go
// ❌ 错误:直接使用米
mesh.AltitudeCm = proto.Int32(10)  // 10米? NO! 这是10厘米

// ✅ 正确:转换为厘米
altitudeMeters := 10.0
mesh.AltitudeCm = proto.Int32(int32(altitudeMeters * 100))  // 1000厘米
```

**错误3: 三角形索引越界**
```go
// ✅ 验证三角形索引
func validateTriangles(vertices int, indices []int32) error {
    for _, idx := range indices {
        if idx < 0 || idx >= int32(vertices) {
            return fmt.Errorf("索引越界: %d (顶点数: %d)", idx, vertices)
        }
    }
    return nil
}
```

**错误4: 嵌套组访问错误**
```go
// proto2 的 group 语法生成嵌套结构体

// ❌ 错误
strips := mesh.Strips  // 类型是 []*WaterSurfaceTileProto_Mesh_Strips

// ✅ 正确
for _, strip := range mesh.GetStrips() {
    vertices := strip.GetStripVertices()
    // 处理条带
}
```

### 4. 版本兼容性注意事项

- proto2 语法,所有字段都是 optional
- tile_type 默认值是 ALL_LAND(值为1)
- 添加新字段不影响旧代码
- sint32 类型使用 zigzag 编码,支持负数高度(如死海)

## 参考资料

### Protocol Buffers
- [Protocol Buffers Language Guide (proto2)](https://protobuf.dev/programming-guides/proto2/)
- [Encoding: sint32/sint64](https://protobuf.dev/programming-guides/encoding/#signed-ints)
- [Group Syntax](https://protobuf.dev/programming-guides/proto2/#groups)

### 3D 图形学
- [Triangle Strips](https://en.wikipedia.org/wiki/Triangle_strip)
- [Triangle Mesh](https://en.wikipedia.org/wiki/Triangle_mesh)
- [Level of Detail](https://en.wikipedia.org/wiki/Level_of_detail_(computer_graphics))
- [Alpha Blending](https://en.wikipedia.org/wiki/Alpha_compositing)

### 地形相关
- [Digital Elevation Model (DEM)](https://en.wikipedia.org/wiki/Digital_elevation_model)
- [Heightmap](https://en.wikipedia.org/wiki/Heightmap)
- [Terrain Rendering](https://developer.nvidia.com/gpugems/gpugems3/part-i-geometry/chapter-1-generating-complex-procedural-terrains-using-gpu)

### Google Earth
- [Google Earth Engine](https://earthengine.google.com/)
- [Terrain Data](https://developers.google.com/earth-engine/datasets/catalog/USGS_SRTMGL1_003)
