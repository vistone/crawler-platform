# Store/tilestorage.go

## 文件概要
- **定位**：统一的瓦片存储入口，负责在 Redis 缓存与本地持久化（BBolt/SQLite）之间协调，支持同步模式、缓存模式和异步持久化。
- **外部接口**：`TileStorage` 提供 `Put/PutBatch/Get/Delete/Exists/WarmupCache/InvalidateCache/Close` 等方法。

## 配置结构：`TileStorageConfig`
| 字段 | 说明 |
|------|------|
| `Backend` | `BackendBBolt` 或 `BackendSQLite`，决定持久化后端。 |
| `DBDir` | 持久化根目录。 |
| `RedisAddr` | Redis 地址，启用缓存或异步模式时必需。 |
| `CacheExpiration` | 缓存过期时间（0 = 永不过期）。 |
| `EnableCache` | 是否使用 Redis 作为读缓存。 |
| `EnableAsyncPersist` | 是否先写 Redis、再异步刷盘。 |
| `PersistBatchSize` / `PersistInterval` | 异步刷盘的批次大小与刷新间隔。 |
| `ClearRedisAfterPersist` | 持久化成功后是否清理 Redis（指针区分“未设置”与“显式 false”）。 |

## 核心逻辑
### 初始化
1. 校验配置（后端类型、DBDir）。
2. 若启用缓存：自动填充默认 `RedisAddr`，并调用 `InitRedis` 验证连接。
3. 异步模式下：补齐默认的批次/间隔，默认 `ClearRedisAfterPersist = true`，并启动 `persistWorker`。

### `persistWorker`
- 使用带缓冲的 `persistQueue` 收集 `persistTask`（dataType + tilekey + value）。
- 每当达到批次或定时器到期时，按 dataType 分组：
  - BBolt：调用 `PutTilesBBoltBatch`。
  - SQLite：逐条调用 `PutTileSQLite`（当前未实现批量 SQL）。
- 成功后根据配置异步删除 Redis 缓存。

### 对外方法
| 方法 | 说明 |
|------|------|
| `Put` | 异步模式：写 Redis 并 enqueue；同步模式：先持久化再写缓存。 |
| `PutBatch` | 批量写入（持久化 → Redis）。 |
| `Get` | 缓存优先，再回源持久化并异步回填缓存。 |
| `Delete` | 删除缓存后删除持久化记录。 |
| `Exists` | 先查缓存，再尝试 `Get`。 |
| `InvalidateCache` | 强制删除缓存记录。 |
| `WarmupCache` | 读取持久化数据批量写入 Redis。 |
| `Close` | 关闭异步 worker → 关闭 Redis → 关闭持久化连接。 |
| `GetBackend` / `IsCacheEnabled` / `IsAsyncPersistEnabled` / `GetPendingPersistCount` | 状态查询辅助。 |

## 依赖关系
- BBolt/SQLite：通过 `PutTileBBolt`, `PutTilesBBoltBatch`, `PutTileSQLite` 等函数完成持久化。
- Redis：通过 `PutTileRedis`, `PutTilesRedisBatch`, `GetTileRedis`, `DeleteTileRedis` 等函数提供缓存能力。
- `dbpath.go`：用于决定具体 `.g3db` 文件位置，`TileStorage` 不直接处理路径。

## 注意事项
- 异步模式必须启用 Redis，否则 `Put` 会直接返回错误。
- SQLite 批量 API 目前仍逐条写入，如果后续需要真正的事务批量，可考虑扩展 `PutTilesSQLiteBatch` 并在此切换。
- `ClearRedisAfterPersist` 的默认行为是 `true`（指针非 nil），如需“保留缓存”需显式设置为 `false`。

