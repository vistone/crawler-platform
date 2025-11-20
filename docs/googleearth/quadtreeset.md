# QuadTreeSet 协议文档

## 协议概述

**文件名**: `quadtreeset.proto`  
**包名**: `GoogleEarth.Q2`  
**语法版本**: proto2  
**Go 包**: `GoogleEarth`

### 功能定位

QuadTreeSet 协议定义了 Google Earth 中四叉树瓦片数据的层次结构和时间版本管理系统。它是 Google Earth 空间索引的核心组成部分，负责组织和管理不同类型的地理数据图层（影像、地形、矢量等），并支持历史数据的时间旅行功能。

### 与其他协议的关系

- **RockTree**: 提供节点级别的空间索引和几何数据
- **QuadTreeSet**: 提供瓦片级别的图层组织和版本管理
- **DbRoot**: 提供全局配置和图层树结构定义

## 核心数据结构

### QuadtreeNode - 四叉树节点

表示四叉树中的一个节点，包含该节点的所有图层、通道和元数据信息。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| flags | int32 | 可选 | 节点标志位，使用位掩码表示节点特性 |
| cache_node_epoch | int32 | 可选 | 缓存节点时间戳，用于缓存失效控制 |
| layer | QuadtreeLayer[] | 可选 | 图层列表，支持多图层叠加显示 |
| channel | QuadtreeChannel[] | 可选 | 通道列表，用于数据流控制 |

#### NodeFlags 枚举

| 标志名 | 值 | 说明 |
|--------|---|------|
| NODE_FLAGS_CHILD_COUNT | 4 | 子节点计数标志 |
| NODE_FLAGS_CACHE_BIT | 4 | 缓存位标志 |
| NODE_FLAGS_DRAWABLE_BIT | 5 | 可绘制位标志，表示该节点有可渲染内容 |
| NODE_FLAGS_IMAGE_BIT | 6 | 图像位标志，表示包含影像数据 |
| NODE_FLAGS_TERRAIN_BIT | 7 | 地形位标志，表示包含地形数据 |

#### 使用示例

```go
// 创建四叉树节点
node := &pb.QuadtreeNode{
    Flags:           proto.Int32(1 << 5 | 1 << 6), // DRAWABLE + IMAGE
    CacheNodeEpoch:  proto.Int32(12345),
    Layer:           make([]*pb.QuadtreeLayer, 0),
    Channel:         make([]*pb.QuadtreeChannel, 0),
}

// 检查节点标志
hasImage := (node.GetFlags() & (1 << 6)) != 0
hasTerrain := (node.GetFlags() & (1 << 7)) != 0

fmt.Printf("节点包含影像: %v, 包含地形: %v\n", hasImage, hasTerrain)
```

### QuadtreeLayer - 四叉树图层

定义单个数据图层的类型、版本和提供商信息。

#### 字段说明

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| type | LayerType | 必填 | - | 图层类型（影像/地形/矢量/历史影像） |
| layer_epoch | int32 | 必填 | - | 图层时间戳，用于版本管理和缓存控制 |
| provider | int32 | 可选 | - | 数据提供商标识符 |
| dates_layer | QuadtreeImageryDates | 可选 | - | 日期图层信息，用于历史影像 |

#### LayerType 枚举

| 类型名 | 值 | 说明 | 用途 |
|--------|---|------|------|
| LAYER_TYPE_IMAGERY | 0 | 影像图层 | 卫星图、航拍影像 |
| LAYER_TYPE_TERRAIN | 1 | 地形图层 | 高程数据、DEM |
| LAYER_TYPE_VECTOR | 2 | 矢量图层 | 道路、边界等矢量数据 |
| LAYER_TYPE_IMAGERY_HISTORY | 3 | 影像历史图层 | 支持时间旅行的历史影像 |

#### 使用示例

