# streaming_imagery 协议详细文档

## 协议概述

### 基本信息
- **协议名称**: Streaming Imagery Protocol
- **proto 文件**: `GoogleEarth/proto/streaming_imagery.proto`
- **语法版本**: proto2
- **包名**: GoogleEarth
- **Go 包**: GoogleEarth
- **生成文件**: `GoogleEarth/pb/streaming_imagery.pb.go`

### 功能定位
Streaming Imagery 协议定义地球卫星影像、航拍图片的流式传输格式。它负责:
- 影像瓦片数据的编码和传输
- 多种图像格式支持(JPEG/PNG/DXT/JPEG2000)
- 主数据和 Alpha 通道分离传输
- 透明度处理和图像压缩优化

### 与其他协议的关系
Streaming Imagery 属于数据内容层,提供视觉底图:

```
dbroot (配置层)
    ↓
RockTree + QuadTreeSet (空间索引层)
    ↓
Imagery (影像底图) + Diorama (3D模型) + Terrain (地形)
```

## 数据结构详解

### 核心消息类型

#### 1. EarthImageryPacket - 地球影像数据包

**用途**: 封装单张地球影像瓦片的图像数据和元信息,是该协议的唯一核心消息。

**字段说明**:

| 字段名 | 类型 | 必填 | 默认值 | 说明 |
|--------|------|------|--------|------|
| image_type | Codec | optional | JPEG | 图像编码格式 |
| image_data | bytes | optional | - | 图像主数据字节流 |
| alpha_type | SeparateAlphaType | optional | NONE | 透明通道类型 |
| image_alpha | bytes | optional | - | 独立的 Alpha 通道数据 |

**Go 类型定义**:
```go
type EarthImageryPacket struct {
    ImageType  *EarthImageryPacket_Codec
    ImageData  []byte
    AlphaType  *EarthImageryPacket_SeparateAlphaType
    ImageAlpha []byte
}
```

#### 2. Codec - 图像编解码器枚举

**用途**: 定义支持的图像压缩格式。

**枚举值**:
```go
const (
    EarthImageryPacket_JPEG      = 0  // 标准 JPEG 压缩
    EarthImageryPacket_JPEG2000  = 1  // JPEG2000 小波压缩
    EarthImageryPacket_DXT1      = 2  // DXT1 压缩(无 Alpha)
    EarthImageryPacket_DXT5      = 3  // DXT5 压缩(带 Alpha)
    EarthImageryPacket_PNG_RGBA  = 4  // PNG 格式(RGBA 四通道)
)
```

**格式特点**:
- **JPEG**: 有损压缩,适合卫星影像,文件小,不支持透明
- **JPEG2000**: 小波压缩,质量更高,文件稍大
- **DXT1**: GPU 原生格式,不支持 Alpha,解码快
- **DXT5**: GPU 原生格式,支持 Alpha,适合混合瓦片
- **PNG_RGBA**: 无损压缩,支持完整 Alpha,文件大

#### 3. SeparateAlphaType - 独立 Alpha 通道类型

**用途**: 定义分离 Alpha 通道的编码格式。

**枚举值**:
```go
const (
    EarthImageryPacket_NONE       = 0  // 无独立 Alpha 通道
    EarthImageryPacket_PNG        = 1  // PNG 格式的 Alpha 通道
    EarthImageryPacket_JPEG_ALPHA = 2  // JPEG 编码的 Alpha 通道
    EarthImageryPacket_RLE_1_BIT  = 3  // 1 位 RLE 压缩的 Alpha 通道
)
```

**Alpha 通道用途**:
- 水陆边界的半透明混合
- 云层遮罩
- 地图标注的透明度
- 多图层叠加

## 使用场景说明

### 场景1: 基础影像瓦片传输

最常见的场景,传输标准 JPEG 影像:

```
服务器:
  1. 读取影像瓦片(如256x256 JPEG)
  2. 创建 EarthImageryPacket
  3. 设置 image_type = JPEG
  4. 填充 image_data
  5. 序列化并发送

客户端:
  1. 接收数据包
  2. 反序列化
  3. 解码 JPEG 数据
  4. 渲染到地球表面
```

### 场景2: 带透明度的海岸线瓦片

水陆混合区域需要 Alpha 通道:

