package Store

import (
	"encoding/binary"
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// BBoltManager 管理 bbolt 连接的池，避免重复打开同一数据库文件
type BBoltManager struct {
	mu   sync.Mutex          // 保护连接池
	pool map[string]*bolt.DB // dbPath -> *bolt.DB
	opts *bolt.Options       // 打开参数
}

var defaultBoltManager = NewBBoltManager()

// NewBBoltManager 创建管理器
func NewBBoltManager() *BBoltManager {
	return &BBoltManager{
		pool: make(map[string]*bolt.DB),
		opts: &bolt.Options{Timeout: 2 * time.Second}, // 独占锁等待超时
	}
}

// getOrOpenDB 获取或打开指定路径的 bbolt 数据库
func (m *BBoltManager) getOrOpenDB(dbPath string) (*bolt.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, ok := m.pool[dbPath]; ok && db != nil {
		return db, nil
	}

	// 确保目录存在
	if err := os.MkdirAll(path.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := bboltRecoverIfNeeded(dbPath, m.opts)
	if err != nil {
		return nil, err
	}
	m.pool[dbPath] = db
	return db, nil
}

// CloseAll 关闭所有已打开的数据库连接
func (m *BBoltManager) CloseAll() error {
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

// bboltRecoverIfNeeded 检测并在必要时尝试修复(或重新创建)损坏的 bbolt 数据库
func bboltRecoverIfNeeded(dbPath string, opts *bolt.Options) (*bolt.DB, error) {
	// 如果文件不存在,直接正常创建
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return bolt.Open(dbPath, 0o600, opts)
	}

	// 第一次尝试正常打开
	db, err := bolt.Open(dbPath, 0o600, opts)
	if err == nil {
		return db, nil
	}

	// 若打开失败,认为可能损坏: 先备份原文件,再新建
	backupPath := dbPath + ".corrupt." + time.Now().Format("20060102_150405")
	_ = os.Rename(dbPath, backupPath)
	return bolt.Open(dbPath, 0o600, opts)
}

// CompressTileKeyToUint64 将 tilekey 压缩为 64 位整数（高48位为路径位，低位为层级）
func CompressTileKeyToUint64(tilekey string) (uint64, error) {
	lvl := len(tilekey)
	if lvl <= 0 || lvl > 24 {
		return 0, errors.New("长度应在1..24之间")
	}
	var bits uint64
	for j := 0; j < lvl; j++ {
		c := tilekey[j]
		if c < '0' || c > '3' {
			return 0, errors.New("仅允许字符'0'..'3'")
		}
		val := uint64(c - '0')
		// 高位写入：每层使用2bit，按从高到低排布
		shift := 64 - uint64(j+1)*2
		bits |= (val & 0x03) << shift
	}
	// 低位写入层级值
	return bits | uint64(lvl), nil
}

// EncodeKeyBigEndian 将 uint64 主键编码为 8 字节大端
func EncodeKeyBigEndian(id uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], id)
	return buf[:]
}

// encodeKeyBigEndian 将 uint64 主键编码为 8 字节大端
func encodeKeyBigEndian(id uint64) []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], id)
	return buf[:]
}

// PutTileBBoltWithMetadata 写入单条数据（带元数据）：以压缩后的 tilekey 作为 bbolt 的 key
// dbdir: 数据库根目录；dataType: 数据类型（imagery/terrain/vector）；tilekey: 唯一标识；value: 负载数据；metadata: 元数据
func PutTileBBoltWithMetadata(dbdir, dataType, tilekey string, value []byte, metadata *TileMetadata) error {
	// 生成数据库文件路径（集合/分层策略在 getDBPath 内实现）
	dbPath := getDBPath(dbdir, dataType, tilekey, "bbolt") // 指定存储类型为bbolt

	// 打开连接（复用连接池）
	db, err := defaultBoltManager.getOrOpenDB(dbPath)
	if err != nil {
		return err
	}

	// 编码数据和元数据
	encodedData, err := encodeTileDataWithMetadata(value, metadata)
	if err != nil {
		return err
	}

	// 压缩 tilekey 以获得唯一、紧凑的主键
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	// 使用 dataType 作为桶名，便于分类管理
	bucketName := []byte(strings.ToLower(dataType))

	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return err
		}
		return b.Put(key, encodedData)
	})
}