```go
// 创建影像图层
imageryLayer := &pb.QuadtreeLayer{
    Type:        pb.QuadtreeLayer_LAYER_TYPE_IMAGERY.Enum(),
    LayerEpoch:  proto.Int32(56789),
    Provider:    proto.Int32(1),
}

// 创建历史影像图层
historyLayer := &pb.QuadtreeLayer{
    Type:        pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY.Enum(),
    LayerEpoch:  proto.Int32(99999),
    Provider:    proto.Int32(2),
    DatesLayer: &pb.QuadtreeImageryDates{
        SharedTileDate:        proto.Int32(20231115),
        SharedTileMilliseconds: proto.Int32(0),
    },
}

// 检查图层类型
if imageryLayer.GetType() == pb.QuadtreeLayer_LAYER_TYPE_IMAGERY {
    fmt.Println("这是影像图层")
}
```

### QuadtreeChannel - 四叉树通道

定义数据通道的配置信息。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| type | int32 | 必填 | 通道类型标识符 |
| channel_epoch | int32 | 必填 | 通道时间戳，用于版本控制 |

#### 使用示例

```go
// 创建数据通道
channel := &pb.QuadtreeChannel{
    Type:         proto.Int32(1),
    ChannelEpoch: proto.Int32(11111),
}
```

### QuadtreeImageryDates - 四叉树影像日期

管理历史影像的日期信息，支持时间旅行功能。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| dated_tile | QuadtreeImageryDatedTile[] | 可选 | 日期瓦片列表，每个表示特定日期的影像 |
| shared_tile_date | int32 | 可选 | 共享瓦片日期（YYYYMMDD 格式） |
| coarse_tile_dates | int32[] | 可选 | 粗粒度瓦片日期列表，用于低精度预览 |
| shared_tile_milliseconds | int32 | 可选 | 共享瓦片的毫秒数（一天内的时间） |

#### 使用示例

```go
// 创建历史影像日期信息
dates := &pb.QuadtreeImageryDates{
    SharedTileDate:         proto.Int32(20231115), // 2023年11月15日
    SharedTileMilliseconds: proto.Int32(43200000), // 正午12:00
    CoarseTileDates:        []int32{20231101, 20231110, 20231120},
}

// 添加具体日期的瓦片
datedTile := &pb.QuadtreeImageryDatedTile{
    Date:           proto.Int32(20231115),
    DatedTileEpoch: proto.Int32(77777),
    Provider:       proto.Int32(1),
}
dates.DatedTile = append(dates.DatedTile, datedTile)
```

### QuadtreeImageryDatedTile - 影像日期瓦片

表示特定日期的影像瓦片数据。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| date | int32 | 必填 | 日期（YYYYMMDD 格式，如 20231115） |
| dated_tile_epoch | int32 | 必填 | 日期瓦片时间戳 |
| provider | int32 | 必填 | 数据提供商标识符 |
| timed_tiles | QuadtreeImageryTimedTile[] | 可选 | 定时瓦片列表（同一天的不同时刻） |

#### 使用示例

```go
// 创建特定日期的影像瓦片
datedTile := &pb.QuadtreeImageryDatedTile{
    Date:           proto.Int32(20231115),
    DatedTileEpoch: proto.Int32(88888),
    Provider:       proto.Int32(1),
}

// 添加同一天不同时刻的瓦片
morningTile := &pb.QuadtreeImageryTimedTile{
    Milliseconds:   proto.Int32(28800000), // 上午8:00
    TimedTileEpoch: proto.Int32(88881),
    Provider:       proto.Int32(1),
}
noonTile := &pb.QuadtreeImageryTimedTile{
    Milliseconds:   proto.Int32(43200000), // 正午12:00
    TimedTileEpoch: proto.Int32(88882),
    Provider:       proto.Int32(1),
}
datedTile.TimedTiles = []*pb.QuadtreeImageryTimedTile{morningTile, noonTile}
```

### QuadtreeImageryTimedTile - 影像定时瓦片

表示特定时刻的影像瓦片数据（一天内的具体时间）。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| milliseconds | int32 | 必填 | 毫秒数（一天内的时间，0-86400000） |
| timed_tile_epoch | int32 | 必填 | 定时瓦片时间戳 |
| provider | int32 | 可选 | 数据提供商标识符 |

### QuadtreePacket - 四叉树数据包

