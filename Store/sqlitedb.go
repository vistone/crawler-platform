package Store

import (
	"database/sql"
	"errors"
	"os"
	"path"
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

	if err := initSchema(db); err != nil {
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

// initSchema 初始化 tiles 表
func initSchema(db *sql.DB) error {
	// 启用 WAL 模式以提升并发读写性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		// WAL 模式在某些环境可能不支持，记录但不阻塞
		// 可选：记录日志或返回错误
	}

	// 使用 BLOB 存储 tile_id（8字节），避免 SQLite INTEGER 对 uint64 高位的限制
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tiles (
			tile_id BLOB PRIMARY KEY,
			value   BLOB NOT NULL
		);
	`)
	return err
}

// PutTileSQLite 写入（UPSERT）单条数据
func PutTileSQLite(dbdir, dataType, tilekey string, value []byte) error {
	dbPath := getDBPath(dbdir, dataType, tilekey)
	db, err := defaultSQLiteManager.getOrOpenDB(dbPath)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID) // 使用 8 字节 BLOB
	_, err = db.Exec(`
		INSERT INTO tiles(tile_id, value) VALUES(?, ?)
		ON CONFLICT(tile_id) DO UPDATE SET value=excluded.value;
	`, key, value)
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
		dbPath := getDBPath(dbdir, dataType, tilekey)
		if grouped[dbPath] == nil {
			grouped[dbPath] = make(map[string][]byte)
		}
		grouped[dbPath][tilekey] = value
	}

	// 对每个数据库文件执行批量写入
	for dbPath, group := range grouped {
		db, err := defaultSQLiteManager.getOrOpenDB(dbPath)
		if err != nil {
			return err
		}

		// 单个事务批量写入
		tx, err := db.Begin()
		if err != nil {
			return err
		}

		// 准备 UPSERT 语句
		stmt, err := tx.Prepare(`
			INSERT INTO tiles(tile_id, value) VALUES(?, ?)
			ON CONFLICT(tile_id) DO UPDATE SET value=excluded.value;
		`)
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
			if _, err := stmt.Exec(key, value); err != nil {
				tx.Rollback()
				return err
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
	dbPath := getDBPath(dbdir, dataType, tilekey)
	db, err := defaultSQLiteManager.getOrOpenDB(dbPath)
	if err != nil {
		return nil, err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return nil, err
	}
	key := encodeKeyBigEndian(tileID)
	row := db.QueryRow(`SELECT value FROM tiles WHERE tile_id=?;`, key)
	var val []byte
	if err := row.Scan(&val); err != nil {
		return nil, err
	}
	return val, nil
}

// DeleteTileSQLite 删除
func DeleteTileSQLite(dbdir, dataType, tilekey string) error {
	dbPath := getDBPath(dbdir, dataType, tilekey)
	db, err := defaultSQLiteManager.getOrOpenDB(dbPath)
	if err != nil {
		return err
	}
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)
	_, err = db.Exec(`DELETE FROM tiles WHERE tile_id=?;`, key)
	return err
}

// CloseAllSQLite 关闭所有 SQLite 连接
func CloseAllSQLite() error {
	return defaultSQLiteManager.CloseAll()
}
