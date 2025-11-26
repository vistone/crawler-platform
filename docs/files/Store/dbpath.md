# Store/dbpath.go

## 文件概要
- **定位**：统一的数据库路径分片策略，根据 `tilekey` 长度决定磁盘目录与文件名。
- **适用范围**：BBolt 与 SQLite 均依赖该函数来决定 `.g3db` 文件位置，确保两者的文件结构一致。

## 核心函数：`getDBPath`
| 输入 | 说明 |
|------|------|
| `dbdir` | 数据库根目录（`TileStorageConfig.DBDir`） |
| `dataType` | imagery / terrain / vector / … |
| `tilekey` | 四叉树字符串，长度 1~24 |

### 算法
1. 取 `tilekey` 前 4 位作为文件名前缀（不足 4 位则全取），文件名：`<prefix>.g3db`。
2. 根据长度分层：
   - `<=8`：`<dbdir>/<dataType>/base.g3db`
   - `9-12`：`<dbdir>/<dataType>/8/<prefix>.g3db`
   - `13-16`：`<dbdir>/<dataType>/12/<prefix>.g3db`
   - `>=17`：`<dbdir>/<dataType>/<length>/<prefix>.g3db`（每个层级单独目录）
3. 函数内部会调用 `os.MkdirAll` 确保目录存在。

### 测试辅助
- `GetDBPathForTest` 只是对外暴露 `getDBPath`，方便在 `test/store/sqlitedb_test.go` 等测试中验证路径策略。

## 交互关系
- `PutTileBBolt`, `PutTileSQLite` 等函数在写入前都会调用 `getDBPath`，因此路径策略变更只需改动此处。
- 路径策略决定的是“文件分片”，与主键压缩（`CompressTileKeyToUint64`）互不干扰：前者决定文件，后者保证文件内主键唯一。

## 注意事项
- 当前策略为经验值：0-8 层统一 `base.g3db`，9+ 层按目录划分，避免单文件过大。
- 如果将来需要支持更深层级（>24）或自定义分片，只需扩展该函数并同步更新文档即可。