传输四叉树节点数据的容器，使用稀疏表示优化传输效率。

#### 字段说明

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| packet_epoch | int32 | 必填 | 数据包时间戳 |
| SparseQuadtreeNode | group | 可选 | 稀疏四叉树节点列表（proto2 group 语法） |

#### SparseQuadtreeNode 组

| 字段名 | 类型 | 必填 | 说明 |
|--------|------|------|------|
| index | int32 | 必填 | 节点索引（在四叉树中的位置，0-3 表示四个子节点） |
| Node | QuadtreeNode | 必填 | 节点数据 |

#### 使用示例

```go
// 创建四叉树数据包
packet := &pb.QuadtreePacket{
    PacketEpoch: proto.Int32(99999),
}

// 添加稀疏节点（只传输有数据的节点）
sparseNode1 := &pb.QuadtreePacket_SparseQuadtreeNode{
    Index: proto.Int32(0), // 第一个子节点（西北）
    Node: &pb.QuadtreeNode{
        Flags:          proto.Int32(1 << 5 | 1 << 6),
        CacheNodeEpoch: proto.Int32(12345),
    },
}
sparseNode2 := &pb.QuadtreePacket_SparseQuadtreeNode{
    Index: proto.Int32(2), // 第三个子节点（东南）
    Node: &pb.QuadtreeNode{
        Flags:          proto.Int32(1 << 7),
        CacheNodeEpoch: proto.Int32(12346),
    },
}

// 注意：子节点1和3没有数据，因此不传输（稀疏表示）
packet.SparseQuadtreeNode = []*pb.QuadtreePacket_SparseQuadtreeNode{
    sparseNode1,
    sparseNode2,
}

// 处理数据包
for _, sparse := range packet.SparseQuadtreeNode {
    fmt.Printf("节点索引: %d, 标志: %d\n", 
        sparse.GetIndex(), 
        sparse.Node.GetFlags())
}
```

## 数据组织模式

### 稀疏四叉树设计

QuadTreeSet 采用稀疏四叉树表示，只传输包含数据的节点：

```
根节点 (packet_epoch: 99999)
├── [0] 西北子节点 (包含影像数据)
├── [1] 东北子节点 (空，不传输)
├── [2] 东南子节点 (包含地形数据)
└── [3] 西南子节点 (空，不传输)
```

优势：
- **带宽优化**：只传输有数据的节点，减少网络流量
- **内存优化**：客户端只需存储实际存在的节点
- **灵活性**：支持任意稀疏程度的数据分布

### 多图层叠加

同一节点可包含多种类型的图层：

```go
node := &pb.QuadtreeNode{
    Layer: []*pb.QuadtreeLayer{
        {
            Type:       pb.QuadtreeLayer_LAYER_TYPE_IMAGERY.Enum(),
            LayerEpoch: proto.Int32(10001),
        },
        {
            Type:       pb.QuadtreeLayer_LAYER_TYPE_TERRAIN.Enum(),
            LayerEpoch: proto.Int32(10002),
        },
        {
            Type:       pb.QuadtreeLayer_LAYER_TYPE_VECTOR.Enum(),
            LayerEpoch: proto.Int32(10003),
        },
    },
}
```

### 时间版本管理

使用多层次的 epoch 时间戳实现精细的版本控制：

```
QuadtreePacket.packet_epoch       # 数据包级别版本
  └── QuadtreeNode.cache_node_epoch   # 节点级别版本
        └── QuadtreeLayer.layer_epoch      # 图层级别版本
              └── DatedTile.dated_tile_epoch   # 日期瓦片级别版本
                    └── TimedTile.timed_tile_epoch  # 时刻瓦片级别版本
```

缓存策略示例：

```go
func shouldUpdate(cached *pb.QuadtreeNode, fresh *pb.QuadtreeNode) bool {
    // 比较节点版本
    if cached.GetCacheNodeEpoch() < fresh.GetCacheNodeEpoch() {
        return true
    }
    
    // 比较各图层版本
    for i, freshLayer := range fresh.Layer {
        if i >= len(cached.Layer) {
            return true
        }
        if cached.Layer[i].GetLayerEpoch() < freshLayer.GetLayerEpoch() {
            return true
        }
    }
    
    return false
}
```