```
服务器:
  1. RGB 主数据使用 JPEG 压缩
  2. Alpha 通道单独用 PNG 或 RLE 压缩
  3. 创建数据包:
     - image_type = JPEG
     - image_data = JPEG RGB 数据
     - alpha_type = PNG
     - image_alpha = PNG Alpha 数据
  4. 发送

客户端:
  1. 解码主图像(JPEG RGB)
  2. 解码 Alpha 通道(PNG)
  3. 合成 RGBA 图像
  4. 半透明混合渲染
```

### 场景3: GPU 优化的 DXT 纹理

移动设备或高性能渲染使用 DXT 格式:

```
优势:
  - GPU 原生格式,无需 CPU 解码
  - 内存占用小(6:1 或 4:1 压缩比)
  - 快速纹理上传和渲染

服务器:
  1. 预压缩为 DXT1 或 DXT5
  2. image_type = DXT1
  3. image_data = DXT 压缩数据
  4. 发送

客户端:
  1. 直接上传到 GPU 纹理
  2. 无需解码,立即渲染
```

## Go 代码使用示例

### 1. 导入包

```go
import (
    pb "your-project/GoogleEarth"
    "google.golang.org/protobuf/proto"
    "image/jpeg"
    "image/png"
    "os"
)
```

### 2. 创建标准 JPEG 影像包

```go
// 读取 JPEG 影像文件
jpegData, err := os.ReadFile("tile_256x256.jpg")
if err != nil {
    log.Fatalf("读取文件失败: %v", err)
}

// 创建影像数据包
packet := &pb.EarthImageryPacket{
    ImageType: pb.EarthImageryPacket_JPEG.Enum(),
    ImageData: jpegData,
    AlphaType: pb.EarthImageryPacket_NONE.Enum(),
}

// 序列化
data, err := proto.Marshal(packet)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

fmt.Printf("数据包大小: %d 字节\n", len(data))
```

### 3. 解析影像数据包

```go
// 从服务器接收的数据
packetBytes := []byte{...}

// 反序列化
packet := &pb.EarthImageryPacket{}
err := proto.Unmarshal(packetBytes, packet)
if err != nil {
    log.Fatalf("解析失败: %v", err)
}

// 检查图像类型
switch packet.GetImageType() {
case pb.EarthImageryPacket_JPEG:
    fmt.Println("图像格式: JPEG")
    // 解码 JPEG
    img, err := jpeg.Decode(bytes.NewReader(packet.GetImageData()))
    if err != nil {
        log.Fatalf("JPEG 解码失败: %v", err)
    }
    fmt.Printf("图像尺寸: %dx%d\n", img.Bounds().Dx(), img.Bounds().Dy())
    
case pb.EarthImageryPacket_PNG_RGBA:
    fmt.Println("图像格式: PNG")
    img, err := png.Decode(bytes.NewReader(packet.GetImageData()))
    if err != nil {
        log.Fatalf("PNG 解码失败: %v", err)
    }
    
case pb.EarthImageryPacket_DXT1, pb.EarthImageryPacket_DXT5:
    fmt.Println("图像格式: DXT (GPU 压缩)")
    // DXT 数据直接上传到 GPU
    // 不需要 CPU 解码
}
```

### 4. 创建带 Alpha 通道的影像包

```go
// 读取 RGB 主图像
rgbData, _ := os.ReadFile("tile_rgb.jpg")

// 读取 Alpha 通道(单独的PNG文件)
alphaData, _ := os.ReadFile("tile_alpha.png")

// 创建数据包
packet := &pb.EarthImageryPacket{
    ImageType:  pb.EarthImageryPacket_JPEG.Enum(),
    ImageData:  rgbData,
    AlphaType:  pb.EarthImageryPacket_PNG.Enum(),
    ImageAlpha: alphaData,
}

// 序列化
data, err := proto.Marshal(packet)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

fmt.Printf("主图像: %d 字节\n", len(rgbData))
fmt.Printf("Alpha通道: %d 字节\n", len(alphaData))
fmt.Printf("总计: %d 字节\n", len(data))
```

### 5. 合成 RGBA 图像

