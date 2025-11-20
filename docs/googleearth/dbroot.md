# dbroot 协议详细文档

## 协议概述

### 基本信息
- **协议名称**: dbroot Protocol
- **proto 文件**: `GoogleEarth/proto/dbroot.proto`
- **语法版本**: proto3
- **包名**: GoogleEarth.dbroot
- **Go 包**: GoogleEarth
- **生成文件**: `GoogleEarth/pb/dbroot.pb.go`

### 功能定位
dbroot 协议是 Google Earth 的全局配置中心,定义了客户端的所有运行参数和数据库组织结构。它负责:
- 客户端渲染选项(大气层、星空、缓存等)
- 数据获取策略(QPS限制、最大请求数等)
- 图层树结构(嵌套特征、KML组织)
- 样式定义(颜色、图标、弹窗)
- 服务器URL配置(认证、搜索、街景等)
- 行星模型参数(半径、扁率、高程)
- 多语言翻译支持

### 与其他协议的关系
dbroot 在数据架构中处于配置层顶端:

```
dbroot (配置层)
  ├── 配置 → RockTree (空间索引)
  ├── 配置 → QuadTreeSet (四叉树)
  ├── 配置 → Diorama (全景)
  ├── 配置 → Imagery (影像)
  └── 配置 → Terrain (地形)
```

## 数据结构详解

### 核心消息类型

#### 1. DbRootProto - 数据库根协议

**用途**: Google Earth 数据库的根配置,包含所有全局设置和特征定义。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| database_name | string | 数据库名称(如"Earth") |
| imagery_present | bool | 是否存在影像数据 |
| proto_imagery | bool | 是否为协议影像 |
| terrain_present | bool | 是否存在地形数据 |
| provider_info | ProviderInfo[] | 数据提供商信息列表 |
| nested_feature | NestedFeatureProto[] | 嵌套特征列表(图层树) |
| style_attribute | StyleAttributeProto[] | 样式属性列表 |
| style_map | StyleMapProto[] | 样式映射列表 |
| end_snippet | EndSnippetProto | 结尾片段(核心配置集合) |
| translation_entry | TranslationEntryProto[] | 翻译条目(多语言) |
| language | string | 语言代码(如"zh-CN") |
| version | int32 | 版本号 |
| dbroot_reference | DbRootReferenceProto[] | 数据库根引用 |
| database_version | DatabaseVersionProto | 数据库版本信息 |
| refresh_timeout | int32 | 刷新超时时间(秒) |

#### 2. NestedFeatureProto - 嵌套特征协议

**用途**: 定义 Google Earth 的图层树结构,支持递归嵌套,是组织 KML 图层的核心。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| feature_type | FeatureType | 特征类型(点/面/线/地形) |
| kml_url | StringIdOrValueProto | KML URL |
| database_url | StringIdOrValueProto | 数据库URL |
| layer | LayerProto | 图层配置 |
| folder | FolderProto | 文件夹配置 |
| requirement | RequirementProto | 客户端要求 |
| channel_id | int32 | 通道ID |
| display_name | StringIdOrValueProto | 显示名称 |
| is_visible | bool | 是否可见 |
| is_enabled | bool | 是否启用 |
| is_checked | bool | 是否默认选中 |
| is_expandable | bool | 是否可展开 |
| layer_menu_icon_path | string | 图层菜单图标路径 |
| description | StringIdOrValueProto | 描述信息 |
| look_at | LookAtProto | 视角定义 |
| children | NestedFeatureProto[] | 子特征列表(递归) |
| client_config_script_name | string | 客户端配置脚本 |
| diorama_data_channel_base | int32 | 全景数据通道基址 |
| replica_data_channel_base | int32 | 副本数据通道基址 |

**特征类型枚举** (FeatureType):
```go
const (
    NestedFeatureProto_TYPE_POINT_Z     = 1  // 点要素(带高程)
    NestedFeatureProto_TYPE_POLYGON_Z   = 2  // 面要素(带高程)
    NestedFeatureProto_TYPE_LINE_Z      = 3  // 线要素(带高程)
    NestedFeatureProto_TYPE_TERRAIN     = 4  // 地形要素
)
```

#### 3. EndSnippetProto - 结尾片段协议