## 使用场景

### 场景1: 加载当前视图的瓦片数据

```go
func loadViewportTiles(minLat, maxLat, minLon, maxLon float64, zoomLevel int) error {
    // 1. 计算需要加载的瓦片索引范围
    tiles := calculateTileIndices(minLat, maxLat, minLon, maxLon, zoomLevel)
    
    // 2. 请求瓦片数据包
    for _, tileIndex := range tiles {
        packet, err := fetchQuadtreePacket(tileIndex)
        if err != nil {
            return err
        }
        
        // 3. 处理稀疏节点
        for _, sparse := range packet.SparseQuadtreeNode {
            node := sparse.Node
            
            // 4. 处理各个图层
            for _, layer := range node.Layer {
                switch layer.GetType() {
                case pb.QuadtreeLayer_LAYER_TYPE_IMAGERY:
                    loadImageryData(layer)
                case pb.QuadtreeLayer_LAYER_TYPE_TERRAIN:
                    loadTerrainData(layer)
                case pb.QuadtreeLayer_LAYER_TYPE_VECTOR:
                    loadVectorData(layer)
                }
            }
        }
    }
    
    return nil
}
```

### 场景2: 时间旅行（查看历史影像）

```go
func loadHistoricalImagery(lat, lon float64, date int32) error {
    // 1. 获取包含历史数据的节点
    packet, err := fetchQuadtreePacketForLocation(lat, lon)
    if err != nil {
        return err
    }
    
    // 2. 查找历史影像图层
    for _, sparse := range packet.SparseQuadtreeNode {
        for _, layer := range sparse.Node.Layer {
            if layer.GetType() != pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY {
                continue
            }
            
            // 3. 在日期图层中查找指定日期
            dates := layer.GetDatesLayer()
            for _, datedTile := range dates.DatedTile {
                if datedTile.GetDate() == date {
                    // 4. 加载该日期的影像
                    fmt.Printf("找到日期 %d 的影像, epoch: %d\n",
                        datedTile.GetDate(),
                        datedTile.GetDatedTileEpoch())
                    
                    // 5. 如果有多个时刻，选择最接近的
                    for _, timedTile := range datedTile.TimedTiles {
                        ms := timedTile.GetMilliseconds()
                        hour := ms / 3600000
                        minute := (ms % 3600000) / 60000
                        fmt.Printf("  时刻: %02d:%02d\n", hour, minute)
                    }
                    
                    return loadImageryByEpoch(datedTile.GetDatedTileEpoch())
                }
            }
        }
    }
    
    return fmt.Errorf("未找到日期 %d 的历史影像", date)
}
```

### 场景3: 缓存管理

```go
type QuadtreeCache struct {
    cache map[string]*pb.QuadtreePacket
    mu    sync.RWMutex
}

func (c *QuadtreeCache) Get(tileKey string) (*pb.QuadtreePacket, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    packet, ok := c.cache[tileKey]
    return packet, ok
}

func (c *QuadtreeCache) Update(tileKey string, freshPacket *pb.QuadtreePacket) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    cached, exists := c.cache[tileKey]
    if !exists {
        // 直接缓存
        c.cache[tileKey] = freshPacket
        return
    }
    
    // 检查是否需要更新
    if cached.GetPacketEpoch() >= freshPacket.GetPacketEpoch() {
        return // 缓存的版本更新，不需要更新
    }
    
    // 更新缓存
    c.cache[tileKey] = freshPacket
    log.Printf("更新瓦片 %s 缓存，epoch: %d -> %d",
        tileKey,
        cached.GetPacketEpoch(),
        freshPacket.GetPacketEpoch())
}
```

## 最佳实践

### 1. 稀疏节点处理

```go
// ✓ 正确：只处理实际存在的节点
for _, sparse := range packet.SparseQuadtreeNode {
    processNode(sparse.GetIndex(), sparse.Node)
}

// ✗ 错误：假设所有索引0-3都存在
for i := 0; i < 4; i++ {
    // 某些索引可能不存在
}
```

