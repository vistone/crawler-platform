# 文档与源码对齐状态（阶段一）

> 目标：先完成“差异盘点”，后续按照目录分批补充 / 修订文档。  
> 范围：核心目录 `Store/`、`utlsclient/` 已完成初步核查，其余目录将在下一阶段继续补齐。

## Store 模块

- ✅ **Redis 键格式**：`docs/design/storage-spec.md` 已更新为 `<dataType>:<tilekey>`（tm 规划同样带前缀），与 `Store/redisdb.go` 保持一致，并新增 `docs/files/Store/redisdb.md` 解释细节。
- ✅ **SQLite schema**：文档已说明当前只存在 `tile_id`+`value` 两列，同时在 `docs/files/Store/sqlitedb.md` 中写明与旧描述的差异。
- ✅ **逐文件说明**：新增
  - `docs/files/Store/bblotdb.md`
  - `docs/files/Store/sqlitedb.md`
  - `docs/files/Store/dbpath.md`
  - `docs/files/Store/redisdb.md`
  - `docs/files/Store/tilestorage.md`
  每份文档列出了职责、关键函数与与历史描述的偏差。
- 🔄 **tm 相关描述**：`docs/design/storage-spec.md` 第 5 节改为“规划状态”，明确当前代码尚未实现 tm 专用结构。

## utlsclient 模块

1. **FILE_STRUCTURE.md 已修订**  
   - 已移除 `test/utlsclient/test_simple.go`、`test_structure.go` 等不存在的测试程序描述；明确测试位于 `utlsclient/*_test.go` 并给出运行命令。

2. **示例 & 测试文档缺少新增结构**  
   - 现状：`utlsclient` 目录新增了 `connection_helpers.go`, `connection_validator.go`, `ip_access_controller.go` 等文件，但 `docs/utlsclient/` 下没有逐文件说明。  
   - 计划：后续在 `docs/files/utlsclient/` 中为这些文件提供简介（职责、主要函数、输入输出），并在模块 README 中加入链接。

## 后续步骤

1. 进入 `utlsclient/` 目录：更新 `docs/utlsclient/FILE_STRUCTURE.md`、为新增源文件补写 `docs/files/utlsclient/*.md`。  
2. 扩展扫描范围到 `GoogleEarth/`, `cmd/`, `config/`, `test/`，持续完善差异清单。  
3. 每完成一个目录，就在 `docs/files/<path>.md` 中沉淀逐文件说明，并在 `docs/README.md` 上挂载索引。