**用途**: 包含 Google Earth 客户端的所有核心配置选项和服务器URL,是配置的集大成者。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| model | PlanetModelProto | 行星模型(半径、扁率) |
| authentication_server | StringIdOrValueProto | 认证服务器URL |
| disable_authentication | bool | 禁用认证 |
| client_options | ClientOptionsProto | 客户端选项 |
| fetching_options | FetchingOptionsProto | 获取选项 |
| time_machine_options | TimeMachineOptionsProto | 时光机选项 |
| csi_options | CsiOptionsProto | CSI选项 |
| search_tab | SearchTabProto[] | 搜索标签页 |
| cobrand_info | CobrandProto[] | 品牌信息 |
| valid_database | DatabaseDescProto[] | 有效数据库 |
| config_script | ConfigScriptProto[] | 配置脚本 |
| deauth_server | StringIdOrValueProto | 注销服务器URL |
| autopia_options | AutopiaOptionsProto | Autopia(街景)选项 |
| search_config | SearchConfigProto | 搜索配置 |
| search_info | SearchInfoProto | 搜索信息 |
| elevation_service_base_url | string | 高程服务基础URL |
| elevation_service_query_path | string | 高程服务查询路径 |
| pro_upgrade_url | StringIdOrValueProto | 专业版升级URL |
| earth_intl_url | StringIdOrValueProto | Earth国际化URL |
| marketing_url | StringIdOrValueProto | 营销URL |
| tutorial_url | StringIdOrValueProto | 教程URL |
| keyboard_shortcuts_url | StringIdOrValueProto | 键盘快捷键URL |
| release_notes_url | StringIdOrValueProto | 发布说明URL |
| hide_user_data | bool | 隐藏用户数据 |
| user_guide_url | StringIdOrValueProto | 用户指南URL |
| support_center_url | StringIdOrValueProto | 支持中心URL |
| business_listing_url | StringIdOrValueProto | 商业列表URL |
| support_answer_url | StringIdOrValueProto | 支持答案URL |
| support_topic_url | StringIdOrValueProto | 支持主题URL |
| support_request_url | StringIdOrValueProto | 支持请求URL |
| earth_community_url | StringIdOrValueProto | Earth社区URL |
| earth_plugin_url | StringIdOrValueProto | Earth插件URL |
| add_content_url | StringIdOrValueProto | 添加内容URL |
| sketchup_not_installed_url | StringIdOrValueProto | SketchUp未安装URL |
| sketchup_error_url | StringIdOrValueProto | SketchUp错误URL |
| free_license_url | StringIdOrValueProto | 免费许可URL |
| pro_license_url | StringIdOrValueProto | 专业许可URL |
| hide_explore_view | bool | 隐藏探索视图 |
| timed_texture_url | string | 定时纹理URL |
| privacy_policy_url | StringIdOrValueProto | 隐私政策URL |
| do_ge_plus_upgrade | bool | 执行GE Plus升级 |
| rocktree_data_proto | string | RockTree数据协议版本 |
| filmstrip_config | FilmstripConfigProto[] | 胶片条配置 |
| show_signin_button | bool | 显示登录按钮 |
| pro_upsell_url | StringIdOrValueProto | 专业版推销URL |
| earth_plugin_download_url | StringIdOrValueProto | 插件下载URL |
| feedback_url | StringIdOrValueProto | 反馈URL |
| hangout_meet_url | StringIdOrValueProto | Hangout会议URL |

#### 4. ClientOptionsProto - 客户端选项协议

**用途**: 配置客户端的渲染行为、性能参数和功能开关。

**重要字段**:

| 字段名 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| disable_disk_cache | bool | false | 禁用磁盘缓存 |
| disable_embedded_browser_vista | bool | false | 禁用内嵌浏览器 |
| draw_atmosphere | bool | true | 绘制大气层 |
| draw_stars | bool | true | 绘制星空 |
| shading_enabled | bool | true | 启用阴影 |
| throttle_imagery_fetches | bool | true | 限制影像获取 |
| use_protobuf_quadtree_packets | bool | false | 使用protobuf四叉树包 |
| use_extended_copyright_ids | bool | false | 使用扩展版权ID |
| polar_tile_merging_level | int32 | 0 | 极地瓦片合并级别 |
| internal_debug_build_version | string | "" | 内部调试版本 |
| precipitations_options | PrecipitationsOptionsProto | - | 降水效果选项 |
| capture_options | CaptureOptionsProto | - | 捕获选项 |
| show_2d_maps_icon | bool | true | 显示2D地图图标 |
| disable_internal_browser | bool | false | 禁用内部浏览器 |
| internal_logging_enabled | bool | false | 启用内部日志 |
| maps_options | MapsOptionsProto | - | 地图选项 |

