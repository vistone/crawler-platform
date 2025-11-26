# Store/sqlitedb.go

## 文件概要
- **定位**：为瓦片数据提供 SQLite 持久化实现，封装连接池、损坏恢复、表结构初始化以及 CRUD API。
- **适用场景**：当 `TileStorage` 选择 `BackendSQLite` 时，所有写入/读取都会委托到这里。

## 主要组件
### `SQLiteManager`
- 维护 `dbPath -> *sql.DB` 的连接池，串行化打开/关闭，避免重复打开同一文件。
- 默认 DSN：`file:<path>?_busy_timeout=2000&cache=shared&mode=rwc`，保证可写并在 2s 内等待锁。
- 连接配置：`SetConnMaxLifetime(0)`、`SetMaxOpenConns(1)`、`SetMaxIdleConns(1)`，确保 SQLite 以单连接方式运作。
- `getOrOpenDB`：在打开前调用 `sqliteRecoverIfNeeded`，若文件损坏会自动重命名为 `*.corrupt.<timestamp>` 并重建。

### `sqliteRecoverIfNeeded`
- 如果文件不存在：直接 `sql.Open`。
- 如果存在：先尝试 `Ping`；失败则备份原文件并重建。
- 这是当前 SQLite 层唯一的“自动修复”入口。

### `initSchema`
- 每次打开连接后执行，确保存在
  ```sql
  CREATE TABLE IF NOT EXISTS tiles (
      tile_id BLOB PRIMARY KEY,
      value   BLOB NOT NULL
  );
  ```
- **注意**：文档 `docs/design/storage-spec.md` 旧描述中的 `epoch/provider_id` 列目前尚未实现；当前表只有 `tile_id` 与 `value` 两列。

## 读写 API
| 函数 | 说明 |
|------|------|
| `PutTileSQLite` | 通过 `getDBPath` 定位文件 → 压缩 tilekey (`CompressTileKeyToUint64` + `encodeKeyBigEndian`) → `INSERT ... ON CONFLICT DO UPDATE`。 |
| `PutTilesSQLiteBatch` | 先按数据库路径分组，再对每个文件开启事务，预编译 UPSERT 语句逐条写入，适合批处理。 |
| `GetTileSQLite` | 同样压缩 tilekey 作为主键，通过 `QueryRow` 读取 `value`。 |
| `DeleteTileSQLite` | 根据主键删除记录。 |
| `CloseAllSQLite` | 转发给默认管理器，关闭所有打开的 `*sql.DB`。 |

## 交互关系
- `TileStorage` 的同步持久化和异步刷盘逻辑会调用这些 API。
- `getDBPath`（定义在 `Store/dbpath.go`）负责路径分片，本文件仅处理同一数据库文件内的 CRUD。
- `CompressTileKeyToUint64`/`encodeKeyBigEndian` 定义在 `Store/bblotdb.go`，SQLite 与 BBolt 共用同一压缩主键格式。

## 与文档的对齐点
- Redis 键、KV 头部等规范由其他文件处理，这里仅存储原始瓦片 `value`，没有额外头部或元数据列。
- 若后续需要落地 `epoch/provider` 等列，应在 `initSchema` 与读写 SQL 中同步添加，再更新本文档。当前版本反映的是“最小两列 schema”。

