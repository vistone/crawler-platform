# 存储架构与键值编码规范（KV + SQLite + tm）

本文档总结并固化当前讨论形成的统一存储规范，用于指导 Redis（KV 缓存）、BBolt（KV 持久化）、SQLite（列式持久化）三者的一致性实现与互操作。内容围绕以下核心：
- 键（Key）格式与生成时机
- 值（Value）头部极简编码（epoch、provider_id）
- 路径分片与压缩键的协同
- tm（带时间）数据类型的复合键设计
- 兼容与空间开销考量

---

## 1. 总体原则（结论）

- Redis 键使用“原始 tilekey”，不含数据类型前缀（Redis 已分库按类型隔离）。
- BBolt/SQLite 主键一律使用“压缩后的 tileID”（8 字节，大端 BLOB）。
- 路径分片始终基于“原始 tilekey”，与压缩键无冲突：
  - 生成文件路径时使用 `getDBPath`（原始 tilekey → 分层目录/文件）。
  - 写入持久化时，将原始 tilekey压缩为 uint64（含层级位），再编码为 8 字节大端主键。
- KV 值统一采用“极简自描述头部”（无版本号）：
  - 头部顺序：epoch(Uvarint) → provider_id(Uvarint) → data
  - provider 为空时编码为 0（单字节），空间极小。
- SQLite 按列式存储（不使用 KV 头部）：
  - 普通表：tile_id(BLOB PK)、epoch(INTEGER)、provider_id(INTEGER NULL)、data(BLOB)
  - tm 表：PRIMARY KEY(tile_id, t)，列含 t(INTEGER 毫秒)、epoch、provider_id、data
- 仅存 `provider_id`（整数）；版权字符串、offset 等由 dbroot 的 JSON 或映射表在运行时解释，不进入瓦片数据存储。

---

## 2. 键（Key）规范

### 2.1 Redis（分库按类型）
- 普通瓦片（imagery/terrain/vector/q2/qp）：
  - 键：原始 tilekey（字符串），如 `"01230123"`
- tm（带时间瓦片，tm 专用 Redis 库）：
  - 键：`原始tilekey:milliseconds`，如 `"01230123:1732612345678"`
- 说明：不再包含 `{dataType}` 前缀，因为 Redis 已通过分库隔离类型。

### 2.2 BBolt/SQLite（统一主键）
- 主键：压缩后的 tileID（8 字节大端 BLOB），来自：
  - `compressTileKeyToUint64` → `encodeKeyBigEndian`
- 含层级位的压缩键是必要的：保证同一数据库文件内混合层级时的主键唯一与可逆解码。

---

## 3. 值（Value）极简头部编码（KV 专用）

- KV 的 Value 采用极简自描述头部（无版本号）：
  - 头部顺序：epoch(Uvarint) → provider_id(Uvarint) → data(BLOB)
- Uvarint 编码规则：每字节低 7 位为数值，高位 0x80 表示“后续有字节”；最后一个字节不置高位。
- 示例：
  - epoch = 1029：1029 = 8×128 + 5 → Uvarint：`85 08`
  - provider_id = 124：`7C`
  - 头部字节前缀（hex）：`85 08 7C`，后接原始 `data`
  - provider 为空：Uvarint 编码为 `00`
- 空间开销：
  - provider 为空时，头部通常 ≈ 3 字节（epoch≈2B + provider_id=0≈1B）。
  - 相比在 data 中混存 epoch/provider，头部可独立、快速读取，且开销极低。

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

## 5. tm（带时间）数据类型的复合键

### 5.1 Redis（tm 库）
- 键：`原始tilekey:milliseconds`
- 值：KV 头部（epoch Uvarint、provider_id Uvarint） + data

### 5.2 BBolt（KV 持久化）
- 复合主键（二进制）：
  - 8 字节 tileID（压缩后的 uint64 大端）
  - 8 字节毫秒时间戳（big-endian）
  - 共 16 字节，支持按 tileID 聚簇、按时间排序/迭代。

### 5.3 SQLite（列式）
- 表主键：PRIMARY KEY(tile_id BLOB, t INTEGER)
- 列：epoch(INTEGER)、provider_id(INTEGER NULL)、data(BLOB)
- 索引：按需添加 `idx_t(t)`、`idx_provider(provider_id)`、`idx_epoch(epoch)`。

---

## 6. provider 存储与解释

- 瓦片值中仅存 `provider_id`（整数），不存版权字符串或偏移：
  - KV：Uvarint 编码 `provider_id`（为空为 0）。
  - SQLite：`provider_id INTEGER NULL`（为空存 NULL）。
- 展示时由系统加载全局 JSON（dbroot 提供）进行 `provider_id → 版权字符串/offset` 映射；瓦片数据存储不嵌入这些字符串，避免膨胀。

---

## 7. 兼容性与容错

- 旧数据兼容：
  - 解码 KV 值时，若长度不足以解析两个 Uvarint（epoch/provider_id），视为旧格式：`epoch=0`、`provider_id=0`、`data=原值`。
- provider 为空：
  - KV 头部编码为 `00`（Uvarint 的 0）。
  - SQLite 存 NULL。
- Redis 分库安全：
  - 已按类型分库，键不再需要 `{dataType}` 前缀；库本身即类型边界。

---

## 8. 读写流程（简图）

- 写入（异步持久化开启时）：
  1. 下载数据 → Redis 写入：键=原始 tilekey（或 tm 键），值=Uvarint 头部 + data（毫秒级响应）。
  2. 后台 Worker 批量持久化：
     - 路径分片：`getDBPath`（原始 tilekey）
     - 主键生成：`compressTileKeyToUint64` → `encodeKeyBigEndian`（8B）
     - BBolt：KV 写入（普通 8B 主键；tm 16B 复合主键）。
     - SQLite：列式写入（tile_id/t/epoch/provider_id/data）。
  3. 持久化成功 → 根据配置清理 Redis。

- 读取：
  - 快速读（近期数据）→ Redis（直接解析 Uvarint 头部拿 epoch/provider_id）。
  - 历史/范围/统计 → SQLite（列式 + 索引）。

---

## 9. 示例对照

- KV 头部示例（十六进制）：
  - 条件：`epoch=1029`、`provider_id=124`
  - Uvarint：epoch → `85 08`，provider_id → `7C`
  - Value 前缀：`85 08 7C` + `data`

- Redis 键示例：
  - 普通：`"01230123"`
  - tm：`"01230123:1732612345678"`

- BBolt/SQLite 主键（BLOB 8B）：来自 `compressTileKeyToUint64` → `encodeKeyBigEndian`。

---

## 10. 设计取舍与空间评估

- KV 头部开销：provider 为空时，通常 ≈ 3B；数据量巨大时也可控（相对每条瓦片 data 的体量极小）。
- SQLite 列式不使用头部，provider 为空存 NULL，查询友好、空间更省。
- 压缩键的层级位不可去除（混层文件下的主键唯一性与可逆性）。

---

## 11. 落地任务建议（实现顺序）

1. KV 头部编解码工具：Uvarint epoch / provider_id 的 encode/decode。
2. Redis 键切换为原始 tilekey（tm 键追加 `:milliseconds`）。
3. 持久化阶段：路径分片（原始 key）＋ 压缩主键（8B BLOB）。
4. SQLite 表/索引：普通表与 tm 表增加列并建索引（按需）。
5. 异步持久化 Worker：按规范批量写入；成功后根据配置清理 Redis。
6. 旧数据兼容：KV 解码器容错（无头部 → epoch=0/provider=0）。

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

