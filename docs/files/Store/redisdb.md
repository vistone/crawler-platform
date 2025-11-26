# Store/redisdb.go

## 文件概要
- **定位**：封装 Redis 连接管理、分库策略与瓦片级 KV 操作，是缓存/异步持久化的基础。
- **使用者**：`TileStorage`、测试代码以及任何需要 Redis 缓存的模块。

## 主要组件
### `RedisManager`
- 维护 `dataType -> *redis.Client`，并记录每个数据类型映射到的 Redis DB（0~15）。
- `getOrInitClientForDataType` 逻辑：
  1. 若地址变更，关闭旧客户端并清空映射。
  2. 调用 `findSafeRedisDBs` 扫描所有 DBSize，按 key 数量从少到多分配。
  3. 创建新客户端并 `Ping` 验证。
- 通过 `CloseRedis` 可关闭所有客户端（`TileStorage.Close()` 会调用）。

### 键格式
- `buildRedisKey(dataType, tilekey)` 返回 `<dataType>:<tilekey>`。
- **注意**：这与 `docs/design/storage-spec.md` 旧描述（“不加前缀”）不符，目前实现确实带前缀，已在差异清单中记录并会同步更新文档。

### 读写 API
| 函数 | 说明 |
|------|------|
| `PutTileRedis` / `GetTileRedis` / `DeleteTileRedis` | 基本 CRUD，支持过期时间。 |
| `PutTilesRedisBatch` | 使用 Pipeline 分批写入，默认每批 1000 条，支持大规模缓存预热。 |
| `ExistsTileRedis` / `GetTileTTLRedis` | Key 存在性与 TTL 查询。 |
| `ScanAllRedisDBs` | 调试用，遍历 0~15 号 DB 的 key 数量。 |
| `SetEpoch` / `GetEpoch` / `SetMetadata` / `GetMetadata` | 利用 `dataType:_meta` hash 存储元数据（epoch、版本信息等）。 |

## 支撑工具
- `findSafeRedisDB` / `findSafeRedisDBs`：扫描 16 个数据库，优先选择 key 最少的 DB，实现自动分库。
- `InitRedis`：快速测试连接（已被 `TileStorage` 初始化缓存时调用）。

## 交互关系
- `TileStorage` 在启用缓存或异步持久化时，会调用 `PutTileRedis`/`PutTilesRedisBatch` 等函数。
- `TileStorage.WarmupCache` / `InvalidateCache` / `Exists` 等高层 API 直接复用这里的能力。
- 其他模块（例如测试工具）也可以单独使用 `SetEpoch`, `GetMetadata` 等函数管理 Redis 中的额外信息。

## 当前实现与文档差异
- 键名：带 `dataType` 前缀（`imagery:01230123`）。若需要无前缀方案，需改动 `buildRedisKey` 并更新所有调用。
- KV 结构：只存原始 `value`，没有 `epoch/provider` 头部（由上层自行处理或暂未实现）。与旧文档的“极简头部”描述不符，已在 `docs/files/status.md` 记录。

