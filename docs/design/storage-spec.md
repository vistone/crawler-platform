# 存储架构与键值编码规范（KV + SQLite + tm）

本文档总结并固化当前讨论形成的统一存储规范，用于指导 Redis（KV 缓存）、BBolt（KV 持久化）、SQLite（列式持久化）三者的一致性实现与互操作。内容围绕以下核心：
- 键（Key）格式与生成时机
- 值（Value）头部极简编码（epoch、provider_id）
- 路径分片与压缩键的协同
- tm（带时间）数据类型的复合键设计
- 兼容与空间开销考量

---

## 1. 总体原则（当前实现）

- Redis 键格式：`<dataType>:<tilekey>`（tm：`<dataType>:<tilekey>:<milliseconds>`）。
- BBolt / SQLite 主键一致：`compressTileKeyToUint64` → `encodeKeyBigEndian` 得到 8 字节大端主键。
- 路径分片使用原始 tilekey，由 `getDBPath` 按长度区间决定目录/文件。
- 值存储：目前直接写入瓦片的二进制 `value`，没有额外的 epoch/provider 头部。
- SQLite 表结构：`tiles(tile_id BLOB PRIMARY KEY, value BLOB NOT NULL)`，尚未持久化 epoch/provider。

---

## 2. 键（Key）规范

### 2.1 Redis（分库 + 键前缀）
- 普通瓦片（imagery/terrain/vector/q2/qp）：
  - 键：`<dataType>:<tilekey>`，如 `"imagery:01230123"`
- tm（带时间瓦片）：
  - 键：`<dataType>:<tilekey>:<milliseconds>`，如 `"imagery:01230123:1732612345678"`
- 说明：虽然 Redis 已按类型分库，代码仍保留前缀以便排查；如需切换为“纯 tilekey”方案，需同步修改 `Store/redisdb.go` 与所有调用。

### 2.2 BBolt/SQLite（统一主键）
- 主键：压缩后的 tileID（8 字节大端 BLOB），来自：
  - `compressTileKeyToUint64` → `encodeKeyBigEndian`
- 含层级位的压缩键是必要的：保证同一数据库文件内混合层级时的主键唯一与可逆解码。

---

## 3. 值（Value）存储策略

- **当前实现**：Redis、BBolt、SQLite 均直接存储瓦片的原始字节数组，不包含额外头部。
- **元数据**：epoch/provider 等信息仍由 dbroot 或外部元数据源维护，未写入瓦片值本身。
- **扩展思路**：若未来要引入头部（例如 Uvarint epoch/provider），需要同步调整
  - Redis 写入/读取：`Store/redisdb.go`
  - BBolt / SQLite CRUD：`Store/bblotdb.go`、`Store/sqlitedb.go`
  - 相关缓存与测试：`TileStorage` 及 `test/store`。

---

## 4. 路径分片与压缩键协同

- 路径分片：始终使用原始 tilekey → `getDBPath`（按长度区间/前缀命名规则）：
  - 0–8 层：`/dataType/base.g3db`
  - 9–12 层：`/dataType/8/{prefix4}.g3db`
  - 13–16 层：`/dataType/12/{prefix4}.g3db`
  - 17+ 层：`/dataType/{level}/{prefix4}.g3db`
- 压缩键：在持久化阶段，将原始 tilekey 压缩为 uint64（包含层级位），再编码为 8 字节大端作为主键。
- 二者职责分离：
  - 分片选择文件 → 原始 tilekey
  - 同文件主键唯一性与可逆解码 → 压缩 tileID（含层级）

---

## 5. tm（带时间）数据类型

当前代码尚未实现 tm 专用的键或表结构。若未来要为 tm 引入独立存储，可参考下列方向：

1. **Redis**：键扩展为 `<dataType>:<tilekey>:<milliseconds>`，值仍直接存瓦片数据。
2. **BBolt**：设计 16 字节复合主键（tileID + 时间戳），并按数据类型划分 bucket。
3. **SQLite**：新增 `PRIMARY KEY(tile_id, t)` 的表，并根据需要建立 epoch/provider/t 索引。

在上述改动落地前，本节仅作为规划说明。

---

## 6. provider 存储与解释（当前状态）

- 当前实现**没有**把 provider 信息写入 Redis/BBolt/SQLite，所有持久化值都是原始瓦片数据。
- 若业务需要展示 provider、版权等信息，仍依赖 dbroot 或外部映射。
- 如需在存储层记录 provider，可在未来扩展 Value 头部或 SQLite 列结构，并同步修改 `Store/*` 相关代码。

---

## 7. 兼容性与容错

- 由于当前值没有头部，兼容逻辑主要集中在“文件损坏自动备份 + 重建”：
  - BBolt：`bboltRecoverIfNeeded` 会将损坏文件重命名为 `*.corrupt.<timestamp>` 后重建。
  - SQLite：`sqliteRecoverIfNeeded` 同理。
- Redis 连接会在地址变更时自动刷新客户端并重新分配数据库编号。
- 如未来引入头部或 tm 结构，需要新增相应的容错/回退机制。

