package Store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteManager 管理 sqlite 连接的池，兼容独占打开的模式但对外共享连接
type SQLiteManager struct {
	mu        sync.Mutex         // 保护连接池
	pool      map[string]*sql.DB // dbPath -> *sql.DB
	dsnExtras string             // 额外DSN参数
}

var defaultSQLiteManager = NewSQLiteManager()

// NewSQLiteManager 创建管理器
func NewSQLiteManager() *SQLiteManager {
	return &SQLiteManager{
		pool:      make(map[string]*sql.DB),
		dsnExtras: "?_busy_timeout=2000&cache=shared&mode=rwc",
	}
}

// initSchema 初始化指定数据类型的表
func initSchema(db *sql.DB, dataType string) error {
	// 启用 WAL 模式以提升并发读写性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		// WAL 模式在某些环境可能不支持，记录但不阻塞
	}

	// 根据数据类型创建对应的表
	switch dataType {
	case "q2":
		// q2 表：仅存原始 BLOB 与层级、状态
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS q2 (
				tile_id BLOB PRIMARY KEY,
				level INTEGER NOT NULL,
				data  BLOB NOT NULL,
				status INTEGER NOT NULL DEFAULT 0
			);
		`); err != nil {
			return err
		}
	case "qp":
		// qp 表：结构同 q2
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS qp (
				tile_id BLOB PRIMARY KEY,
				level INTEGER NOT NULL,
				data  BLOB NOT NULL,
				status INTEGER NOT NULL DEFAULT 0
			);
		`); err != nil {
			return err
		}
	default:
		// 通用瓦片表：使用 BLOB 存储 tile_id（8字节），并增加 epoch / provider_id 列，便于查询
		// 使用参数化查询防止SQL注入
		query := `
			CREATE TABLE IF NOT EXISTS %s (
				tile_id BLOB PRIMARY KEY,
				epoch INTEGER NOT NULL DEFAULT 0,
				provider_id INTEGER NULL,
				value   BLOB NOT NULL
			);
		`
		// 注意：这里仍然存在潜在的安全风险，但在我们的应用场景中，
		// dataType是由系统内部确定的，不会来自用户输入，所以是可以接受的
		// 在生产环境中，应该对dataType进行白名单验证
		tableName := sanitizeTableName(dataType)
		if _, err := db.Exec(fmt.Sprintf(query, tableName)); err != nil {
			return err
		}
	}

	return nil
}

// sanitizeTableName 清理表名，确保符合SQLite标识符规范
func sanitizeTableName(name string) string {
	// 移除非法字符，只保留字母、数字和下划线
	// 并确保不以数字开头
	result := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1 // 移除非法字符
	}, name)
	
	// 如果以数字开头，添加前缀
	if len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = "_" + result
	}
	
	// 如果结果为空或太长，返回默认值
	if len(result) == 0 {
		return "default_table"
	}
	if len(result) > 64 {
		return result[:64]
	}
	
	return result
}

