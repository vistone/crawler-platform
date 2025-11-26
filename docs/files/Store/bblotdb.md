# Store/bblotdb.go

## 文件概要
- **定位**：BBolt（bbolt）存储实现，负责连接池、损坏恢复、瓦片主键压缩与 CRUD。
- **适用场景**：`TileStorage` 使用 `BackendBBolt` 时，通过这里读写本地 `.g3db` 数据库。

## 主要组件
### `BBoltManager`
- 维护 `dbPath -> *bolt.DB` 的连接池，避免重复打开同一文件。
- 默认 `bolt.Options` 仅设置 `Timeout: 2s`，确保遇到独占锁时有限等待。
- `getOrOpenDB`：创建目录 → 调用 `bboltRecoverIfNeeded` → 缓存连接。
- `CloseAll`：关闭并清空池，用于 `TileStorage.Close()`。

### `bboltRecoverIfNeeded`
- 文件不存在：直接创建。
- 文件存在且打开失败：将旧文件重命名为 `*.corrupt.<timestamp>` 后重建，避免永久损坏。

### 主键工具
- `CompressTileKeyToUint64`：将四叉树 tilekey 压缩为 64bit，格式为“逐层 2bit + 低位记录层级”，支持 1~24 层。
- `encodeKeyBigEndian`：把 uint64 转成 8 字节大端，这就是 BBolt/SQLite 共用的主键。

## 读写 API
| 函数 | 说明 |
|------|------|
| `PutTileBBolt` | 通过 `getDBPath` 定位数据库 → 获取连接 → 以 `strings.ToLower(dataType)` 为桶名写入。 |
| `PutTilesBBoltBatch` | 先按文件分组，再在单个事务内遍历写入；适合大批量导入。 |
| `GetTileBBolt` | 打开桶并读取主键，返回一份拷贝（避免 bbolt 页复用问题）。 |
| `DeleteTileBBolt` | 在桶里删除主键，bucket 不存在时返回错误。 |
| `CloseAllBBolt` | 对外暴露的关闭接口，直接调用管理器。 |

## 交互关系
- `getDBPath`（`Store/dbpath.go`）负责决定哪一个 `.g3db` 文件，BBolt 层负责文件内的数据。
- `TileStorage` 在同步模式、批量模式或异步刷盘时会调 `PutTileBBolt` / `PutTilesBBoltBatch` 等函数。
- SQLite 复用了 `CompressTileKeyToUint64` / `encodeKeyBigEndian`，保证不同后端的主键可逆。

## 现状说明
- Bucket 名以数据类型（小写）区分。例如 imagery/terrain 分别使用独立 bucket。
- 当前实现只存 `value` 原始瓦片字节；epoch/provider 等元数据不在此层存储，与 `docs/design/storage-spec.md` 旧描述不一致，已在差异清单中记录。