---

## 8. 读写流程（简图）

- 写入（异步持久化开启时）：
  1. 下载数据 → Redis 写入：键=`<dataType>:<tilekey>`，值=原始 data。
  2. 后台 Worker 批量持久化：
     - 路径分片：`getDBPath`（原始 tilekey）
     - 主键生成：`compressTileKeyToUint64` → `encodeKeyBigEndian`
     - BBolt：按 bucket（dataType）写入 8B 主键。
     - SQLite：执行 `INSERT ... ON CONFLICT DO UPDATE`，表结构只有 `tile_id/value`。
  3. 可选：持久化成功后按配置删除 Redis。

- 读取：
  - 缓存命中：`GetTileRedis` 直接返回 value。
  - 缓存未命中：按后端查询（BBolt/SQLite）→ 返回数据并异步回填缓存。

---

## 9. 示例对照

- Redis 键：
  - imagery 层：`imagery:01230123`
  - tm 规划：`imagery:01230123:1732612345678`（尚未落地）
- BBolt / SQLite 主键：
  - `tilekey = "01230123"` → `CompressTileKeyToUint64` → 8B 大端 (`encodeKeyBigEndian`)
- SQLite 表结构：
  ```sql
  CREATE TABLE IF NOT EXISTS tiles (
      tile_id BLOB PRIMARY KEY,
      value   BLOB NOT NULL
  );
  ```

---

## 10. 设计取舍与空间评估

- 为降低复杂度，目前未写入任何额外元数据，Redis/BBolt/SQLite 都只存瓦片原始字节。
- 压缩主键（8B big-endian）在 BBolt/SQLite 间复用，可保证混层文件内主键唯一与可逆。
- 若未来新增列/头部，需要评估额外空间开销、兼容路径及迁移策略。

---

## 11. 后续演进建议

1. **元数据头部**：决定是否需要在值中编码 epoch/provider；如需要，统一编解码并迁移历史数据。
2. **SQLite 扩展列**：根据业务需要增加 epoch/provider/t 等列，并补充索引。
3. **tm 支持**：实现 Redis 键、BBolt/SQLite 复合主键以及 `TileStorage` 层的读写流程。
4. **批量优化**：为 SQLite 提供真正的批量事务写入，减少逐条写的开销。
5. **文档同步**：维持 `docs/files/*.md` 与代码的一致性，更新差异清单。

---

## 12. 相关代码元素（导航）
- `getDBPath`
- `compressTileKeyToUint64`
- `encodeKeyBigEndian`
- `PutTileBBolt`
- `PutTilesBBoltBatch`
- `PutTileSQLite`
- `PutTilesRedisBatch`
- `TileStorage`

---

如需扩展（例如为 tm 增加更多元数据、或支持 provider 过滤索引），可在 SQLite 侧增列/索引，KV 端保持极简头部不变，继续以 JSON 提供运行时映射。

## 13. q2/qp 处理方法（集合类元数据）

13.1 存储范围与层级规则
- 仅在层级可被 4 整除的锚点层存储：level ∈ {0,4,8,12,…}
- 任意 tilekey 存取时先映射到最近的锚点祖先：
  - level = len(tilekey)
  - anchorLen = level - (level % 4)
  - anchorKey = tilekey[:anchorLen]（当 anchorLen==0 表示根锚点；如需将 "0" 视为根别名，可统一映射到根锚点）
- 不存 epoch/provider：q2/qp 的 epoch 来自 dbroot 解析，值仅为原始元数据（protobuf/二进制），不加任何头部。

13.2 分库与主键
- 分库路由：始终用原始 tilekey（或其锚点 anchorKey）调用 `getDBPath` 生成目标库路径，复用既有分层策略。
- 主键：统一使用压缩后的 tileID（含层级位）→ 8 字节大端 BLOB（由 `compressTileKeyToUint64` → `encodeKeyBigEndian`）。
- 说明：压缩键包含层级位，保证同一分片文件内混层唯一与可逆解码；与瓦片表主键保持一致。

13.3 表结构（最小实现）
- 建议每类一表（或统一一表加 type 字段），最小列集合：
  - tile_id BLOB PRIMARY KEY（8B 压缩主键）
  - level INTEGER NOT NULL CHECK(level % 4 = 0)
  - data BLOB NOT NULL（原始 q2/qp 二进制，无头部）
  - status INTEGER NOT NULL DEFAULT 0（集合下载状态：0=PENDING，1=IN_PROGRESS，2=DONE，3=ERROR）
- 可选增强列：
  - prefix4 INTEGER NOT NULL（原始 tilekey 前4级路径位编码，便于区域筛选；与 `getDBPath` 分片天然对齐）
- 索引建议：
  - idx_status(status)、idx_level(level)、可选 idx_prefix4(prefix4)
- SQLite 初始化需启用 WAL（见规范）；写入前执行健康检查与损坏自动修复。

