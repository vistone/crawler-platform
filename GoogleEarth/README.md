# GoogleEarth Proto 生成代码包

## 包说明

本目录包含从 Protocol Buffers 定义文件生成的 Go 语言代码，用于 Google Earth 数据格式的序列化和反序列化。

## 目录结构

```
GoogleEarth/
├── proto/              # Proto 源文件
│   ├── RockTree.proto
│   ├── dbroot.proto
│   ├── quadtreeset.proto
│   ├── diorama_streaming.proto
│   ├── streaming_imagery.proto
│   └── terrain.proto
├── pb/                 # 生成的 Go 代码
│   ├── RockTree.pb.go
│   ├── dbroot.pb.go
│   ├── quadtreeset.pb.go
│   ├── diorama_streaming.pb.go
│   ├── streaming_imagery.pb.go
│   └── terrain.pb.go
└── README.md           # 本文件
```

## 生成的代码文件

| 文件名 | 大小 | 行数 | 源 Proto | 说明 |
|--------|------|------|----------|------|
| RockTree.pb.go | ~126KB | 2,939 | RockTree.proto | 空间索引与节点数据 |
| dbroot.pb.go | ~371KB | 6,763 | dbroot.proto | 数据库根配置 |
| quadtreeset.pb.go | ~33KB | 821 | quadtreeset.proto | 四叉树数据集 |
| diorama_streaming.pb.go | ~51KB | 1,141 | diorama_streaming.proto | 全景流数据 |
| streaming_imagery.pb.go | ~13KB | 313 | streaming_imagery.proto | 影像流数据 |
| terrain.pb.go | ~22KB | 519 | terrain.proto | 地形数据 |

**总计**: ~600KB, 12,496 行代码

## 使用方法

### 1. 导入包

```go
import (
    pb "crawler-platform/GoogleEarth/pb"
)
```

### 2. 创建消息

```go
// 创建节点键值
nodeKey := &pb.NodeKey{
    Path:  proto.String("0123"),
    Epoch: proto.Uint32(12345),
}

// 创建四叉树节点
qtNode := &pb.QuadtreeNode{
    Flags:          proto.Int32(1 << 6), // IMAGE_BIT
    CacheNodeEpoch: proto.Int32(99999),
}
```

### 3. 序列化

```go
// 序列化为二进制
data, err := proto.Marshal(nodeKey)
if err != nil {
    log.Fatal(err)
}

// 序列化为 JSON（调试用）
import "google.golang.org/protobuf/encoding/protojson"
jsonData, err := protojson.Marshal(nodeKey)
```

### 4. 反序列化

```go
// 从二进制反序列化
nodeKey := &pb.NodeKey{}
err := proto.Unmarshal(data, nodeKey)

// 从 JSON 反序列化
err = protojson.Unmarshal(jsonData, nodeKey)
```

## 依赖要求

```go
require (
    google.golang.org/protobuf v1.36.10
)
```

## 重新生成代码

如果修改了 proto 文件，使用以下命令重新生成：

```bash
cd /home/stone/crawler-platform

# 生成所有文件
protoc --go_out=GoogleEarth/pb \
       --go_opt=paths=source_relative \
       --proto_path=GoogleEarth/proto \
       GoogleEarth/proto/*.proto

# 验证编译
cd GoogleEarth/pb && go build .
```

## 文档

详细的使用文档请参阅：

- [总览文档](../docs/googleearth/README.md)
- [RockTree 协议](../docs/googleearth/RockTree.md)
- [DbRoot 协议](../docs/googleearth/dbroot.md)
- [QuadTreeSet 协议](../docs/googleearth/quadtreeset.md)
- [Diorama 协议](../docs/googleearth/diorama_streaming.md)
- [Imagery 协议](../docs/googleearth/streaming_imagery.md)
- [Terrain 协议](../docs/googleearth/terrain.md)

## 版本信息

- **生成时间**: 2025-11-19
- **protoc 版本**: 3.21.12
- **protoc-gen-go 版本**: v1.36.5
- **Go 版本**: 1.25

## 注意事项

1. **不要手动修改生成的代码**：所有 `.pb.go` 文件都是自动生成的，手动修改将在下次重新生成时丢失。
2. **使用 Get 方法**：访问 optional 字段时使用 `GetXxx()` 方法而非直接访问指针。
3. **版本兼容性**：确保 protobuf 运行时库版本与生成代码的版本兼容。

## 许可证

本代码根据项目主许可证发布。