#### 5. FetchingOptionsProto - 获取选项协议

**用途**: 控制数据获取的性能限制和并发策略。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| max_requests_per_query | int32 | 每次查询最大请求数 |
| max_drawable | int32 | 最大可绘制对象数 |
| max_imagery | int32 | 最大影像数 |
| max_terrain | int32 | 最大地形数 |
| max_quadtree | int32 | 最大四叉树数 |
| max_diorama_metadata | int32 | 最大全景元数据数 |
| max_diorama_data | int32 | 最大全景数据数 |
| safe_overall_qps | float | 安全总体QPS(每秒请求数) |
| safe_imagery_qps | float | 安全影像QPS |
| max_num_geocode_request | int32 | 最大地理编码请求数 |
| domains_for_https | string | HTTPS域名列表 |
| hosts_for_http | string | HTTP主机列表 |

#### 6. StyleAttributeProto - 样式属性协议

**用途**: 定义 KML 特征的视觉样式,包括颜色、线宽、图标等。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| style_id | string | 样式ID(唯一标识) |
| provider_id | int32 | 提供商ID |
| poly_color_abgr | uint32 | 多边形颜色(ABGR格式) |
| line_color_abgr | uint32 | 线条颜色(ABGR格式) |
| line_width | float | 线条宽度 |
| label_color_abgr | uint32 | 标签颜色(ABGR格式) |
| label_scale | float | 标签缩放比例 |
| placemark_icon_color_abgr | uint32 | 地标图标颜色 |
| placemark_icon_scale | float | 地标图标缩放 |
| placemark_icon_path | StringIdOrValueProto | 地标图标路径 |
| placemark_icon_x | float | 地标图标X偏移 |
| placemark_icon_y | float | 地标图标Y偏移 |
| placemark_icon_width | float | 地标图标宽度 |
| placemark_icon_height | float | 地标图标高度 |
| pop_up | PopUpProto | 弹出窗口配置 |
| draw_flag | DrawFlagProto[] | 绘制标志列表 |

#### 7. PlanetModelProto - 行星模型协议

**用途**: 定义地球的物理参数,用于坐标转换和投影计算。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| radius | double | 地球半径(米,默认6378137.0) |
| flattening | double | 地球扁率(默认1/298.257223563) |
| elevation_bias | double | 高程偏差(米) |
| negative_altitude_exponent_bias | float | 负高度指数偏差 |
| compressed_negative_altitude_threshold | float | 压缩负高度阈值 |

#### 8. EncryptedDbRootProto - 加密数据库根协议

**用途**: 支持加密传输的 dbroot 数据。

**字段说明**:

| 字段名 | 类型 | 说明 |
|--------|------|------|
| encryption_type | EncryptionType | 加密类型 |
| encryption_data | bytes | 加密数据(密钥等) |
| dbroot_data | bytes | 加密后的数据库根数据 |

**加密类型枚举**:
```go
const (
    EncryptedDbRootProto_ENCRYPTION_XOR = 0  // XOR加密
)
```

## 使用场景说明

### 场景1: 初始化 Google Earth 客户端

客户端启动时的配置加载流程:

```
1. 从服务器下载 dbroot 数据
   ↓
2. 解析 DbRootProto
   ↓
3. 应用 ClientOptions (渲染设置)
   ↓
4. 应用 FetchingOptions (性能限制)
   ↓
5. 加载 NestedFeature (构建图层树)
   ↓
6. 解析 PlanetModel (坐标系统)
   ↓
7. 缓存 StyleAttribute (样式定义)
   ↓
8. 客户端就绪
```

### 场景2: 构建图层树

通过递归遍历 NestedFeature 构建 UI 图层树:

```
根特征
├── 图层: 边界和地名
│   ├── 子特征: 国际边界
│   └── 子特征: 地名标签
├── 图层: 兴趣点
│   ├── 子特征: 餐厅
│   └── 子特征: 酒店
└── 图层: 3D建筑
    └── 子特征: 建筑模型
```

### 场景3: QPS 控制

根据 FetchingOptions 限制请求速率:

```go
safeQPS := fetchingOptions.GetSafeOverallQps() // 如 10.0
interval := time.Second / time.Duration(safeQPS) // 100ms

// 每次请求前检查
ticker := time.NewTicker(interval)
defer ticker.Stop()

for range ticker.C {
    // 发送请求
}
```

## Go 代码使用示例

### 1. 导入包

```go
import (
    pb "your-project/GoogleEarth"
    "google.golang.org/protobuf/proto"
    "os"
)
```

### 2. 解析 dbroot 文件

```go
// 从文件读取 dbroot 数据
data, err := os.ReadFile("dbroot.pb")
if err != nil {
    log.Fatalf("读取文件失败: %v", err)
}

// 反序列化
dbroot := &pb.DbRootProto{}
err = proto.Unmarshal(data, dbroot)
if err != nil {
    log.Fatalf("解析 dbroot 失败: %v", err)
}

// 访问基本信息
fmt.Printf("数据库名称: %s\n", dbroot.GetDatabaseName())
fmt.Printf("有影像数据: %v\n", dbroot.GetImageryPresent())
fmt.Printf("有地形数据: %v\n", dbroot.GetTerrainPresent())
fmt.Printf("版本: %d\n", dbroot.GetVersion())
```

### 3. 遍历图层树

```go
// 递归遍历特征树
func traverseFeatures(features []*pb.NestedFeatureProto, level int) {
    indent := strings.Repeat("  ", level)
    
    for _, feature := range features {
        // 获取显示名称
        name := feature.GetDisplayName().GetValue()
        if name == "" {
            name = "<无名称>"
        }
        
        // 打印特征信息
        fmt.Printf("%s- %s (可见:%v, 启用:%v, 选中:%v)\n",
            indent, name,
            feature.GetIsVisible(),
            feature.GetIsEnabled(),
            feature.GetIsChecked())
        
        // 递归处理子特征
        if len(feature.GetChildren()) > 0 {
            traverseFeatures(feature.GetChildren(), level+1)
        }
    }
}

// 使用示例
fmt.Println("图层树结构:")
traverseFeatures(dbroot.GetNestedFeature(), 0)
```

### 4. 读取客户端选项

```go
// 获取客户端选项
clientOpts := dbroot.GetEndSnippet().GetClientOptions()

// 检查渲染选项
if clientOpts.GetDrawAtmosphere() {
    fmt.Println("渲染大气层效果")
}

if clientOpts.GetDrawStars() {
    fmt.Println("渲染星空背景")
}

// 检查四叉树包格式
if clientOpts.GetUseProtobufQuadtreePackets() {
    fmt.Println("使用 Protobuf 四叉树包")
} else {
    fmt.Println("使用旧格式四叉树包")
}

// 获取极地瓦片合并级别
level := clientOpts.GetPolarTileMergingLevel()
fmt.Printf("极地瓦片合并级别: %d\n", level)
```

### 5. 应用获取选项

```go
// 获取数据获取限制
fetchOpts := dbroot.GetEndSnippet().GetFetchingOptions()

// 创建请求限制器
type RequestLimiter struct {
    MaxImagery  int32
    MaxTerrain  int32
    MaxQuadtree int32
    SafeQPS     float32
}

limiter := &RequestLimiter{
    MaxImagery:  fetchOpts.GetMaxImagery(),
    MaxTerrain:  fetchOpts.GetMaxTerrain(),
    MaxQuadtree: fetchOpts.GetMaxQuadtree(),
    SafeQPS:     fetchOpts.GetSafeOverallQps(),
}

fmt.Printf("请求限制:\n")
fmt.Printf("  最大影像数: %d\n", limiter.MaxImagery)
fmt.Printf("  最大地形数: %d\n", limiter.MaxTerrain)
fmt.Printf("  最大四叉树数: %d\n", limiter.MaxQuadtree)
fmt.Printf("  安全QPS: %.2f\n", limiter.SafeQPS)
```