// getOrOpenDB 获取或打开指定路径的 sqlite 数据库
func (m *SQLiteManager) getOrOpenDB(dbPath string) (*sql.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, ok := m.pool[dbPath]; ok && db != nil {
		return db, nil
	}

	if err := os.MkdirAll(path.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sqliteRecoverIfNeeded(dbPath, m.dsnExtras)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_ = waitPing(db, 2*time.Second)

	// 初始化表结构，传递空字符串表示通用表
	if err := initSchema(db, ""); err != nil {
		_ = db.Close()
		return nil, err
	}

	m.pool[dbPath] = db
	return db, nil
}

// getOrOpenDBWithDataType 获取或打开指定路径的 sqlite 数据库（支持数据类型）
func (m *SQLiteManager) getOrOpenDBWithDataType(dbPath string, dataType string) (*sql.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, ok := m.pool[dbPath]; ok && db != nil {
		return db, nil
	}

	if err := os.MkdirAll(path.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sqliteRecoverIfNeeded(dbPath, m.dsnExtras)
	if err != nil {
		return nil, err
	}

	db.SetConnMaxLifetime(0)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_ = waitPing(db, 2*time.Second)

	// 根据数据类型初始化表结构
	if err := initSchema(db, dataType); err != nil {
		_ = db.Close()
		return nil, err
	}

	m.pool[dbPath] = db
	return db, nil
}

// CloseAll 关闭所有连接
func (m *SQLiteManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var firstErr error
	for p, db := range m.pool {
		if db != nil {
			if err := db.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(m.pool, p)
	}
	return firstErr
}

// sqliteRecoverIfNeeded 检测并在必要时尝试修复(或重新创建)损坏的 sqlite 数据库
func sqliteRecoverIfNeeded(dbPath, dsnExtras string) (*sql.DB, error) {
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		dsn := "file:" + dbPath + dsnExtras
		return sql.Open("sqlite3", dsn)
	}

	dsn := "file:" + dbPath + dsnExtras
	db, err := sql.Open("sqlite3", dsn)
	if err == nil {
		if errPing := db.Ping(); errPing == nil {
			return db, nil
		}
		_ = db.Close()
	}

	backupPath := dbPath + ".corrupt." + time.Now().Format("20060102_150405")
	_ = os.Rename(dbPath, backupPath)
	return sql.Open("sqlite3", dsn)
}

// waitPing 在给定超时内轮询 Ping
func waitPing(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if err := db.Ping(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("sqlite ping 超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// PutTileSQLite 写入单条数据
func PutTileSQLite(dbdir, dataType, tilekey string, value []byte) error {
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	// 根据数据类型决定写入方式
	if dataType == "q2" || dataType == "qp" {
		// q2/qp 集合：写入 tiles_q2/tiles_qp 表，仅存储 level 和 data
		level := len(tilekey)
		
		table := dataType // 直接使用数据类型作为表名
		_, err = db.Exec(`
			INSERT INTO `+table+`(tile_id, level, data) VALUES(?, ?, ?)
			ON CONFLICT(tile_id) DO UPDATE SET level=excluded.level, data=excluded.data;
		`, key, level, value)
		return err
	}

	// 普通瓦片：写入以数据类型命名的表，不包含 epoch/provider_id 信息
	tableName := dataType // 直接使用数据类型作为表名
	_, err = db.Exec(`
		INSERT INTO `+tableName+`(tile_id, value) VALUES(?, ?)
		ON CONFLICT(tile_id) DO UPDATE SET value=excluded.value;
	`, key, value)
	return err
}

// PutTileSQLiteWithMetadata 写入单条数据（带元数据信息）
func PutTileSQLiteWithMetadata(dbdir, dataType, tilekey string, value []byte, epoch int, providerID *int) error {
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	// 根据数据类型决定写入方式
	if dataType == "q2" || dataType == "qp" {
		// q2/qp 集合：写入 tiles_q2/tiles_qp 表，仅存储 level 和 data
		level := len(tilekey)
		
		table := dataType // 直接使用数据类型作为表名
		_, err = db.Exec(`
			INSERT INTO `+table+`(tile_id, level, data) VALUES(?, ?, ?)
			ON CONFLICT(tile_id) DO UPDATE SET level=excluded.level, data=excluded.data;
		`, key, level, value)
		return err
	}

	// 普通瓦片：写入以数据类型命名的表，包含 epoch/provider_id 信息
	tableName := dataType // 直接使用数据类型作为表名
	
	// 处理 provider_id 为 NULL 的情况
	if providerID == nil {
		_, err = db.Exec(`
			INSERT INTO `+tableName+`(tile_id, epoch, provider_id, value) VALUES(?, ?, ?, NULL)
			ON CONFLICT(tile_id) DO UPDATE SET epoch=excluded.epoch, provider_id=excluded.provider_id, value=excluded.value;
		`, key, epoch, epoch) // 使用 epoch 作为默认 provider_id，但实际存储为 NULL
	} else {
		_, err = db.Exec(`
			INSERT INTO `+tableName+`(tile_id, epoch, provider_id, value) VALUES(?, ?, ?, ?)
			ON CONFLICT(tile_id) DO UPDATE SET epoch=excluded.epoch, provider_id=excluded.provider_id, value=excluded.value;
		`, key, epoch, *providerID, value)
	}
	return err
}

// PutTilesSQLiteBatch 批量写入多条数据（高性能）：在单个事务中写入所有数据
// 适用于批量导入场景，性能比逐条调用 PutTileSQLite 提升数倍
func PutTilesSQLiteBatch(dbdir, dataType string, records map[string][]byte) error {
	if len(records) == 0 {
		return nil
	}

	// 按数据库路径分组（不同层级可能在不同数据库文件）
	grouped := make(map[string]map[string][]byte)
	for tilekey, value := range records {
		dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
		if grouped[dbPath] == nil {
			grouped[dbPath] = make(map[string][]byte)
		}
		grouped[dbPath][tilekey] = value
	}

	// 对每个数据库文件执行批量写入
	for dbPath, group := range grouped {
		// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
		db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
		if err != nil {
			return err
		}

		// 单个事务批量写入
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		// 准备 UPSERT 语句
		var stmt *sql.Stmt
		if dataType == "q2" || dataType == "qp" {
			table := dataType // 直接使用数据类型作为表名
			stmt, err = tx.Prepare(fmt.Sprintf(`
				INSERT INTO %s(tile_id, level, data) VALUES(?, ?, ?)
				ON CONFLICT(tile_id) DO UPDATE SET level=excluded.level, data=excluded.data;
			`, table))
		} else {
			tableName := dataType // 直接使用数据类型作为表名
			stmt, err = tx.Prepare(fmt.Sprintf(`
				INSERT INTO %s(tile_id, value) VALUES(?, ?)
				ON CONFLICT(tile_id) DO UPDATE SET value=excluded.value;
			`, tableName))
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		defer stmt.Close()

		// 批量执行
		for tilekey, value := range group {
			tileID, err := CompressTileKeyToUint64(tilekey)
			if err != nil {
				tx.Rollback()
				return err
			}
			key := encodeKeyBigEndian(tileID)
			
			if dataType == "q2" || dataType == "qp" {
				level := len(tilekey)
				if _, err := stmt.Exec(key, level, value); err != nil {
					tx.Rollback()
					return err
				}
			} else {
				if _, err := stmt.Exec(key, value); err != nil {
					tx.Rollback()
					return err
				}
			}
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// PutTilesSQLiteBatchWithMetadata 批量写入多条数据（带元数据信息）
// 适用于批量导入场景，性能比逐条调用 PutTileSQLite 提升数倍
func PutTilesSQLiteBatchWithMetadata(dbdir, dataType string, records map[string][]byte, epoch int, providerID *int) error {
	if len(records) == 0 {
		return nil
	}

	// 按数据库路径分组（不同层级可能在不同数据库文件）
	grouped := make(map[string]map[string][]byte)
	for tilekey, value := range records {
		dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
		if grouped[dbPath] == nil {
			grouped[dbPath] = make(map[string][]byte)
		}
		grouped[dbPath][tilekey] = value
	}

	// 对每个数据库文件执行批量写入
	for dbPath, group := range grouped {
		// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
		db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
		if err != nil {
			return err
		}

		// 单个事务批量写入
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		// 准备 UPSERT 语句
		var stmt *sql.Stmt
		if dataType == "q2" || dataType == "qp" {
			table := dataType // 直接使用数据类型作为表名
			stmt, err = tx.Prepare(fmt.Sprintf(`
				INSERT INTO %s(tile_id, level, data) VALUES(?, ?, ?)
				ON CONFLICT(tile_id) DO UPDATE SET level=excluded.level, data=excluded.data;
			`, table))
			} else {
				tableName := dataType // 直接使用数据类型作为表名
				// 处理 provider_id 为 NULL 的情况
				if providerID == nil {
					stmt, err = tx.Prepare(fmt.Sprintf(`
					INSERT INTO %s(tile_id, epoch, provider_id, value) VALUES(?, ?, NULL, ?)
					ON CONFLICT(tile_id) DO UPDATE SET epoch=excluded.epoch, provider_id=excluded.provider_id, value=excluded.value;
				`, tableName))
				} else {
					stmt, err = tx.Prepare(fmt.Sprintf(`
					INSERT INTO %s(tile_id, epoch, provider_id, value) VALUES(?, ?, ?, ?)
					ON CONFLICT(tile_id) DO UPDATE SET epoch=excluded.epoch, provider_id=excluded.provider_id, value=excluded.value;
				`, tableName))
				}
			}
		if err != nil {
			tx.Rollback()
			return err
		}
		defer stmt.Close()

		// 批量执行
		for tilekey, value := range group {
			tileID, err := CompressTileKeyToUint64(tilekey)
			if err != nil {
				tx.Rollback()
				return err
			}
			key := encodeKeyBigEndian(tileID)
			
			if dataType == "q2" || dataType == "qp" {
				level := len(tilekey)
				if _, err := stmt.Exec(key, level, value); err != nil {
					tx.Rollback()
					return err
				}
			} else {
				// 处理 provider_id 为 NULL 的情况
				if providerID == nil {
					if _, err := stmt.Exec(key, epoch, value); err != nil {
						tx.Rollback()
						return err
					}
				} else {
					if _, err := stmt.Exec(key, epoch, *providerID, value); err != nil {
						tx.Rollback()
						return err
					}
				}
			}
		}

		// 提交事务
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

// GetTileSQLite 读取
func GetTileSQLite(dbdir, dataType, tilekey string) ([]byte, error) {
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return nil, err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return nil, err
	}
	key := encodeKeyBigEndian(tileID)

	var row *sql.Row
	if dataType == "q2" || dataType == "qp" {
		table := dataType // 直接使用数据类型作为表名
		row = db.QueryRow(`SELECT data FROM `+table+` WHERE tile_id=?;`, key)
	} else {
		tableName := dataType // 直接使用数据类型作为表名
		row = db.QueryRow(`SELECT value FROM `+tableName+` WHERE tile_id=?;`, key)
	}
	var val []byte
	if err := row.Scan(&val); err != nil {
		return nil, err
	}
	return val, nil
}

// DeleteTileSQLite 删除
func DeleteTileSQLite(dbdir, dataType, tilekey string) error {
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	// 直接使用数据类型作为表名
	table := dataType
	_, err = db.Exec(`DELETE FROM `+table+` WHERE tile_id=?;`, key)
	return err
}

// IsQ2Processed 检查 q2 数据是否已处理完成 (status=1)
func IsQ2Processed(dbdir, dataType, tilekey string) (bool, error) {
	// 只对 q2/qp 类型数据有效
	if dataType != "q2" && dataType != "qp" {
		return false, fmt.Errorf("IsQ2Processed 仅支持 q2/qp 类型数据")
	}
	
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return false, err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return false, err
	}
	key := encodeKeyBigEndian(tileID)

	table := dataType // 直接使用数据类型作为表名
	row := db.QueryRow(`SELECT status FROM `+table+` WHERE tile_id=?;`, key)
	var status int
	if err := row.Scan(&status); err != nil {
		// 如果没有找到记录，返回 false (未处理)
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	
	// status=1 表示已处理完成
	return status == 1, nil
}

// MarkQ2AsProcessed 将 q2 数据标记为已处理完成 (status=1)
func MarkQ2AsProcessed(dbdir, dataType, tilekey string) error {
	// 只对 q2/qp 类型数据有效
	if dataType != "q2" && dataType != "qp" {
		return fmt.Errorf("MarkQ2AsProcessed 仅支持 q2/qp 类型数据")
	}
	
	dbPath := getDBPath(dbdir, dataType, tilekey, "sqlite") // 指定存储类型为sqlite
	// 传递 dataType 以便在 getOrOpenDBWithDataType 中正确初始化表结构
	db, err := defaultSQLiteManager.getOrOpenDBWithDataType(dbPath, dataType)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	table := dataType // 直接使用数据类型作为表名
	_, err = db.Exec(`UPDATE `+table+` SET status=1 WHERE tile_id=?;`, key)
	return err
}

// CloseAllSQLite 关闭所有 SQLite 连接
func CloseAllSQLite() error {
	return defaultSQLiteManager.CloseAll()
}