// PutTileBBolt 写入单条数据：以压缩后的 tilekey 作为 bbolt 的 key
// dbdir: 数据库根目录；dataType: 数据类型（imagery/terrain/vector）；tilekey: 唯一标识；value: 负载数据
func PutTileBBolt(dbdir, dataType, tilekey string, value []byte) error {
	// 生成数据库文件路径（集合/分层策略在 getDBPath 内实现）
	dbPath := getDBPath(dbdir, dataType, tilekey, "bbolt") // 指定存储类型为bbolt

	// 打开连接（复用连接池）
	db, err := defaultBoltManager.getOrOpenDB(dbPath)
	if err != nil {
		return err
	}

	// 压缩 tilekey 以获得唯一、紧凑的主键
	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	// 使用 dataType 作为桶名，便于分类管理
	bucketName := []byte(strings.ToLower(dataType))

	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return err
		}
		return b.Put(key, value)
	})
}

// PutTilesBBoltBatch 批量写入多条数据（高性能）：在单个事务中写入所有数据
// 适用于批量导入场景，性能比逐条调用 PutTileBBolt 提升 10-100 倍
func PutTilesBBoltBatch(dbdir, dataType string, records map[string][]byte) error {
	if len(records) == 0 {
		return nil
	}

	// 按数据库路径分组（不同层级可能在不同数据库文件）
	grouped := make(map[string]map[string][]byte)
	for tilekey, value := range records {
		dbPath := getDBPath(dbdir, dataType, tilekey, "bbolt") // 指定存储类型为bbolt
		if grouped[dbPath] == nil {
			grouped[dbPath] = make(map[string][]byte)
		}
		grouped[dbPath][tilekey] = value
	}

	// 对每个数据库文件执行批量写入
	for dbPath, group := range grouped {
		db, err := defaultBoltManager.getOrOpenDB(dbPath)
		if err != nil {
			return err
		}

		bucketName := []byte(strings.ToLower(dataType))

		// 单个事务批量写入
		err = db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(bucketName)
			if err != nil {
				return err
			}

			for tilekey, value := range group {
				tileID, err := CompressTileKeyToUint64(tilekey)
				if err != nil {
					return err
				}
				key := encodeKeyBigEndian(tileID)
				if err := b.Put(key, value); err != nil {
					return err
				}
			}
			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}

// GetTileBBolt 读取单条数据
func GetTileBBolt(dbdir, dataType, tilekey string) ([]byte, error) {
	dbPath := getDBPath(dbdir, dataType, tilekey, "bbolt") // 指定存储类型为bbolt
	db, err := defaultBoltManager.getOrOpenDB(dbPath)
	if err != nil {
		return nil, err
	}

	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return nil, err
	}
	key := encodeKeyBigEndian(tileID)

	bucketName := []byte(strings.ToLower(dataType))

	var val []byte
	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return errors.New("bucket not found")
		}
		val = b.Get(key)
		if val == nil {
			return errors.New("key not found")
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return val, nil
}

// DeleteTileBBolt 删除单条数据
func DeleteTileBBolt(dbdir, dataType, tilekey string) error {
	dbPath := getDBPath(dbdir, dataType, tilekey, "bbolt") // 指定存储类型为bbolt
	db, err := defaultBoltManager.getOrOpenDB(dbPath)
	if err != nil {
		return err
	}

	tileID, err := CompressTileKeyToUint64(tilekey)
	if err != nil {
		return err
	}
	key := encodeKeyBigEndian(tileID)

	bucketName := []byte(strings.ToLower(dataType))

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		if b == nil {
			return nil // bucket不存在，视为删除成功
		}
		return b.Delete(key)
	})
}

// CloseAllBBolt 关闭所有 BBolt 连接
func CloseAllBBolt() error {
	return defaultBoltManager.CloseAll()
}