### 6. 解析行星模型

```go
// 获取行星模型
planetModel := dbroot.GetEndSnippet().GetModel()

// 提取地球参数
type EarthModel struct {
    Radius            float64
    Flattening        float64
    ElevationBias     float64
    SemiMinorAxis     float64  // 计算得出
    Eccentricity      float64  // 计算得出
}

model := &EarthModel{
    Radius:        planetModel.GetRadius(),
    Flattening:    planetModel.GetFlattening(),
    ElevationBias: planetModel.GetElevationBias(),
}

// 计算半短轴
model.SemiMinorAxis = model.Radius * (1 - model.Flattening)

// 计算离心率
e2 := 2*model.Flattening - model.Flattening*model.Flattening
model.Eccentricity = math.Sqrt(e2)

fmt.Printf("地球模型参数:\n")
fmt.Printf("  半径: %.2f m\n", model.Radius)
fmt.Printf("  扁率: %.10f\n", model.Flattening)
fmt.Printf("  半短轴: %.2f m\n", model.SemiMinorAxis)
fmt.Printf("  离心率: %.10f\n", model.Eccentricity)
```

### 7. 查找样式定义

```go
// 构建样式映射
styleMap := make(map[string]*pb.StyleAttributeProto)
for _, style := range dbroot.GetStyleAttribute() {
    styleMap[style.GetStyleId()] = style
}

// 查找特定样式
styleID := "default_point"
if style, ok := styleMap[styleID]; ok {
    fmt.Printf("样式 %s:\n", styleID)
    
    // 解析ABGR颜色
    polyColor := style.GetPolyColorAbgr()
    a := (polyColor >> 24) & 0xFF
    b := (polyColor >> 16) & 0xFF
    g := (polyColor >> 8) & 0xFF
    r := polyColor & 0xFF
    
    fmt.Printf("  多边形颜色: RGBA(%d, %d, %d, %d)\n", r, g, b, a)
    fmt.Printf("  线宽: %.1f\n", style.GetLineWidth())
    fmt.Printf("  标签缩放: %.2f\n", style.GetLabelScale())
}
```

### 8. 创建简单的 dbroot

```go
// 创建最小的 dbroot 配置
dbroot := &pb.DbRootProto{
    DatabaseName:    proto.String("MyEarth"),
    ImageryPresent:  proto.Bool(true),
    TerrainPresent:  proto.Bool(true),
    Version:         proto.Int32(1),
}

// 设置行星模型
dbroot.EndSnippet = &pb.EndSnippetProto{
    Model: &pb.PlanetModelProto{
        Radius:     proto.Float64(6378137.0),  // WGS84 半径
        Flattening: proto.Float64(1.0 / 298.257223563),  // WGS84 扁率
    },
}

// 设置客户端选项
dbroot.EndSnippet.ClientOptions = &pb.ClientOptionsProto{
    DrawAtmosphere: proto.Bool(true),
    DrawStars:      proto.Bool(true),
}

// 设置获取选项
dbroot.EndSnippet.FetchingOptions = &pb.FetchingOptionsProto{
    MaxImagery:      proto.Int32(10),
    MaxTerrain:      proto.Int32(10),
    SafeOverallQps:  proto.Float32(5.0),
}

// 序列化
data, err := proto.Marshal(dbroot)
if err != nil {
    log.Fatalf("序列化失败: %v", err)
}

// 保存到文件
err = os.WriteFile("custom_dbroot.pb", data, 0644)
if err != nil {
    log.Fatalf("保存文件失败: %v", err)
}
fmt.Printf("已保存 dbroot (%d 字节)\n", len(data))
```

### 9. 处理加密的 dbroot