13.4 状态管理（与任务流转同步）
- 初始写入/生成集合任务：status=0(PENDING)
- 任一集合下任务开始执行：status→1(IN_PROGRESS)
- 集合全部下载完成（由任务模块判定）：status→2(DONE)
- 集合任务整体失败：status→3(ERROR)，后续可重置为 PENDING 重新入队
- 当集合原始数据更新（解析到新版本）：重置为 PENDING
- 状态更新应使用事务保证原子；按需为状态字段建索引，便于筛选与统计。

13.5 读取与寻址
- 存取一律按锚点：将任意 tilekey 映射到 anchorKey，再用 `getDBPath` 路由到库，使用压缩主键访问记录。
- 区域/层级筛选：使用 level 与可选 prefix4 索引实现快速查询；跨库查询先定位库集合，随后并行聚合。

13.6 Redis 使用原则
- q2/qp 属于“集合类元数据”，默认不走 Redis 缓存；直接写入 SQLite 便于后续查询、统计与调度。
- 如需临时缓存（调试场景），也仅缓存原始二进制，不加头部；生产环境建议关闭以简化架构。

13.7 兼容与约束
- 仅存锚点层记录，避免非整除层级重复存储导致爆炸。
- 值不包含 epoch/provider，保持极简；展示或分析时由 dbroot JSON 做映射解释。
- 主键与瓦片表一致（压缩 8B BLOB），与 `getDBPath` 分片一致（原始 tilekey 路由），保证系统全局一致性。

## 14. q2 任务队列（Redis）

14.1 键与数据结构
- 分库：q2/qp 各自使用独立 Redis 库，键名不携带数据类型前缀（库即类型边界）。
- 任务队列：`queue`（List）
  - 元素：直接推入解析生成的 JSON 项（来自 imagery_list、terrain_list、vector_list、q2_list 的每个元素），保持原始结构，便于调试与回溯。
- 任务状态：`status:{tilekey}`（Hash）
  - `status`: PENDING｜IN_PROGRESS｜DONE｜ERROR（字符串，可读性好）
  - `completed`: true｜false（初始 false，持久化成功后 true）
- 轻量索引集合：
  - `status:PENDING`、`status:IN_PROGRESS`、`status:DONE`、`status:ERROR`（Set，成员为 tilekey）
  - 可选排序集合：`status:PENDING:ts`（ZSet，score=scheduled_at/priority），便于按时间或优先级批量拉取。

14.2 流程与原子性
- 入队：解析 JSON → LPUSH `queue`；初始化状态：HSET `status:{tilekey}` status=PENDING completed=false；SADD `status:PENDING` {tilekey}
- 取任务：Worker BRPOP `queue` → 解析出 tilekey → 原子更新 HSET status=IN_PROGRESS；SMOVE `status:PENDING` → `status:IN_PROGRESS`
- 持久化成功：按照 `getDBPath` 路由分库，主键使用 `compressTileKeyToUint64` → `encodeKeyBigEndian`（8B BLOB）；成功后 HSET completed=true,status=DONE；SMOVE `status:IN_PROGRESS` → `status:DONE`；按配置执行 Redis 清理（见 `TileStorage` 的 `ClearRedisAfterPersist`）。
- 失败重试：HSET status=ERROR，并可附加 `last_error` 字段；后续重置为 PENDING 再入队。
- 可选锚点集合状态：`anchor_status:{anchorKey}`（Hash）用于记录集合层级整体状态（PENDING/IN_PROGRESS/DONE/ERROR 与 completed），`anchorKey` 为最近 level%4==0 的祖先键。

14.3 并发与幂等
- BRPOP 自带抢占；状态更新采用 HSET+SMOVE，简洁且具备良好并发语义。
- 如需强原子性，将“取任务→置 IN_PROGRESS→移动索引”封装为 Lua 脚本执行。
- 去重：可在入队前使用 SADD `seen:{hash}` 去重，或在状态表发现已是 IN_PROGRESS/DONE 则不重复入队。

14.4 说明
- Redis 负责“在线队列与即时状态”，复杂筛选（区域/层级/汇总）交由 SQLite 完成。
- 键名不含 `{dataType}`，因分库已隔离类型；保持与现有约定一致。

## 15. 中央任务库与索引（SQLite）

15.1 状态码与索引
- 集合锚点记录使用整数状态码，便于高效索引：
  - `status_code`: 0=PENDING，1=IN_PROGRESS，2=DONE，3=ERROR（与 Redis 字符串状态语义对齐）
- 索引：
  - `idx_status(status_code)`、`idx_level(level)`、可选 `idx_prefix4(prefix4)` 用于区域/层级筛选。

15.2 读写与对齐
- 写入：仅 level%4==0 的锚点记录；主键统一 8B BLOB（包含层级位）；值为原始 q2/qp 二进制，不含头部；`status_code` 初始为 0。
- 流转：任务开始置 1，全部完成置 2，失败置 3；若集合原始数据变更，重置为 0。
- 对齐：SQLite 中的整数状态码用于统计与复杂筛选；Redis 的字符串状态用于在线调度与监控，两者含义一致，各司其职。