```go
// 解析数据包
packet := &pb.EarthImageryPacket{}
proto.Unmarshal(packetBytes, packet)

// 解码主图像
rgbImg, _ := jpeg.Decode(bytes.NewReader(packet.GetImageData()))

// 解码 Alpha 通道
var alphaImg image.Image
if packet.GetAlphaType() == pb.EarthImageryPacket_PNG {
    alphaImg, _ = png.Decode(bytes.NewReader(packet.GetImageAlpha()))
}

// 合成 RGBA
bounds := rgbImg.Bounds()
rgba := image.NewRGBA(bounds)

for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
    for x := bounds.Min.X; x < bounds.Max.X; x++ {
        // 获取 RGB
        r, g, b, _ := rgbImg.At(x, y).RGBA()
        
        // 获取 Alpha
        var a uint32
        if alphaImg != nil {
            _, _, _, a = alphaImg.At(x, y).RGBA()
        } else {
            a = 0xFFFF  // 完全不透明
        }
        
        // 设置 RGBA
        rgba.SetRGBA(x, y, color.RGBA{
            R: uint8(r >> 8),
            G: uint8(g >> 8),
            B: uint8(b >> 8),
            A: uint8(a >> 8),
        })
    }
}

// 保存合成结果
f, _ := os.Create("tile_rgba.png")
defer f.Close()
png.Encode(f, rgba)
```

### 6. 处理 RLE 压缩的 Alpha

```go
// 解码 1-bit RLE 压缩的 Alpha 通道
func decodeRLE1BitAlpha(rleData []byte, width, height int) []byte {
    alpha := make([]byte, width*height)
    pos := 0
    
    for i := 0; i < len(rleData); i++ {
        run := int(rleData[i] & 0x7F)   // 低7位:重复次数
        value := (rleData[i] & 0x80) != 0  // 高1位:0或1
        
        var alphaValue byte
        if value {
            alphaValue = 0xFF  // 不透明
        } else {
            alphaValue = 0x00  // 完全透明
        }
        
        for j := 0; j < run && pos < len(alpha); j++ {
            alpha[pos] = alphaValue
            pos++
        }
    }
    
    return alpha
}

// 使用示例
packet := &pb.EarthImageryPacket{}
proto.Unmarshal(packetBytes, packet)

if packet.GetAlphaType() == pb.EarthImageryPacket_RLE_1_BIT {
    width, height := 256, 256
    alphaData := decodeRLE1BitAlpha(packet.GetImageAlpha(), width, height)
    fmt.Printf("解码 Alpha: %d 字节\n", len(alphaData))
}
```

### 7. 格式选择逻辑

```go
// 根据场景选择合适的编码格式
func selectImageryFormat(hasAlpha bool, forGPU bool, qualityPriority bool) (
    imageCodec pb.EarthImageryPacket_Codec,
    alphaCodec pb.EarthImageryPacket_SeparateAlphaType,
) {
    // GPU优先模式
    if forGPU {
        if hasAlpha {
            return pb.EarthImageryPacket_DXT5, pb.EarthImageryPacket_NONE
        }
        return pb.EarthImageryPacket_DXT1, pb.EarthImageryPacket_NONE
    }
    
    // 质量优先模式
    if qualityPriority {
        if hasAlpha {
            return pb.EarthImageryPacket_PNG_RGBA, pb.EarthImageryPacket_NONE
        }
        return pb.EarthImageryPacket_JPEG2000, pb.EarthImageryPacket_NONE
    }
    
    // 默认:带宽优先
    if hasAlpha {
        return pb.EarthImageryPacket_JPEG, pb.EarthImageryPacket_PNG
    }
    return pb.EarthImageryPacket_JPEG, pb.EarthImageryPacket_NONE
}

// 使用示例
imageCodec, alphaCodec := selectImageryFormat(
    true,   // 有Alpha通道
    false,  // 不是GPU模式
    false,  // 带宽优先
)

packet := &pb.EarthImageryPacket{
    ImageType: imageCodec.Enum(),
    AlphaType: alphaCodec.Enum(),
}
```

### 8. 批量处理影像瓦片