### 2. 图层类型检查

```go
// ✓ 正确：使用枚举比较
if layer.GetType() == pb.QuadtreeLayer_LAYER_TYPE_IMAGERY {
    // 处理影像图层
}

// ✗ 错误：使用魔数
if layer.GetType() == 0 {
    // 代码可读性差
}
```

### 3. 日期格式处理

```go
// ✓ 正确：解析 YYYYMMDD 格式
date := datedTile.GetDate()
year := date / 10000
month := (date % 10000) / 100
day := date % 100

t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)

// ✗ 错误：直接当作时间戳使用
t := time.Unix(int64(date), 0) // 这是错误的！
```

### 4. 缓存失效策略

```go
func isCacheValid(cached *pb.QuadtreeNode, ttl time.Duration) bool {
    // 使用 cache_node_epoch 作为版本标识
    cacheTime := time.Unix(int64(cached.GetCacheNodeEpoch()), 0)
    return time.Since(cacheTime) < ttl
}
```

## 性能优化

### 1. 批量请求

```go
// 批量请求多个瓦片
func batchFetchTiles(tileKeys []string) ([]*pb.QuadtreePacket, error) {
    results := make([]*pb.QuadtreePacket, len(tileKeys))
    
    // 使用并发请求
    var wg sync.WaitGroup
    errChan := make(chan error, len(tileKeys))
    
    for i, key := range tileKeys {
        wg.Add(1)
        go func(idx int, tileKey string) {
            defer wg.Done()
            
            packet, err := fetchQuadtreePacket(tileKey)
            if err != nil {
                errChan <- err
                return
            }
            results[idx] = packet
        }(i, key)
    }
    
    wg.Wait()
    close(errChan)
    
    if len(errChan) > 0 {
        return nil, <-errChan
    }
    
    return results, nil
}
```

### 2. 内存优化

```go
// 只保留当前视口需要的瓦片
func (c *QuadtreeCache) PruneOutsideViewport(viewport Bounds) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    for key := range c.cache {
        if !viewport.Contains(key) {
            delete(c.cache, key)
        }
    }
}
```

## 常见错误

### 错误1: 忽略稀疏表示

```go
// ✗ 错误：假设有4个子节点
children := make([]*pb.QuadtreeNode, 4)
for _, sparse := range packet.SparseQuadtreeNode {
    children[sparse.GetIndex()] = sparse.Node // 可能索引越界！
}

// ✓ 正确：使用 map 处理稀疏数据
children := make(map[int32]*pb.QuadtreeNode)
for _, sparse := range packet.SparseQuadtreeNode {
    children[sparse.GetIndex()] = sparse.Node
}
```

### 错误2: 日期格式混淆

```go
// ✗ 错误：YYYYMMDD 不是 Unix 时间戳
timestamp := time.Unix(int64(datedTile.GetDate()), 0)

// ✓ 正确：手动解析日期
date := datedTile.GetDate()
year := date / 10000
month := (date % 10000) / 100
day := date % 100
t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
```

### 错误3: 版本比较错误

```go
// ✗ 错误：直接比较时间戳可能溢出
if layer1.GetLayerEpoch() - layer2.GetLayerEpoch() > 0 {
    // 可能溢出
}

// ✓ 正确：分别比较
if layer1.GetLayerEpoch() > layer2.GetLayerEpoch() {
    // 安全
}
```

## 参考资料

- [四叉树数据结构](https://en.wikipedia.org/wiki/Quadtree)
- [空间索引技术](https://en.wikipedia.org/wiki/Spatial_database)
- [Protocol Buffers Group 语法](https://protobuf.dev/programming-guides/proto2/#groups)

## 相关协议

- [RockTree 协议](./RockTree.md) - 节点级别的空间索引
- [DbRoot 协议](./dbroot.md) - 全局配置
- [影像流协议](./streaming_imagery.md) - 影像数据传输

---

**最后更新**: 2025-11-19  
**协议版本**: v1.0