```go
// 解密 XOR 加密的 dbroot
func decryptXOR(encData []byte, key []byte) []byte {
    result := make([]byte, len(encData))
    keyLen := len(key)
    
    for i := 0; i < len(encData); i++ {
        result[i] = encData[i] ^ key[i%keyLen]
    }
    
    return result
}

// 解析加密的 dbroot
encDbroot := &pb.EncryptedDbRootProto{}
err := proto.Unmarshal(encData, encDbroot)
if err != nil {
    log.Fatalf("解析失败: %v", err)
}

// 检查加密类型
if encDbroot.GetEncryptionType() == pb.EncryptedDbRootProto_ENCRYPTION_XOR {
    // 解密
    decrypted := decryptXOR(
        encDbroot.GetDbrootData(),
        encDbroot.GetEncryptionData(),
    )
    
    // 解析 dbroot
    dbroot := &pb.DbRootProto{}
    err = proto.Unmarshal(decrypted, dbroot)
    if err != nil {
        log.Fatalf("解析 dbroot 失败: %v", err)
    }
    
    fmt.Printf("成功解密 dbroot: %s\n", dbroot.GetDatabaseName())
}
```

## 最佳实践建议

### 1. 字段填充建议

**必须填充的核心字段**:
- database_name: 数据库标识
- imagery_present/terrain_present: 数据可用性标识
- end_snippet.model: 行星模型(坐标转换必需)
- end_snippet.client_options: 客户端渲染配置
- end_snippet.fetching_options: 性能限制配置

**图层树组织**:
- nested_feature: 至少包含根级图层
- display_name: 每个特征都应有名称
- is_visible/is_enabled: 明确指定可见性

### 2. 性能优化要点

**获取选项调优**:
- 根据网络带宽设置合理的 QPS 限制
- 限制并发请求数避免过载
- 使用 max_imagery/max_terrain 控制内存使用

**缓存策略**:
- dbroot 数据变化不频繁,可以长期缓存
- 使用 refresh_timeout 控制缓存刷新
- version 字段用于缓存失效判断

**图层树优化**:
- 默认折叠深层嵌套的文件夹
- 延迟加载未可见图层的 KML 数据
- 使用 is_checked 控制默认加载内容

### 3. 常见错误和解决方法

**错误1: proto3 字段访问**
```go
// proto3 中基本类型字段没有指针,直接访问
// ❌ 错误
name := *dbroot.DatabaseName  // proto3 不是指针

// ✅ 正确
name := dbroot.GetDatabaseName()  // 使用 Get 方法
```

**错误2: 递归遍历栈溢出**
```go
// ✅ 添加深度限制
func traverseFeatures(features []*pb.NestedFeatureProto, level int) {
    if level > 20 {  // 限制最大深度
        log.Printf("警告: 图层树深度超过限制")
        return
    }
    // ... 正常处理
}
```

**错误3: ABGR 颜色转换错误**
```go
// ABGR 格式: 0xAABBGGRR
// ✅ 正确的转换
func abgrToRGBA(abgr uint32) (r, g, b, a uint8) {
    a = uint8((abgr >> 24) & 0xFF)
    b = uint8((abgr >> 16) & 0xFF)
    g = uint8((abgr >> 8) & 0xFF)
    r = uint8(abgr & 0xFF)
    return
}

// ✅ RGBA 到 ABGR
func rgbaToABGR(r, g, b, a uint8) uint32 {
    return (uint32(a) << 24) | (uint32(b) << 16) | (uint32(g) << 8) | uint32(r)
}
```

### 4. 版本兼容性注意事项

- **proto3 语法**: 所有字段都有默认值,未设置的字段不会出现在序列化数据中
- **零值语义**: 空字符串、0、false 不会被序列化
- **向后兼容**: 添加新字段安全,旧客户端会忽略
- **枚举默认值**: 枚举的第一个值(通常是 0)应该是最安全的默认值

## 参考资料

### Protocol Buffers 文档
- [Protocol Buffers Language Guide (proto3)](https://protobuf.dev/programming-guides/proto3/)
- [Style Guide](https://protobuf.dev/programming-guides/style/)
- [Encoding](https://protobuf.dev/programming-guides/encoding/)

### Google Earth 技术
- [Google Earth Pro](https://www.google.com/earth/versions/)
- [KML Tutorial](https://developers.google.com/kml/documentation/kml_tut)
- [WGS84 坐标系统](https://en.wikipedia.org/wiki/World_Geodetic_System)

### 相关标准
- [ISO 19111 (坐标参考系统)](https://www.iso.org/standard/74039.html)
- [EPSG:4326 (WGS84经纬度)](https://epsg.io/4326)