```go
// 批量转换影像瓦片
func convertImageryTiles(inputDir, outputDir string) error {
    files, err := os.ReadDir(inputDir)
    if err != nil {
        return err
    }
    
    for _, file := range files {
        if !strings.HasSuffix(file.Name(), ".jpg") {
            continue
        }
        
        // 读取图像
        imgPath := filepath.Join(inputDir, file.Name())
        imgData, err := os.ReadFile(imgPath)
        if err != nil {
            log.Printf("读取失败 %s: %v", file.Name(), err)
            continue
        }
        
        // 创建数据包
        packet := &pb.EarthImageryPacket{
            ImageType: pb.EarthImageryPacket_JPEG.Enum(),
            ImageData: imgData,
            AlphaType: pb.EarthImageryPacket_NONE.Enum(),
        }
        
        // 序列化
        data, err := proto.Marshal(packet)
        if err != nil {
            log.Printf("序列化失败 %s: %v", file.Name(), err)
            continue
        }
        
        // 保存
        outPath := filepath.Join(outputDir, 
            strings.TrimSuffix(file.Name(), ".jpg") + ".pb")
        err = os.WriteFile(outPath, data, 0644)
        if err != nil {
            log.Printf("保存失败 %s: %v", file.Name(), err)
            continue
        }
        
        fmt.Printf("已转换: %s → %s (%d 字节)\n", 
            file.Name(), filepath.Base(outPath), len(data))
    }
    
    return nil
}
```

## 最佳实践建议

### 1. 字段填充建议

**必填字段**:
- image_type: 必须指定图像格式
- image_data: 必须包含图像数据

**可选但推荐**:
- alpha_type 和 image_alpha: 有透明需求时使用

### 2. 性能优化要点

**格式选择策略**:
```
移动设备 → DXT1/DXT5 (GPU原生,省内存)
桌面端高质量 → JPEG2000 (质量优先)
默认场景 → JPEG (兼容性好,带宽友好)
需要透明 → JPEG + PNG Alpha (分离存储,优化传输)
```

**带宽优化**:
- JPEG 质量设置为 75-85(视觉质量与文件大小平衡)
- Alpha 通道使用 RLE_1_BIT(海陆边界等简单场景)
- 批量传输多个瓦片时考虑外层压缩(gzip/brotli)

**内存优化**:
- 及时释放已解码的图像数据
- DXT 格式直接上传 GPU,不占用系统内存
- 使用图像缓存池,避免重复解码

### 3. 常见错误和解决方法

**错误1: 遗漏默认值设置**
```go
// ❌ 未设置编解码器
packet := &pb.EarthImageryPacket{
    ImageData: jpegData,
}
// 默认值 JPEG 会自动应用

// ✅ 显式设置(推荐)
packet := &pb.EarthImageryPacket{
    ImageType: pb.EarthImageryPacket_JPEG.Enum(),
    ImageData: jpegData,
    AlphaType: pb.EarthImageryPacket_NONE.Enum(),
}
```

**错误2: Alpha 通道尺寸不匹配**
```go
// ✅ 确保主图像和 Alpha 通道尺寸一致
func validateAlphaSize(rgbImg, alphaImg image.Image) error {
    if rgbImg.Bounds() != alphaImg.Bounds() {
        return fmt.Errorf("尺寸不匹配: RGB=%v, Alpha=%v",
            rgbImg.Bounds(), alphaImg.Bounds())
    }
    return nil
}
```

**错误3: DXT 数据直接解码**
```go
// ❌ 错误:尝试用标准库解码 DXT
img, _ := jpeg.Decode(bytes.NewReader(dxtData))  // 失败

// ✅ 正确:DXT 数据上传到 GPU
// OpenGL 示例
gl.CompressedTexImage2D(
    gl.TEXTURE_2D,
    0,
    gl.COMPRESSED_RGBA_S3TC_DXT5_EXT,
    width, height,
    0,
    dxtData,
)
```

### 4. 版本兼容性注意事项

- proto2 语法,所有字段都有默认值
- image_type 默认 JPEG,alpha_type 默认 NONE
- 添加新的编解码器枚举值不影响旧代码
- 旧客户端会忽略未知的编解码器类型

## 参考资料

### Protocol Buffers
- [Protocol Buffers Language Guide (proto2)](https://protobuf.dev/programming-guides/proto2/)

### 图像格式规范
- [JPEG Standard](https://jpeg.org/jpeg/)
- [JPEG2000](https://jpeg.org/jpeg2000/)
- [PNG Specification](http://www.libpng.org/pub/png/spec/)
- [S3 Texture Compression (DXT)](https://en.wikipedia.org/wiki/S3_Texture_Compression)

### Google Earth
- [Web Tiles API](https://developers.google.com/maps/documentation/tile)
- [Tile Coordinate System](https://developers.google.com/maps/documentation/javascript/coordinates)

### 图像处理库
- [Go image package](https://pkg.go.dev/image)
- [go-dxt (DXT encoder/decoder)](https://github.com/GalambosMedia/go-dxt)
